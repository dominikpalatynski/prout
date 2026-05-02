# PRD — Runtime Environment Audit Trail and Live Operation Visibility

## Problem Statement

Toolshed already has the right lifecycle anchors for preview work:

- `internal/jobs/preview_start_flow.go` drives `preview-start` through `source_materialization`, `compose_preparation`, and `runtime_deployment`
- `internal/jobs/preview_cleanup_flow.go` drives teardown and workspace cleanup for superseded attempts
- `internal/jobs/operation_request_progress.go` persists the current step snapshot on `operation_requests`
- `internal/runtime/dockercompose/backend.go` owns `docker compose config`, `up`, and `down`

What is missing is durable history and live execution visibility.

Today the system only keeps the latest `current_step`, `current_step_state`, and `current_step_details_json` for one `Operation Request`. That is a snapshot, not an audit trail. When the worker advances or retries, the previous state is overwritten. At the runtime boundary the problem is sharper: the Docker Compose runner currently uses `CombinedOutput()`, so `docker compose up` is invisible until the command exits, and successful command output is discarded entirely.

The result is that an operator can tell that a preview is "in runtime_deployment", but cannot answer practical questions such as:

- Did `docker compose up` actually start?
- Which workspace action already ran?
- Is the worker waiting, retrying, or stuck?
- What was the last meaningful runtime message?
- What happened to a superseded attempt before cleanup removed its workspace?

## Goals

1. Create a durable append-only audit trail for workspace and runtime lifecycle operations.
2. Make long-running runtime commands observable while they are still running.
3. Keep retrieval rooted in existing domain anchors: `Operation Request` and `Runtime Environment`.
4. Preserve the current orchestration model instead of redesigning `preview-start`.
5. Keep audit payloads safe: no repository env-var values, no bearer tokens, and no application log streaming by default.

## Non-Goals

- Full distributed tracing or OpenTelemetry rollout.
- Streaming container application logs from `docker compose logs`.
- Introducing "workspace" as a new operator-facing top-level domain entity.
- Multi-node pub/sub infrastructure in the first version.

## Solution

Introduce one append-only audit/event system for runtime work, backed by PostgreSQL and exposed through polling and SSE.

The current `operation_requests.current_step*` fields should remain the fast current snapshot. Audit history should move to a separate append-only model instead of trying to encode history inside `current_step_details_json`.

## Current Gaps in the Code

- `internal/jobs/operation_request_progress.go` persists only the latest step snapshot. There is no historical step log.
- `operation_requests` has no `updated_at`, so even simple "last touched" ordering is weak.
- `internal/runtime/dockercompose/backend.go` only returns command output after exit through `CombinedOutput()`.
- The API exposes `GET /api/runtime-environments` and webhook-centric operation detail, but there is no direct operation feed and no live stream.
- Structured logs include request, repository, PR, and River job correlation, but not a durable queryable event stream keyed by `operation_request_id` or `runtime_environment_id`.

## Proposed Architecture

### 1. Append-only `audit_events`

Add a new table such as `audit_events` with one row per meaningful lifecycle event.

Recommended columns:

- `id BIGSERIAL PRIMARY KEY`
- `repository_id BIGINT NOT NULL`
- `pull_request_id BIGINT`
- `operation_request_id BIGINT`
- `runtime_environment_id BIGINT`
- `workspace_locator TEXT`
- `source TEXT NOT NULL`
- `level TEXT NOT NULL`
- `event_type TEXT NOT NULL`
- `step TEXT`
- `message TEXT NOT NULL`
- `details_json JSONB`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

Recommended indexes:

- `(operation_request_id, id ASC)`
- `(runtime_environment_id, id ASC)`
- `(repository_id, id DESC)`
- `(workspace_locator, id DESC)` if workspace-locator lookup is needed operationally

This table should be append-only. No updates, no reuse, no "current row" semantics.

### 2. Event Taxonomy

Keep the event vocabulary small and practical. The first version does not need dozens of event kinds.

Recommended categories:

- `operation_request.started`
- `operation_request.retried`
- `operation_request.completed`
- `operation_request.failed`
- `step.entered`
- `step.completed`
- `step.failed`
- `workspace.staging_created`
- `workspace.source_extracted`
- `workspace.promoted`
- `workspace.cleaned`
- `runtime.prepare.started`
- `runtime.prepare.completed`
- `runtime.command.started`
- `runtime.command.output`
- `runtime.command.completed`
- `runtime.snapshot.captured`
- `runtime.superseded`

This is enough to reconstruct the story of one preview attempt without creating a new domain model.

### 3. Snapshot vs History

Keep `operation_requests.current_step`, `current_step_state`, and `current_step_details_json` as the operator's current snapshot.

Add `updated_at` to `operation_requests` and update it whenever progress changes or the request is finalized.

Use `current_step_details_json` only for lightweight "where is it now?" state, for example:

- `workspace_locator`
- `deployment_backend`
- `active_command`
- `command_started_at`
- `last_output_at`
- `last_output_excerpt`
- `last_audit_event_id`

Do not stuff full command output or historical step history into `current_step_details_json`.

### 4. `internal/audit` Package

The empty `internal/audit/` package is the right place to centralize this.

Recommended responsibilities:

- event type definitions
- `Recorder` interface
- DB-backed recorder implementation
- in-process subscriber hub for SSE clients
- no-op recorder for tests and non-runtime callers
- helper constructors for common event payloads

The worker should not need to know whether an event is only persisted or also broadcast live.

### 5. Worker-Level Instrumentation

Emit audit events from the orchestration layer at the boundaries that matter:

- when a worker starts handling an `Operation Request`
- whenever a step enters `in_progress`
- whenever a step completes or fails
- when a runtime attempt is reused, superseded, or finalized
- when cleanup starts and finishes

This should happen close to the existing step-transition code in `internal/jobs/operation_request_progress.go` and the preview flow modules, not as scattered ad hoc log lines.

### 6. Workspace Lifecycle Instrumentation

Workspace management is currently visible only indirectly through success or failure of higher-level steps.

Add audit emission around these operations:

- `CreateStaging`
- `ExtractTarball`
- `PromoteStaging`
- `CleanupStaging`
- `CleanupWorkspace`

The cleanest implementation is a small auditing decorator around the `workspaceManager` interface rather than embedding DB writes directly inside the filesystem package.

### 7. Live Docker Compose Visibility

Replace the current `CombinedOutput()` runner with a streaming command runner.

The runtime backend should be able to emit:

- command start
- stdout/stderr chunks or normalized lines
- command exit status
- elapsed time

That means changing the command runner contract from "return all bytes at the end" to "stream progress while the command runs and return final status".

Implementation direction:

- use `exec.CommandContext`
- wire `StdoutPipe` and `StderrPipe`
- scan output concurrently
- normalize to plain text before persistence
- emit `runtime.command.output` events as output arrives
- still classify final errors with the existing permanent vs retryable rules

This is the key change that makes `docker compose up` observable in real time instead of only after failure.

### 8. Post-Command Runtime Snapshot

After a successful `docker compose up`, capture one lightweight runtime snapshot and persist it as an audit event.

The snapshot should answer:

- which services or containers were created
- whether the exposed service is running
- any container names or IDs that help with diagnosis

This is much more useful to an operator than only knowing that `runtime_deployment` became `completed`.

### 9. API Surface

Add direct audit retrieval APIs instead of forcing operators to infer state from webhook-event detail.

Recommended endpoints:

- `GET /api/operation-requests/{operationRequestID}`
- `GET /api/operation-requests/{operationRequestID}/events?after_id=&limit=`
- `GET /api/runtime-environments/{runtimeEnvironmentID}/events?after_id=&limit=`
- `GET /api/operation-requests/{operationRequestID}/events/stream`

The stream endpoint should use SSE. For the current deployment model, an in-process broadcast hub plus DB replay on reconnect is enough. Do not start with PostgreSQL `LISTEN/NOTIFY` unless later scaling requires it.

Important modeling choice:

- retrieval should be rooted in `Operation Request` or `Runtime Environment`
- `workspace_locator` should remain a filterable technical attribute, not the main operator-facing API root

### 10. Structured Log Correlation

Extend contextual logging so application logs also carry:

- `operation_request_id`
- `runtime_environment_id`
- `workspace_locator`

Logs are not the audit system, but this makes ad hoc terminal debugging line up with the durable event feed.

## Suggested File Touch Points

Likely modules to change:

- `migrations/` for `audit_events` and `operation_requests.updated_at`
- `internal/audit/` for the recorder and live hub
- `internal/jobs/operation_request.go`
- `internal/jobs/operation_request_progress.go`
- `internal/jobs/preview_start_flow.go`
- `internal/jobs/preview_cleanup_flow.go`
- `internal/runtime/runtime.go`
- `internal/runtime/dockercompose/backend.go`
- `internal/server/routes.go`
- `internal/server/api.go`
- `internal/log/log.go`

## Rollout Plan

### Phase 1: Durable Audit Foundation

- add `audit_events`
- add `operation_requests.updated_at`
- emit worker and step-transition events
- expose polling endpoints for operation and runtime event history
- enrich structured logs with operation and runtime identifiers

Value:

- immediate historical visibility
- no change yet to Docker command execution behavior

### Phase 2: Live Runtime Command Streaming

- replace `CombinedOutput()` with a streaming runner
- emit `runtime.command.started`, `output`, and `completed`
- persist compact current-command summary into `current_step_details_json`
- add SSE endpoint for live updates

Value:

- real-time visibility for `docker compose up` and `docker compose down`
- operators can see whether a deploy is active, stalled, or already failed

### Phase 3: Broaden Coverage Across Workspace Management

- audit workspace cleanup and superseded-attempt flows in the same way
- audit future operations such as restart or manual delete under the same event model
- optionally add retention policies for noisy command-output events

Value:

- one coherent audit mechanism across all workspace/runtime management paths

## Data Retention and Safety

Recommended guardrails:

- do not persist repository environment-variable values in audit payloads
- do not persist full rendered Compose YAML in audit events
- do not stream container application logs in this phase
- cap stored command output per command and record truncation explicitly
- keep command output plain and normalized so the audit feed is readable and indexable

If retention becomes a concern, expire only high-volume `runtime.command.output` events first and keep step/state events longer.

## Testing Decisions

- migration tests for `audit_events` and `operation_requests.updated_at`
- store tests for append-only ordering and filtering by operation/runtime
- worker tests asserting event emission around step transitions and failure paths
- Docker Compose backend tests asserting streamed command output and final classification behavior
- API tests for polling endpoints and SSE reconnection behavior

Tests should verify externally visible history and state, not only helper call order.

## Why This Fits the Current Design

- It preserves `preview-start` as one business-level `Operation Type`.
- It keeps `Runtime Environment Status` coarse and operator-facing.
- It uses `Operation Request` and `Runtime Environment` as the existing durable anchors.
- It treats workspace as a technical artifact, which matches `CONTEXT.md`.
- It solves the actual blind spot in `docker compose up` instead of only adding another summary field.

## Recommendation

Implement Phase 1 and Phase 2 together if possible.

Phase 1 alone gives historical audit, but it still leaves the most painful operator question unanswered while `docker compose up` is running. The biggest practical win comes from pairing append-only audit history with a streaming command runner so that the operator can both reconstruct what happened later and watch what is happening now.
