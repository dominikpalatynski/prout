# Toolshed — Technical Architecture Document

**GitHub Automation Bot — Self-Hosted, Personal-Scale**
*Version 2.0 · MVP Technical Specification · April 2026*

---

## Overview

Toolshed is a self-hosted GitHub automation bot built primarily for the maintainer's own open source project, with an architectural path to evolve into a more general-purpose product. The MVP scope is narrow and concrete: orchestrate ephemeral preview environments for pull requests, triggered by GitHub events (PR open/synchronize/close, labels, comments). Configuration of registered repositories happens through a minimal web panel; system-level configuration is provided once via a config file at install time.

The product takes design inspiration from Probot (lightweight, GitHub-native bot pattern) and Prow (built-in plugin model with config-driven behavior), positioning itself between them: more opinionated than a framework, lighter than a Kubernetes-scale CI system.

The first capability is preview environments. Subsequent capabilities (e.g. agent-driven autofix, label management, triage automation) are deliberately out of scope for the MVP but are architecturally accommodated as future plugins within the same monolith.

---

## Goals

1. **Single deployable bot for personal-scale OSS use.** One VPS, one binary, low operational overhead.
2. **Preview environments as a first-class, working capability.** A pull request results in a unique HTTPS URL, posted as a PR comment.
3. **Configuration-via-UI, not config-as-code.** Per-repository behavior (triggers, lifecycle, env vars) is defined and edited in the web panel; nothing related to bot configuration lives in the target repository.
4. **Architecturally extensible.** Adding a new automation capability is implemented as a new internal module, registered through a plugin interface, without restructuring the system.
5. **Initial setup via config file.** System-level configuration (domain, GitHub App credentials, encryption keys, defaults) is supplied once at install time on the host.
6. **Operable from CLI for administrative tasks.** Day-to-day runtime actions are available both in the panel and via CLI; configuration of system-wide concerns is exposed through CLI/config file only.
7. **Path to broader extraction.** Architecture allows for the system to evolve into a more general-purpose tool over time, but no effort is spent on multi-tenant or marketplace-grade concerns in the MVP.

---

## System Architecture

### High-Level Components

```
┌────────────────────────────────────────────────────────────────────┐
│  GitHub                                                            │
│  - GitHub App (webhooks: pull_request, issue_comment, installation)│
│  - GitHub OAuth (panel sign-in)                                    │
│  - GitHub REST API (permissions, comments, status, tarball)        │
└────────────────┬───────────────────────────────────────────────────┘
                 │ webhooks + API
                 ▼
┌────────────────────────────────────────────────────────────────────┐
│  Toolshed VPS (single host)                                        │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  docker-compose stack                                        │  │
│  │                                                              │  │
│  │  ┌──────────┐  ┌────────────────────┐  ┌─────────────────┐   │  │
│  │  │ Traefik  │  │ Toolshed (Go)      │  │ PostgreSQL 16   │   │  │
│  │  │ (proxy,  │  │ - HTTP server      │  │ - state         │   │  │
│  │  │  TLS)    │  │ - webhook receiver │  │ - River queue   │   │  │
│  │  │          │  │ - panel (SSR)      │  │ - build logs    │   │  │
│  │  │          │  │ - CLI entrypoint   │  │ - audit/events  │   │  │
│  │  │          │  │ - River workers    │  │                 │   │  │
│  │  └────┬─────┘  └─────────┬──────────┘  └─────────────────┘   │  │
│  │       │                  │                                   │  │
│  │       │      ┌───────────┴────────────┐                      │  │
│  │       │      │  Docker Socket Proxy   │                      │  │
│  │       │      │  (filtered API access) │                      │  │
│  │       │      └───────────┬────────────┘                      │  │
│  │       │                  │                                   │  │
│  └───────┼──────────────────┼───────────────────────────────────┘  │
│          │                  ▼                                      │
│          │         Docker daemon (host)                            │
│          ▼                  │                                      │
│      Preview                ▼                                      │
│      env stacks   ┌────────────┬────────────┬────────────┐         │
│      (TLS via     │ PR-123     │ PR-456     │ PR-789     │         │
│      Traefik      │ stack      │ stack      │ stack      │         │
│      auto-disc.)  │ (isolated  │ (isolated  │ (isolated  │         │
│                   │  network)  │  network)  │  network)  │         │
│                   └────────────┴────────────┴────────────┘         │
└────────────────────────────────────────────────────────────────────┘
```

### Component Interactions

1. **GitHub → Toolshed (webhooks).** PR events, comments, installation events are delivered as signed webhook payloads. The receiver verifies HMAC-SHA256, persists the event, evaluates triggers, and enqueues jobs.
2. **Toolshed → GitHub (REST API).** The bot reads collaborator permissions, posts/updates PR comments, sets commit status checks, and downloads repository tarballs at specific commit SHAs.
3. **Browser → Toolshed (panel).** Maintainer signs in via GitHub OAuth, manages registered repositories, edits trigger and lifecycle settings, monitors environments, views live build logs (Server-Sent Events), and triggers manual runtime actions.
4. **Toolshed → Docker daemon (via socket proxy).** The bot creates networks, builds and runs preview containers, and tears them down through the Docker Engine API. The socket proxy restricts the API surface exposed to the bot.
5. **Traefik → Preview containers.** Traefik discovers preview containers via Docker labels and routes traffic to per-PR subdomains with automatic TLS.

### Layered Internal Structure

The Toolshed binary is a single Go process organized into layers:

```
toolshed/
├── cmd/toolshed/                 entry point (HTTP + workers + CLI)
├── internal/
│   ├── core/
│   │   ├── webhook/              webhook receiver, signature verification
│   │   ├── githubapp/            GitHub App auth, installation tokens, REST client
│   │   ├── plugins/              Plugin interface and registry
│   │   ├── store/                Postgres access (sqlc-generated)
│   │   └── jobs/                 River setup, common job types
│   ├── plugins/
│   │   └── preview/              Preview-environments plugin (only one in MVP)
│   ├── webui/                    SSR panel (html/template, SSE)
│   └── cli/                      Cobra commands (admin)
├── migrations/                   SQL migrations (goose or similar)
└── config.example.yml            example server-level config
```

The plugin interface in MVP carries a single implementation but is shaped so that future plugins (autofix, triage, etc.) can register against the same event router without touching core code.

---

## Technology Stack

### Backend

| Component | Technology | Rationale |
|---|---|---|
| Core application | **Go** (static binary, `CGO_ENABLED=0`) | Standard for infrastructure tooling; single binary; strong concurrency primitives |
| HTTP server | **`net/http`** (stdlib) | Sufficient surface for webhooks, API, panel |
| Routing/middleware | **`chi`** (or stdlib mux) | Minimal, idiomatic |
| YAML parsing | **`goccy/go-yaml`** | Better error messages for server config |
| Job queue | **River** (`github.com/riverqueue/river`) | Postgres-native, type-safe, retries, periodic jobs |
| Database access | **sqlc** + `pgx` | Type-safe SQL, no ORM bloat |
| Migrations | **goose** | Simple, works with `pgx`/`database/sql` |
| GitHub API client | **`google/go-github`** | Mature, well-maintained |
| Docker client | **Docker Engine SDK for Go** | Talks to Docker daemon via socket proxy |
| HTML rendering | **`html/template`** (stdlib) | SSR panel, no build pipeline |
| CLI framework | **`cobra`** | Standard for Go CLIs |
| Encryption (future) | **`crypto/nacl/box`** | Reserved for later secret-handling work |

### Frontend (panel)

| Component | Technology | Rationale |
|---|---|---|
| Rendering | **Server-side HTML** (Go `html/template`) | No SPA, no React, no build step |
| Styling | **Tailwind via CDN** (or single hand-written CSS file) | Visual baseline without tooling |
| Live updates | **Server-Sent Events (SSE)** | Build log streaming, environment status |
| Auth | **GitHub OAuth** | Re-use GitHub App; access gated by GitHub repo permissions |

### Infrastructure

| Component | Technology | Rationale |
|---|---|---|
| Reverse proxy | **Traefik v3** | Native Docker provider, ACME, industry standard |
| Container runtime | **Docker Engine** (host) | Universal availability; Docker Compose for build/run target |
| Socket proxy | **Tecnativa docker-socket-proxy** | API filtering; reduces blast radius vs direct socket mount |
| TLS provisioning | **Traefik ACME** (DNS-01) | Wildcard certificates; Cloudflare or Route53 in MVP |
| Container image | **`distroless`** or **`scratch`** | Minimal attack surface |

### Storage

| Component | Technology | Rationale |
|---|---|---|
| Primary database | **PostgreSQL 16** | State, queue (River), build logs, audit |
| Workspace storage | **Bind mount** (`/var/lib/toolshed/workspaces`) | Per-build temporary directory, cleaned up after build |

### Observability (MVP scope)

| Component | Technology | Rationale |
|---|---|---|
| Application logs | **stdout** (structured JSON via `slog`) | Captured by Docker logging driver |
| Build logs | **PostgreSQL** + **SSE** | Persisted, searchable, viewed in panel |
| Audit/event log | **PostgreSQL** | Records all webhook events, config changes, runtime actions |
| Metrics | None (deferred) | Personal-scale; not needed in MVP |

---

## Architectural Patterns

### Monolith with Internal Plugins

A single Go process hosts:
- HTTP server (webhooks, API, panel, SSE log streams)
- Worker pool (River-managed goroutines processing jobs)
- Periodic scheduler (River cron) for TTL cleanup and reconciliation
- CLI entrypoint (same binary, different command)

Plugins are internal Go packages registered via a `Plugin` interface (`Name() string`, `Subscribe() []EventType`, `Handle(ctx, event) error`). The MVP ships exactly one plugin (`preview`). The interface exists to keep core/event routing free from preview-specific logic and to enable adding future plugins without touching the core.

### Event-Driven Orchestration

Webhook events are the primary input. The flow is:
1. Webhook handler validates and persists the event.
2. Webhook handler resolves the target repository's configuration from the database.
3. Trigger evaluation (server-side, using live GitHub permission data) decides whether to act.
4. Matching plugins receive the event and enqueue jobs.
5. Workers asynchronously perform long-running tasks (clone, build, deploy, teardown).

The webhook handler responds within ~100 ms; everything heavy is asynchronous through River.

### Database-as-Source-of-Truth for Configuration

Per-repository configuration lives only in PostgreSQL. There is no `.toolshed/preview.yml` or analogous file in target repositories. The web panel writes to the same tables that the runtime reads. Configuration changes apply to subsequent triggers immediately; no rebuild, no deploy.

This is a deliberate inversion of the original PRD's config-as-code model. It is appropriate for personal use because (a) one operator, (b) git history is not the desired audit log, (c) UI editing is faster for iteration.

The host-level configuration (domain, GitHub App credentials, encryption keys, defaults) lives in `/etc/toolshed/server.yml` plus environment variables and is loaded once at startup. It is intentionally not editable through the panel.

### Three-Surface Configuration Model

| Surface | Responsibility |
|---|---|
| **Server config file** (`/etc/toolshed/server.yml` + env vars) | One-time install: wildcard domain, GitHub App ID/private key, webhook secret, DNS provider credentials, encryption master key, default limits. Edited via SSH only. |
| **Web panel** | Per-repository configuration: enable/disable, triggers, permissions, lifecycle (TTL, max concurrent), env vars (non-sensitive), exposed service/port, compose file path. Also: dashboard, live logs, manual runtime actions (redeploy, teardown, extend TTL). |
| **CLI** | Admin operations: `init`, `migrate`, `reconcile`, `env list`, `env destroy`, `logs <pr-number>`. Same backing operations as the panel where applicable, exposed for scripting and SSH workflows. |

Each piece of configuration has exactly one source of truth. The panel does not edit server config; the CLI does not edit per-repo config.

### Stateless Authentication for the Panel

Panel sign-in uses GitHub OAuth. A signed-in user's authorization is checked live: they must hold a sufficient permission level on at least one registered repository. There is no separate user database, no password storage, no role table. Sessions are short-lived signed cookies.

---

## Key Technical Decisions

### 1. Personal-scale product, not OSS-for-the-world

- **Decision:** Treat Toolshed as a tool to support the maintainer's own OSS project, with an opt-in path to broader use later.
- **Context:** The original PRD targeted a public OSS audience with strict onboarding and DX requirements.
- **Rationale:** Eliminates ~70% of complexity that existed only to satisfy hypothetical external users (config-as-code in target repos, plugin marketplace, multi-tenant security model, polished onboarding).
- **Trade-offs:** Some decisions (DB-as-source-of-truth, single-VPS-only) become harder to reverse if the product scope later widens. Architecture is kept clean enough that those reversals are feasible but not free.

### 2. Monolith with plugin interface

- **Decision:** Single Go binary; plugins are internal packages implementing a shared interface.
- **Context:** Future capabilities (autofix, triage, label management) are anticipated, but only preview is built in MVP.
- **Rationale:** Lowest operational cost; one process, one log, one deploy. Interface boundary keeps preview-specific logic out of the core, allowing later plugins to be added or extracted to separate processes without rewrites.
- **Trade-offs:** Heavy future plugins (e.g. agent runtimes) will share resources with the bot core. Migration to per-plugin processes is possible but requires plumbing work.

### 3. Postgres + River as queue

- **Decision:** Use PostgreSQL with River for both state and asynchronous job processing.
- **Context:** Webhook handlers must respond within GitHub's 10-second timeout; builds take 1–5 minutes.
- **Rationale:** Postgres is already needed for state; River avoids re-implementing job semantics (retries, scheduling, periodic jobs, `SELECT FOR UPDATE SKIP LOCKED`). Avoids introducing Redis or another broker.
- **Trade-offs:** Lower throughput ceiling than Redis-based queues — irrelevant at personal scale. River is opinionated; replacing it later is non-trivial but rarely necessary.

### 4. Configuration in DB, not in target repositories

- **Decision:** Per-repository configuration lives in PostgreSQL and is edited exclusively in the web panel.
- **Context:** Original PRD specified `.toolshed/preview.yml` in each repo (config-as-code).
- **Rationale:** Operator is the maintainer; UI iteration is faster than git roundtrips. Removes the need for `contents: read` on the repo to fetch config, simplifies trust model (no fork-vs-default-branch distinction needed for config), and makes auto-registration on App install meaningful.
- **Trade-offs:** Loses git as audit log — replaced by an in-app audit table. Loses the "review config in PR" affordance — irrelevant for solo operation. Multi-operator scenarios (future) need explicit conflict handling.

### 5. `docker-compose.preview.yml` stays in the repository

- **Decision:** The compose file describing how to build and run the preview stack remains in the target repository.
- **Context:** Several decisions move config out of repos; this one moves the opposite direction.
- **Rationale:** This file is part of the application's definition, not the bot's configuration. The bot does not dictate how the application is built; it consumes a contract the project already maintains.
- **Trade-offs:** Repository must contain at least one preview-specific file. Acceptable because this file is co-located with the code it builds.

### 6. Single VPS, bot and preview environments share the host

- **Decision:** Toolshed and all preview environments run on the same VPS.
- **Context:** Preview environments need a public IP, TLS, and Docker access; running them remotely from the bot adds significant orchestration complexity.
- **Rationale:** Simplest viable topology. Sufficient for personal-scale workloads. Path to multi-host (workers on separate VPSes) is preserved by River's worker-process separation, but unused in MVP.
- **Trade-offs:** A heavy build can degrade preview-environment performance. Mitigated by `max_concurrent` limits and per-container resource caps (CPU, memory, PIDs) injected at deploy time.

### 7. Docker socket access via socket proxy

- **Decision:** Toolshed accesses the host Docker daemon through `tecnativa/docker-socket-proxy`, not via direct socket mount.
- **Context:** Docker socket access grants root-equivalent control over the host.
- **Rationale:** Socket proxy filters the Docker API surface to only what Toolshed needs (containers, networks, images). Defense in depth. Same pattern Traefik uses inside this stack.
- **Trade-offs:** Marginal added complexity (one extra container). Documented requirement: dedicated VPS, not shared with production workloads.

### 8. Server-side rendered panel, no SPA

- **Decision:** The web panel is rendered server-side with Go `html/template`. SSE handles live log streaming.
- **Context:** The panel's surface is small: list, edit forms, dashboard, log viewer, manual action buttons.
- **Rationale:** No build pipeline, no React, no API/frontend split. Faster to write, faster to render, easier to maintain. Tailwind via CDN gives a usable visual baseline without tooling.
- **Trade-offs:** Highly interactive widgets (e.g. drag-and-drop) are awkward — none are needed. Bringing in HTMX (or similar) is a low-cost option later if interactivity grows.

### 9. GitHub OAuth for panel auth (no separate user system)

- **Decision:** Panel sign-in uses GitHub OAuth; authorization is derived live from GitHub repo permissions.
- **Context:** Personal-scale tool; introducing a user database would be overhead with no benefit.
- **Rationale:** Re-uses the GitHub App relationship. The set of authorized users equals "people with sufficient permission on at least one registered repository."
- **Trade-offs:** Requires GitHub OAuth flow plumbing. No offline access (acceptable). Future multi-tenant scenarios would require a real user/org model.

### 10. No application secret management in MVP

- **Decision:** Preview environments receive only non-sensitive environment variables, configured in the panel.
- **Context:** Original PRD designed a full encrypted-secret pipeline using GitHub Environments + Actions transport. This is a large surface area.
- **Rationale:** For personal OSS preview environments, most workloads can be previewed against in-compose Postgres, mocked external services, and fake API keys. Implementing a proper secret manager (encryption at rest, key rotation, fork approval gating) is several days of work and adds significant attack surface.
- **Trade-offs:** Can't preview features that require real third-party credentials. Migration path: add an `encrypted_secrets` table, NaCl-based encryption with a master key in `server.yml`, and a panel form. The MVP env-var schema generalizes naturally to this.

### 11. Multi-repository support from day 1

- **Decision:** Schema includes `repository_id` foreign keys throughout. Single-repository operation is just one row.
- **Context:** First user is the maintainer's own OSS project, but other personal projects are likely candidates.
- **Rationale:** Adds essentially zero implementation cost; opens optionality. Aligns with the stated path of "maybe something more comes out of this."
- **Trade-offs:** None of consequence at this scale.

### 12. Tarball-based code retrieval

- **Decision:** Toolshed downloads PR head as a tarball via GitHub REST API, not `git clone`.
- **Context:** Preview environments need code at a specific SHA, including from forks.
- **Rationale:** Atomic (single SHA, no race), lightweight (no `.git` history), works for forks via the same installation token, no SSH key plumbing.
- **Trade-offs:** No submodules, no LFS. Future fallback to `git clone` is possible but unnecessary in MVP.

### 13. Compose file sanitization

- **Decision:** Toolshed parses and rejects dangerous constructs in the user-provided compose file before executing it.
- **Context:** Compose can specify host bind mounts, privileged mode, host networking — all container-escape vectors.
- **Rationale:** Even though the operator is the maintainer, fork PRs can mutate compose files. Hard rejection of: `privileged: true`, `cap_add`, host bind mounts, `pid: host`, `network_mode: host`, `ipc: host`, `devices`. Only named volumes and relative-path volumes within the build context permitted.
- **Trade-offs:** Some development patterns (host-bind hot reload) are blocked. For MVP, security takes precedence.

### 14. Server-wide resource limits injected at deploy time

- **Decision:** Every preview container receives default CPU, memory, and PID limits, regardless of compose file content.
- **Context:** A fork PR running an accidental fork bomb or busy-loop can crash the host.
- **Rationale:** Defaults defined in `server.yml`; injected by Toolshed into the generated compose at deploy time. Simple, broadly protective.
- **Trade-offs:** Some legitimate workloads hit the limits. Operator can adjust defaults globally.

### 15. Live GitHub permission checks per trigger event

- **Decision:** For every trigger event, the bot calls GitHub's permissions endpoint to verify the actor's role.
- **Context:** Webhook payloads tell *who* did something, not *whether they're authorized*.
- **Rationale:** GitHub is the source of truth. Live checks make permission revocation immediately effective. The actor checked depends on the event type (sender vs commenter vs PR author).
- **Trade-offs:** ~50 ms latency per webhook. Acceptable.

### 16. Automatic trigger hard-disabled for forks

- **Decision:** The `automatic` trigger type does not apply to PRs from forks, regardless of configuration.
- **Context:** Auto-deploying arbitrary code from external contributors is a footgun.
- **Rationale:** Forked PRs require an explicit label or comment trigger initiated by an authorized user, mirroring the model of GitHub Actions' restrictions on `pull_request` events from forks.
- **Trade-offs:** Maintainers contributing from personal forks must use label/comment triggers. The bot detects this case (PR author has `write+` permission on the base repo) and treats the PR as non-fork for trigger evaluation.

### 17. Build logs in Toolshed (not GitHub)

- **Decision:** Build logs are stored in Postgres and served via the panel; not streamed into GitHub Actions or GitHub commit checks.
- **Context:** Builds happen on the VPS, not on a GitHub runner.
- **Rationale:** Persistent (no 90-day expiry), searchable, accessible from the panel via signed URL referenced in the PR comment.
- **Trade-offs:** Adds a small public-facing surface. Mitigated by signed-URL tokens, read-only access, no auth flow.

---

## Data Flow

### Repository registration (onboarding)

```
1. Maintainer installs the GitHub App on a repository
2. GitHub delivers `installation` webhook to Toolshed
3. Toolshed creates `repositories` row with default config (no triggers, default TTL, default max_concurrent)
4. Maintainer signs into the panel via GitHub OAuth
5. New repo appears in the panel with "Configure triggers to enable"
6. Maintainer fills in: triggers, lifecycle, env vars, compose file path
7. Configuration is persisted; subsequent matching events trigger deployments
```

### Deployment (happy path)

```
1.  Contributor opens / updates / labels a PR on GitHub
2.  Webhook delivered to Toolshed; HMAC-verified; persisted to `webhook_events`
3.  Bot loads the repository's configuration from DB
4.  Bot evaluates trigger rules (type, label name / command, fork status)
5.  Bot calls GitHub permissions endpoint for the actor (sender / commenter / PR author)
6.  Bot enforces `max_concurrent` (DB query)
7.  Bot enqueues a `deploy` job in River
8.  Bot posts initial PR comment ("Preview deploying...")
9.  Bot sets commit status: pending
10. Worker picks up job:
    a. Downloads tarball at PR head SHA via GitHub API
    b. Extracts to /var/lib/toolshed/workspaces/{job-id}/
    c. Reads docker-compose.preview.yml from the extract
    d. Sanitizes (rejects forbidden constructs)
    e. Injects: resource limits, Traefik labels, network, TOOLSHED_* metadata env vars, configured non-sensitive env vars
    f. Streams build output via `docker compose build` to Postgres + SSE channel
    g. Runs `docker compose up -d`
    h. Waits for health
    i. Records environment in DB (PR -> container, subdomain, TTL deadline)
    j. Removes workspace directory
11. Bot edits PR comment with preview URL + log viewer link
12. Bot sets commit status: success (or failure with logs link)
```

### Teardown (event-driven)

```
1. PR closed/merged webhook delivered
2. Bot enqueues teardown job
3. Worker: docker compose down, remove network, remove workspace
4. Worker: update DB (environment -> destroyed)
5. Bot edits PR comment ("Preview environment removed")
```

### Teardown (TTL cleanup)

```
1. River cron runs every 5 minutes
2. Selects environments with ttl_at < now AND status = active
3. Same teardown path as above
```

### Reconciliation (on bot restart)

```
1. Process starts
2. Query DB for all environments in state "active"
3. List Docker containers with Toolshed labels
4. For DB rows without containers: mark destroyed
5. For containers without DB rows: stop and remove (orphan cleanup)
6. For matching pairs: verify health, update if drifted
7. Re-enqueue any River jobs left in `running` state at crash time
8. Resume normal operation
```

### Manual runtime actions (panel or CLI)

```
1. Operator clicks "Redeploy" / "Teardown" / "Extend TTL" in panel,
   OR runs `toolshed env destroy <id>` from CLI
2. Action enqueues an appropriate River job (or updates DB directly for TTL)
3. Worker performs the action; PR comment updated; status check refreshed
4. Audit row written
```

---

## Integration Points

### GitHub App

Toolshed operates as a single GitHub App. All GitHub API access is performed using short-lived installation access tokens generated from the App's private key.

**Required permissions:**
- `contents: read` — tarball download
- `pull_requests: write` — PR comments
- `statuses: write` — commit status checks
- `metadata: read` — basic repo metadata

(Note: with config in the DB, `contents: read` is no longer required for config retrieval — only for tarball download. The original PRD's `actions: write` is dropped because `repository_dispatch` is no longer used.)

**Webhook subscriptions:**
- `pull_request` (opened, synchronize, closed, labeled)
- `issue_comment` (created — for `/deploy` style commands)
- `installation` (created, deleted — for repository registration lifecycle)

### GitHub OAuth

Used only for panel sign-in. Returns the user's identity; authorization is derived from live calls to the repository permissions endpoint.

### Traefik

Runs as part of the Toolshed compose stack. Uses the Docker provider via the socket proxy, watches container labels, applies routing and TLS automatically. Wildcard certificates via Let's Encrypt DNS-01 challenge.

**Supported DNS providers (MVP):** Cloudflare, AWS Route53.

### Docker daemon (host)

Accessed exclusively through `tecnativa/docker-socket-proxy`. Operations exposed: containers, networks, images, services-off, exec-off, commit-off.

### PostgreSQL

Co-located in the Toolshed compose stack. Holds: state tables, River queue tables, build log lines, audit log, webhook event archive.

---

## Scalability Considerations

### MVP scale (single VPS)

- Target: 1 maintainer, 1–5 registered repositories, 5–20 concurrent preview environments, well under 100 deployments/day.
- Bottleneck: Docker builds (CPU/disk).
- Mitigations: per-repo `max_concurrent`, server-wide resource caps, Docker layer caching.

### Vertical scaling

- Larger VPS (more vCPU / RAM / SSD) directly increases concurrent build capacity.
- Postgres benefits from RAM but is not the bottleneck.

### Horizontal scaling (post-MVP)

- **Worker separation:** River supports running workers in a separate process. Toolshed can be split into a webhook/API/panel process and worker processes pulling from the shared queue.
- **Multi-VPS workers:** Workers on additional hosts pull from the same Postgres. Requires shared image registry or rebuilt-per-host strategy.
- **Managed Postgres:** External Postgres can replace the co-located instance with no code changes.
- **Kubernetes target:** The Docker Engine SDK usage is contained behind a small interface; replacing it with a Kubernetes client to deploy preview environments as namespaced resources is the eventual path. Out of scope for MVP.

---

## Reliability & Fault Tolerance

### Webhook delivery

- GitHub retries failed webhook deliveries with exponential backoff.
- Handlers are idempotent: deduplicated by event ID stored in the `webhook_events` table.

### Job failures

- River provides configurable retry with exponential backoff.
- Permanent failures (invalid Dockerfile, sanitizer rejection) skip retry; transient failures (tarball timeout) retry up to a configured cap.
- Failed builds set commit status to `failure` and post error details with a log viewer link.

### Service restart recovery

- On startup, reconciliation compares DB state to running containers (orphans removed, stale rows marked destroyed, drifting pairs corrected).
- Jobs left in `running` state at crash time are re-enqueued.

### TTL-based cleanup

- A River cron runs every 5 minutes regardless of webhook delivery.
- Environments past TTL are torn down even if the `pull_request.closed` event was missed.
- Protects against orphan accumulation from missed webhooks or GitHub outages.

### Data consistency

- Postgres is the single source of truth for state.
- Docker container state is reconciled against the database on startup and periodically.
- Build log lines are written transactionally with job state updates.

---

## Security Considerations

### Authentication

- **GitHub App authentication:** Short-lived installation tokens, auto-rotated.
- **Webhook verification:** HMAC-SHA256 with shared secret; rejected if signature missing or invalid.
- **Panel sign-in:** GitHub OAuth flow; signed cookie sessions; access gated by live repo-permission check.
- **Log viewer:** Signed, time-limited URL tokens for read-only access from PR comments.

### Authorization

- Every trigger event is authorized server-side against live GitHub permission data.
- The actor checked depends on event type (sender / commenter / PR author).
- The mapping from GitHub permission level (`admin`, `maintain`, `write`, `triage`, `read`, `none`) to bot roles (`maintainer`, `collaborator`, `contributor`) is fixed.

### Build isolation

- Each preview environment runs in its own Docker network.
- Toolshed control-plane network is `internal: true`.
- Compose files sanitized: no privileged, no cap_add, no host bind mounts, no host pid/net/ipc, no devices.
- Resource limits (CPU, memory, PIDs) injected into every container.

### Forked PR security model

- Automatic triggers are hard-disabled for forks (PR author lacks `write+` on base).
- Forked PRs require explicit label or comment trigger from an authorized user.
- Authorization verified live, not trusted from webhook payload.

### Data protection

- No application secrets stored in MVP (only non-sensitive env vars).
- Server-level secrets (GitHub App private key, encryption master key, DNS provider credentials) live in `/etc/toolshed/server.yml` referenced by file path or injected via env vars; never hard-coded, never in DB.
- Build logs do not include the operator's GitHub OAuth tokens or installation tokens.

---

## Constraints

### Technical

- **Single VPS in MVP.** No multi-host workers, no Kubernetes.
- **Docker Compose only** as build/run target.
- **GitHub only.** No GitLab/Gitea/Bitbucket.
- **Wildcard domain required**, controlled by the operator.
- **DNS-01 ACME**, requires DNS provider API credentials (Cloudflare or Route53 in MVP).
- **No Git submodules or LFS** in MVP (tarball-based retrieval).
- **VPS sizing:** ≥4 GB RAM, ≥2 vCPU recommended.

### Product

- **Personal-scale tool first.** No effort spent on multi-tenant, marketplace, or polished public onboarding in MVP.
- **No paid features.** Self-hosted only.
- **Single maintainer** assumed; multi-operator coordination not designed for.

---

## Risks & Trade-offs

| Risk | Impact | Mitigation |
|---|---|---|
| Untrusted code execution during build | Malicious Dockerfile from fork can attempt container escape | Compose sanitization; resource limits; network isolation; rootless buildkit (post-MVP) |
| Docker socket compromise | Root-equivalent access to host | Socket proxy filtering; documented dedicated-VPS requirement |
| Build/runtime resource contention | Heavy build degrades running previews | Per-repo `max_concurrent`; server-wide resource caps |
| GitHub API rate limits | Webhook processing stalls during bursts | Installation tokens have 5,000 req/hr; typical use well under |
| Postgres single point of failure | DB outage stops orchestration | Standard Postgres reliability; `pg_dump` backups via cron; managed Postgres later |
| Disk exhaustion from accumulated Docker layers | VPS fills up | Periodic `docker system prune` cron; documented in ops notes |
| Configuration locked to DB (no git history) | No ready audit trail of past configs | In-app audit log table records all config changes (who, what, when) |
| MVP omits secret manager | Some preview workloads can't be modeled | Documented as known limitation; clear migration path defined |

### Architectural compromises accepted in MVP

- Panel and bot share one process. A panel-only crash takes down event handling.
- All preview environments share the host with the bot; a runaway preview can hurt the bot's responsiveness.
- No metrics or alerting. If something breaks silently, operator notices via missing PR comments.

---

## Out of Scope (MVP)

- Capabilities other than preview environments (autofix, triage, label sync, release automation).
- Plugin SDK for third-party extensions.
- Multi-VPS / worker-node architecture.
- Kubernetes deployment target.
- Managed cloud offering.
- Application secret management (encrypted secrets, fork approval gating).
- Custom domains per repository (wildcard only).
- GitLab / Gitea / Bitbucket support.
- Buildpacks or Dockerfile-less builds.
- Git submodules / LFS support.
- Per-repo CPU/memory caps (only server-wide defaults).
- Notification integrations beyond GitHub PR comments (Slack, email).
- Monorepo selective deployments.
- Audit log UI beyond a basic table.

---

## Future Considerations

### Short-term (post-MVP)

- **Application secret management:** encrypted_secrets table, NaCl encryption with master key in server.yml, panel forms with secret/non-secret toggle. Schema generalizes from current env-vars table.
- **Additional plugins:** label sync, comment-driven triage, release-notes assist. Each as a new package in `internal/plugins/`, registered through the existing `Plugin` interface.
- **Audit log UI** in the panel.
- **Docker layer cache management:** automated pruning, cache reuse across PRs of the same repo.
- **Rootless build runtime:** buildkit-rootless or Podman for stronger isolation of fork-PR builds.

### Medium-term

- **Worker process separation:** webhooks/API/panel as one process, workers as another, sharing the Postgres queue. Required precursor to multi-host.
- **Agent-driven plugins:** e.g. autofix that runs an LLM agent on a flagged issue. Agent runtimes are heavier than current jobs and likely warrant being moved to dedicated worker processes.
- **Multi-host workers:** worker nodes on additional VPSes pulling from the shared River queue.
- **Server-level guardrails:** `global_max_concurrent`, `global_max_ttl`, blocklists in server.yml that override per-repo settings.

### Long-term

- **Kubernetes runtime:** replace Docker Engine SDK behind the existing interface with a Kubernetes client; preview environments become namespaced resources with NetworkPolicy isolation.
- **Multi-tenant evolution:** if the project grows beyond personal use, real user/org model, per-tenant resource isolation, billing.
- **VCS provider abstraction:** GitLab / Gitea support behind a provider interface.
- **Managed cloud offering:** architecturally identical to self-hosted ("our hosted instance of your self-hosted tool").
