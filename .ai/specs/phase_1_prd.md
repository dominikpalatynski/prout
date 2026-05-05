# Phase 1 PRD: Walking Skeleton Foundation

## Problem Statement

As the Operator, I need a real walking skeleton for prout before building real GitHub ingress, Preview Environment lifecycle, and deployment runtime behavior. Right now the repository has parts of the scaffold in place, but it does not yet prove the end-to-end Phase 1 promise: start the application, connect to PostgreSQL, validate schema state, accept a GitHub-shaped webhook request, enqueue asynchronous work in River, consume that work in-process, and expose a simple technical way to verify that the pipeline actually ran.

Without this slice, the project remains structurally promising but operationally unproven. That makes later phases riskier because real GitHub integration, deployment orchestration, and panel work would be layered on top of a foundation that has not yet demonstrated the minimal happy path.

## Solution

Phase 1 will deliver a thin but complete technical slice focused on local operation. The Operator will be able to boot the application with a slimmed-down `server.yml` configuration, run migrations through Task-based commands, and start one process that owns both HTTP ingress and River workers. The application will fail fast if PostgreSQL, River, or schema state is not ready.

The system will expose a GitHub-shaped `POST /webhooks/github` endpoint that supports exactly one accepted case in this phase: `pull_request/opened`. A valid request will enqueue a `RecordPingJob`. The worker will consume that job and persist a row into the `_ping` probe table. A temporary debug endpoint will expose recent `_ping` rows so the Operator can manually verify that the asynchronous pipeline executed successfully.

## User Stories

1. As the Operator, I want a minimal working slice of prout, so that later phases build on a proven runtime path instead of assumptions.
2. As the Operator, I want to start prout with a small Phase 1 configuration surface, so that local setup does not pretend GitHub App, OAuth, ACME, or panel features already exist.
3. As the Operator, I want to keep the existing `server.yml` plus env override approach, so that Phase 1 stays consistent with the current repository approach while trimming unused fields.
4. As the Operator, I want migrations to run through Task-based commands, so that schema changes remain explicit and separate from application startup.
5. As the Operator, I want the server to fail fast when the database schema is out of date, so that I do not accidentally run the binary against an invalid schema.
6. As the Operator, I want one process to run both HTTP ingress and River workers, so that Phase 1 proves the full asynchronous pipeline with minimal topology complexity.
7. As the Operator, I want the process to fail if River or its workers cannot start, so that the application never serves partial functionality.
8. As the Operator, I want `/healthz` to remain a cheap process-level probe, so that I can distinguish basic liveness from dependency readiness.
9. As the Operator, I want `/readyz` to check database and River readiness, so that readiness means the Phase 1 pipeline can actually accept and process work.
10. As the Operator, I want a GitHub-shaped webhook route at `POST /webhooks/github`, so that the ingress surface already resembles the later real integration.
11. As the Operator, I want that webhook route to accept a small nested GitHub-like JSON payload, so that the HTTP contract stays close to future GitHub payload structure without requiring full webhook parsing yet.
12. As the Operator, I want the webhook route to require `X-GitHub-Event` and `X-GitHub-Delivery`, so that even the mock ingress keeps the shape of a GitHub delivery.
13. As the Operator, I want Phase 1 to support only `pull_request/opened`, so that the walking skeleton proves one real asynchronous path instead of building an event matrix too early.
14. As the Operator, I want technically invalid requests to return `400`, so that contract violations are visible immediately and are not confused with ignored events.
15. As the Operator, I want syntactically valid but unsupported events or actions to return `202` without enqueueing work, so that ignored cases are distinct from malformed requests.
16. As the Operator, I want `202` for the supported case only after the job is successfully enqueued, so that “accepted” means the request truly entered the asynchronous pipeline.
17. As the Operator, I want enqueue failures to return `500`, so that the ingress contract does not claim success when no background work will happen.
18. As the Operator, I want Phase 1 to skip delivery deduplication, so that event modeling and `webhook_events` persistence remain properly deferred to Phase 2.
19. As the Operator, I want the job payload to contain only the minimal normalized data needed for Phase 1, so that the worker path stays thin and focused.
20. As the Operator, I want the worker job to be called `RecordPingJob`, so that its purpose is explicit and not hidden behind a vague placeholder like `EchoJob`.
21. As the Operator, I want `RecordPingJob` to persist into `_ping`, so that I have a durable probe of worker execution instead of relying only on logs.
22. As the Operator, I want database access for `_ping` to use the existing `sqlc` pattern, so that Phase 1 reinforces the intended store architecture rather than adding a second temporary style.
23. As the Operator, I want a temporary `GET /debug/pings` endpoint, so that I can manually verify that webhook ingestion produced the expected database-side effect.
24. As the Operator, I want `GET /debug/pings` to return the last N ping rows with a limit, so that manual verification is quick and does not expose an unbounded table dump.
25. As the Operator, I want the debug endpoint to be explicitly temporary and technical, so that it does not become an accidental long-term product API.
26. As the Operator, I want River UI to stay out of the Phase 1 acceptance boundary, so that the walking skeleton stays focused on pipeline correctness rather than extra observability surface.
27. As the Operator, I want to verify Phase 1 manually with curl and a debug readback path, so that I can confirm the runtime slice without adding automated integration tests yet.
28. As the future maintainer of prout, I want Phase 1 naming and contracts to stay honest about what is mock and what is real, so that Phase 2 can extend the foundation without untangling misleading placeholders.

## Implementation Decisions

- Phase 1 remains a walking skeleton only. Real GitHub integration concerns stay deferred: signature verification, delivery deduplication, `webhook_events` persistence, installation authentication, and broader event routing.
- The application continues to use the existing `server.yml` plus env override loading mechanism, but the active Phase 1 runtime contract is slimmed down to the fields actually required now.
- Later-phase configuration concerns are explicitly removed from active Phase 1 scope: GitHub App configuration, OAuth, ACME, panel host configuration, public domain routing, and other panel-centric settings are not part of this slice.
- Migrations are executed through Task-based commands rather than a new CLI subcommand. The server process does not mutate schema on startup.
- Startup must perform schema compatibility checking and fail fast if the database is behind the embedded migration set.
- Phase 1 runs as a single process that owns HTTP ingress and River worker execution via separate goroutines under one lifecycle context.
- The process must fail startup if PostgreSQL, River client initialization, or worker startup fails.
- Liveness and readiness are intentionally separated: liveness is process-only, readiness reflects database and River worker pipeline readiness.
- The accepted webhook route is `POST /webhooks/github`.
- The accepted request contract is GitHub-shaped but intentionally minimal: headers include `X-GitHub-Event` and `X-GitHub-Delivery`; the JSON body contains only the minimal nested fields needed for `pull_request/opened`.
- The only supported semantic case in Phase 1 is `pull_request/opened`.
- Unsupported but well-formed events or actions return `202` and do not enqueue work.
- Malformed or incomplete requests return `400`.
- The supported case returns `202` only after successful job enqueue; enqueue failure returns `500`.
- No delivery deduplication is performed in Phase 1. Repeated deliveries may create repeated jobs and repeated `_ping` rows.
- The background job is named `RecordPingJob`.
- `RecordPingJob` carries only the minimal normalized webhook fields needed by the worker, such as delivery identifier, event name, Repository identifier, pull request number, and SHA.
- The worker side effect is a call equivalent to `InsertPing`, creating a new `_ping` row for each successfully processed job.
- `_ping` access follows the existing `sqlc`-based store direction rather than direct raw SQL.
- A new debug read endpoint exposes recent pings as a bounded list with a limit parameter.
- The debug read endpoint is explicitly technical and temporary. It exists to prove Phase 1 behavior, not to define long-term product API shape.
- River UI is not part of the Phase 1 deliverable.
- Manual smoke verification is the acceptance path for this phase.
- The major modules to build or modify are:
- Bootstrap and configuration surface: slim the active Phase 1 config contract while keeping the current loading mechanism.
- Startup guardrail module: schema version check plus dependency startup ordering for PostgreSQL and River.
- Webhook contract adapter: minimal request validation and normalization for the single supported GitHub-shaped delivery.
- Worker module: `RecordPingJob` registration, enqueue path, and worker execution.
- Probe store module: `sqlc` queries for inserting and listing ping rows.
- Debug probe surface: `GET /debug/pings` as a bounded temporary verification endpoint.
- Deep-module candidates for Phase 1 are:
- Webhook normalization and validation as a narrow boundary that turns HTTP input into minimal internal job arguments.
- Startup guardrails as a single module that decides whether the process is allowed to run.
- Probe store access as a stable data boundary around `_ping`.

## Testing Decisions

- Good tests should verify external behavior rather than internal implementation details. For this phase, the most important behaviors are: invalid webhook requests return the right status, accepted webhook requests enqueue work successfully, and successful worker execution produces a durable `_ping` row.
- Phase 1 will rely on manual smoke verification instead of new automated integration tests.
- The manual verification path should cover: bringing up local PostgreSQL, applying migrations, starting the server, posting a supported webhook payload, and reading back recent `_ping` rows from the debug endpoint.
- No new automated module tests are required for this phase.
- If automated tests are introduced later, the best first targets are the webhook contract adapter, the startup guardrails, and the `_ping` store boundary.
- Prior art exists for future integration-style tests in the existing test database helper, which shows the repository already anticipates that style of verification even though Phase 1 will not require it yet.

## Out of Scope

- Real GitHub signature verification.
- Delivery deduplication.
- `webhook_events` persistence.
- GitHub App authentication and installation token flow.
- Repository registration or installation lifecycle handling.
- Preview Environment deployment or teardown runtime behavior.
- Docker Compose runtime orchestration.
- Public routing, TLS, and DNS.
- Panel, OAuth sign-in, and access control.
- River UI.
- Automated integration tests for the Phase 1 walking skeleton.
- A `prout migrate` CLI command.
- Any operator workflow that depends on panel behavior or public HTTPS exposure.

## Further Notes

- This PRD intentionally narrows Phase 1 to a provable technical slice rather than a partial implementation of later product behavior.
- The acceptance story is local and manual by design: post one supported webhook request, observe one enqueued `RecordPingJob`, and verify one new `_ping` record through the debug endpoint.
- The naming choices in this phase should stay honest about the technical nature of the slice. `RecordPingJob` and `debug/pings` are scaffolding-oriented names, not domain names for long-term product features.
