# prout - Architecture Decision Records

This document collects the key architectural decisions made before starting MVP implementation. Format: Michael Nygard ADR (Status / Context / Decision / Consequences).

Each ADR has one status: **Accepted** (decided), **Deferred** (intentionally postponed), **Rejected** (considered and rejected).

---

## ADR-001: Implementation Language - Go

**Status:** Accepted

**Context:**
The PRD points to Go as the stack, but that choice had not been validated. The operator has solid experience in both Go and TypeScript/Node.js. There are two real motivations: (1) prout must work as a tool for the operator's TypeScript OSS project, (2) the operator wants to move into platform engineering as a growth direction and sees Go as the go-to language in that niche.

**Decision:**
Implement in Go.

**Consequences:**

Three independent vectors point in the same direction:

1. **Technical fit for the project.** prout is literally what Go was designed for: webhook receiver + worker pool + Docker SDK + Postgres + CLI. Single binary, static compilation (`CGO_ENABLED=0`), low memory usage, mature concurrency.

2. **Cloud-native ecosystem.** Kubernetes, Docker, Prometheus, Terraform, Crossplane, Argo, Flux, Tekton, Buildkit - roughly 95% of CNCF projects are in Go. Native libraries for Docker Engine SDK, GitHub App auth (`google/go-github` + `bradleyfalzon/ghinstallation`), and the Kubernetes client (`client-go`) are first-class.

3. **The operator's growth goal.** Platform engineering is a niche where Go dominates for good reason. prout is a good excuse-project because its domain problem fits the language perfectly - learning Go happens along the way, not at the expense of the product.

TypeScript/Node.js would be feasible, but it would fight the problem (event loop for CPU-bound builds, weaker typing when interacting with the Docker API, no native process management for long-running workers).

This decision accepts the consequence that the frontend will also be in Go (`templ`) - see ADR-003.

---

## ADR-002: No Plugin Interface in MVP

**Status:** Accepted (intentional deviation from the PRD)

**Context:**
The PRD assumes a plugin interface (`Name() string`, `Subscribe() []EventType`, `Handle(ctx, event) error`) with one plugin (preview) in MVP, to prepare the architecture for future plugins (autofix, triage, label sync). The argument is: "the interface exists so future plugins can register without touching the core."

**Decision:**
No formal plugin interface in MVP. Preview is part of the core, in the `internal/preview` package. The webhook handler calls preview functions directly (`preview.HandlePullRequest(ctx, event)`), without an abstraction layer.

**Consequences:**

Three realistically considered future plugins have different requirement shapes:

- **Autofix** (LLM agent on an issue) - event-driven *plus* long-running and stateful. Is `Handle(ctx, event) error` enough?
- **Triage** - event-driven *plus* cross-event state ("did this user already get an answer to a similar issue 3 days ago?").
- **Label sync** - *declarative and periodic*, not event-driven. `Subscribe() []EventType` does not support that.

Three plausible plugins -> three different interface shapes. A plugin interface designed today for hypothetical future plugins will most likely be wrong for those plugins when they actually start to exist. Classic Fowler "Rule of Three": do not abstract before there are three instances.

**Plus:** the cost of extracting a shared abstraction later from preview + a second plugin is low - refactoring one package, not restructuring the system.

**Minus:** if it turns out the PRD was right, there is one extra refactoring iteration. Acceptable.

---

## ADR-003: Panel Stack - templ + HTMX + Tailwind Standalone

**Status:** Accepted (intentional deviation from the PRD)

**Context:**
The PRD specifies an SSR panel with `html/template` (stdlib) + Tailwind CDN + SSE. The argument: no build pipeline, no React, easier maintenance. All true, but `html/template` has significant drawbacks (runtime errors instead of compile-time, no typing for template data, no IntelliSense, string-based composition).

**Decision:**
- **Templates:** [`templ`](https://templ.guide) - type-safe templates compiled into Go functions.
- **Interactivity:** [HTMX](https://htmx.org) - hypermedia-driven partial updates without an SPA.
- **CSS:** Tailwind standalone CLI (one binary file, no Node.js).
- **Live updates:** SSE (same as in the PRD).

All assets are embedded into the binary via `embed.FS`. Single binary, zero `npm install`.

**Consequences:**

**Pros vs. the PRD:**
- Compile-time checking of templates - refactoring a struct field causes a compiler error in every template that uses that field
- Full IntelliSense in the IDE (because templ compiles to Go)
- Component composition like React, but fully SSR
- HTMX provides partial updates without an SPA - panel interactivity grows naturally
- Tailwind standalone (without Node) removes the `node_modules` supply chain

**Minuses / costs:**
- One extra build step: `templ generate` (analogous to `sqlc generate`)
- IDE plugin required for `.templ` syntax highlighting (available for VSCode and GoLand)
- Smaller community than `html/template`, but the fastest-growing one in Go webdev

This decision assumes the panel will have >=10 views and will evolve - in that scenario type safety wins decisively.

---

## ADR-004: Job Queue - PostgreSQL + River

**Status:** Accepted (aligned with the PRD)

**Context:**
Webhook handlers must respond in <10s (GitHub timeout), but building a preview environment takes 1-5 minutes. An asynchronous job queue is required, with retries, periodic jobs, and observability.

Considered:
- Postgres + River (PRD)
- Postgres + asynq (Redis-based)
- Temporal (workflow engine)
- Manual implementation on Postgres

**Decision:**
PostgreSQL + [River](https://riverqueue.com).

**Consequences:**

**Why not a Redis-based queue (asynq):** it adds a second storage backend and removes transactional coupling between state (`environments` table) and jobs. Redis throughput is unattainable and unnecessary for personal scale anyway.

**Why not Temporal:** Temporal is a *workflow engine*, not a job queue - different classes of tools. prout is not workflow-shaped, because GitHub *is* the external orchestrator (webhook events push the system through states). A second workflow layer would be redundant. Operationally, Temporal is 5-7 processes + Cassandra/Postgres + Elasticsearch - overkill for a single-VPS personal-scale tool.

Temporal is a good option *later* for:
- The autofix plugin (an LLM agent is workflow-shaped)
- A separate project for learning a workflow engine

**Why River:**
- Postgres-native (`SELECT FOR UPDATE SKIP LOCKED`), no new storage
- Type-safe Go API
- Built in: retries, exponential backoff, periodic jobs (cron), leader election, web UI for monitoring
- Transactional coupling of state + jobs (one transaction: update `environments` + insert job)
- Actively developed (Brandur Leach), v1.0+, stable

**What River handles out of the box for prout:**
1. Deploy job with retry on transient failures (tarball download timeout, GitHub API rate limit)
2. Teardown job (idempotent)
3. Periodic TTL cleanup every 5 minutes (River cron)
4. Reconciliation on startup via `client.JobInsert`

**What River does NOT handle (and should not):**
- Streaming build logs - that is not a job, it is a stream. Implemented via Postgres `LISTEN/NOTIFY` + SSE in the panel.
- Canceling a running job from an external trigger (for example PR closed during build) - requires custom logic (deploy checks before committing to DB whether the PR is still open + teardown snoozes when deploy is running).

---

## ADR-005: Docker Integration - subprocess `docker compose` + Engine SDK

**Status:** Accepted (PRD refinement)

**Context:**
The PRD points to "Docker Engine SDK for Go" as the mechanism, but at the same time assumes that prout uses the repo's `docker-compose.preview.yml`. These are technically separate things. Considered:

- A: subprocess `docker compose` + Engine SDK for surrounding operations (target state)
- B: parsing Compose + using Engine SDK directly (reimplementing Compose)
- C: Compose v2 as a Go library (`github.com/docker/compose/v2`)
- D: Dockerfile only, no Compose (crossed out - makes previews with dependencies such as DB impossible)

**Decision:**
Subprocess `docker compose` for build and deploy. Engine SDK for operations around it (listing containers by labels, network inspect, container stats, events).

**Consequences:**

**What goes through subprocess `docker compose`:**
- `docker compose -f ... -p prout-pr-{n} up -d --build` (deploy)
- `docker compose -f ... -p prout-pr-{n} down -v` (teardown)
- Streaming `cmd.Stdout`/`cmd.Stderr` as `io.Reader` -> lines to Postgres + SSE channel

**What goes through Engine SDK (`github.com/docker/docker/client`):**
- `ContainerList` with label filters (reconciliation, dashboard)
- `NetworkInspect` (network health)
- `ContainerStats` (future: CPU/memory monitoring)
- `Events` API for live status updates

**Why not Compose-as-library:** it pulls in half of Docker as a dependency, increases build time from 30s to 3 minutes, and expands the supply-chain surface. Stable CLI > unstable embedded library.

**Operational implications:**
- In the prout container: install `docker-cli` + `docker-compose-plugin` (Alpine: `apk add docker-cli docker-cli-compose`)
- `DOCKER_HOST=tcp://docker-socket-proxy:2375` as an env var - read by both Engine SDK (`client.FromEnv`) and the `docker compose` CLI
- In the [Tecnativa docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) configuration: `CONTAINERS=1`, `NETWORKS=1`, `IMAGES=1`, `VOLUMES=1`, `EXEC=1` (for health checks), `BUILD=1`. Everything else `=0`.

**K8s escape hatch:** Engine SDK sits behind a small interface (`internal/runtime`), so replacing it with a Kubernetes client in the future is a swap of one package implementation, not a system restructure. The subprocess `docker compose` path must be replaced with custom orchestration (Helm chart? Kustomize? CRD?) - that is a larger change.

---

## ADR-006: Panel Auth - GitHub OAuth + `allowed_users` in DB + Add-User UI

**Status:** Accepted (intentional deviation from the PRD)

**Context:**
The PRD assumes GitHub OAuth with a live permission check against the GitHub API on every request. The operator decided they want support for multiple users (a team of 2-3 people around OSS), but does not want a full invitation flow in the style of Dokploy (3-4 weeks of implementation: users table, password hashing, email integration, invitation tokens, organizations, roles, RBAC, ...).

Four scenarios were considered:
- A: Solo operator, admin token in `server.yml`
- B: Team of 2-3 people, GitHub OAuth + `allowed_users` in `server.yml` (SSH editing)
- **B+: Team of 2-3 people, GitHub OAuth + `allowed_users` in DB + add-user UI**
- D: Hosted SaaS-style multi-tenant (Dokploy-grade)

**Decision:**
Variant **B+ minimal**.

**Schema:**
```sql
CREATE TABLE allowed_users (
  github_username TEXT PRIMARY KEY,
  added_at TIMESTAMP NOT NULL DEFAULT NOW(),
  added_by TEXT NOT NULL,        -- operator's github_username
  is_owner BOOLEAN NOT NULL DEFAULT FALSE
);
```

Bootstrap the owner from `server.yml` (`bootstrap_owner: "github_username"`) on first start - that record gets `is_owner=true`.

**Flow:**
1. The owner logs in via GitHub OAuth and gets into the panel
2. Settings -> Users -> Add user -> enters a GitHub username
3. prout validates that the user exists (`GET /users/{username}`), then inserts into `allowed_users`
4. The owner sends the panel URL to a teammate, the teammate clicks "Sign in with GitHub", and gets in

**Consequences:**

**Pros vs. pure B (`server.yml`):**
- No SSH access required to add users
- Auditability (`added_by`, `added_at`)
- The owner can remove a user (DELETE) without restart

**Pros vs. a full invitation flow:**
- No `users` table (`github_username` is a natural unique key)
- No password hashing, no email integration
- No invitation tokens or token expiry
- No forgot-password flow
- Implementation: ~3-4 days vs. ~3 weeks

**Intentional limitations:**
- No roles (every allowed user has full access except adding users). Future migration: `ALTER TABLE allowed_users ADD COLUMN role TEXT`.
- No self-service signup
- Requires every team user to have a GitHub account (acceptable for OSS)

**Independent from panel auth - trigger authorization stays as in the PRD:**

Authorization for actions triggered by a contributor on a PR (for example `/deploy` via comment) still uses a **live GitHub permission check** (`GET /repos/{owner}/{repo}/collaborators/{username}/permission`). These two systems are independent:
- **Panel auth** = "who can enter the prout UI"
- **Trigger auth** = "who can trigger a deploy on a specific PR"

---

## ADR-007: Code Retrieval - Tarball

**Status:** Accepted (aligned with the PRD)

**Context:**
The preview environment must receive the application code at a specific SHA. Options:
- Tarball from the GitHub REST API (`GET /repos/{owner}/{repo}/tarball/{sha}`)
- `git clone --depth 1` with installation token in the URL
- Hybrid

**Decision:**
Tarball.

**Consequences:**

**Pros:**
- Atomic (single SHA, no race condition)
- Lightweight (no `.git` history)
- Works identically for forks and the main repo (same installation token)
- No SSH key plumbing
- Less Go code (HTTP GET + tar/gzip extraction)

**Minuses / limitations:**
- No `.git` directory - builds that use `git rev-parse HEAD` / `git describe` in the pipeline will not work
- No submodules (the tarball does not include submodule contents)
- No Git LFS
- Whole-repo tarball - wasteful for monorepo previews and can load gigabytes into the build context

**Validation for the first use case:**
The operator's TypeScript project - no submodules, no git-dependent build scripts, not a monorepo. Tarball is sufficient.

**Migration path:**
If future repos require `.git` or submodules, add a per-repo config `clone_with_git: bool` and a second code path with `git clone`. Do not do it preemptively - two code paths, one of them untested, is a worse state than one.

**Implementation detail:**
GitHub tarballs have a wrapping directory `{owner}-{repo}-{shortsha}/` - the extraction logic must handle that (strip the first path component or locate `docker-compose.preview.yml` as the workspace root).

---

## ADR-008: Application Secrets - Plain Text Env Vars + Auto-injected `prout_*`

**Status:** Accepted (PRD refinement, "Strategy 1.5")

**Context:**
Preview environments need env vars: some dynamic per PR (`PUBLIC_URL`), some static (`NODE_ENV`), some "test secrets" for API providers (Stripe test mode, OpenAI low-limit key, Resend sandbox).

The PRD chooses: no secret manager in MVP, only non-sensitive env vars. A full secret manager (encrypted, fork approval gating) is postponed until after MVP.

**Decision:**

Three layers of env vars:

1. **Auto-injected by prout** (read-only, visible in the panel as a preview):
   ```
   prout_PR_NUMBER=42
   prout_PR_URL=https://github.com/.../pull/42
   prout_COMMIT_SHA=deadbeef
   prout_PREVIEW_URL=https://pr-42.preview.example.com
   prout_PREVIEW_DOMAIN=pr-42.preview.example.com
   ```

2. **Per-repo env vars** edited in the panel (list of `KEY=value` pairs), stored in Postgres as **plain text**:
   ```sql
   CREATE TABLE repository_env_vars (
     repository_id BIGINT NOT NULL REFERENCES repositories(id),
     key TEXT NOT NULL,
     value TEXT NOT NULL,
     created_at TIMESTAMP NOT NULL DEFAULT NOW(),
     updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
     PRIMARY KEY (repository_id, key)
   );
   ```

3. **Compose-defined** (left untouched) - values in the repo's own `docker-compose.preview.yml`.

prout injects (1) and (2) into Compose at deploy time (`--env-file` or direct modification of `services.<name>.environment`).

**UI disclaimer shown in the panel:**
> Warning: Env vars are stored in plain text. Use only test/sandbox API keys (Stripe test mode, Resend sandbox, low-limit OpenAI keys), never production secrets.

**Consequences:**

**Why plain text and not encryption-at-rest:**

1. Postgres lives on the same host as prout. An attacker with DB access already has access to the host, which means access to the Docker socket, GitHub App private key, and Cloudflare API token. Encryption-at-rest for a DB on the same host protects against a threat model that does not really exist here.

2. Encryption-at-rest makes sense in scenarios where backups might leak (a separate problem - encrypted backups with a separate key), the DB is hosted externally (managed Postgres), or compliance applies (HIPAA, PCI). None of that applies to prout on a single VPS.

3. The real threat for "secrets" in prout is test secrets. A leak means rotating the key in the provider panel, not losing data.

4. Encryption adds surface area: master key management, key rotation, "what if you lose the key", boundary management between encrypted and unencrypted. Those costs are not worth it for test secrets.

**Migration path to encrypted secrets (post-MVP):**

The schema generalizes cleanly:
```sql
ALTER TABLE repository_env_vars ADD COLUMN is_secret BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE repository_env_vars ADD COLUMN encrypted_value BYTEA;
-- at runtime: if is_secret=true, decrypt encrypted_value instead of value
```

The UI gets a toggle: "this is a secret" on add/edit. Master key in `server.yml`. NaCl `crypto/secretbox`.

---

## ADR-009: DNS / TLS - Cloudflare + Let's Encrypt DNS-01 + Wildcard

**Status:** Accepted (aligned with the PRD)

**Context:**
Each PR needs a unique URL: `pr-{n}.preview.example.com`. That requires:
- Wildcard DNS A record (`*.preview.example.com -> IP_VPS`)
- Wildcard TLS certificate (Let's Encrypt will not issue a wildcard cert via HTTP-01 - DNS-01 is required)
- A DNS provider with API support for the DNS-01 challenge

The PRD lists Cloudflare and Route53 as supported in MVP. The operator already has the domain on Cloudflare.

**Decision:**
- **DNS provider:** Cloudflare
- **ACME challenge:** DNS-01 via Traefik
- **Certificate:** wildcard `*.preview.example.com`
- **Reverse proxy:** Traefik v3 with Docker provider

**Consequences:**

**Operator setup (one time):**
1. Cloudflare API Token with the "Edit zone DNS" scope only for the target zone
2. Token in `server.yml`: `acme.cloudflare_api_token`
3. Wildcard A record `*.preview.example.com -> IP_VPS` with proxy **off** (gray cloud - Cloudflare proxy does not work with DNS-01)
4. First prout start: Traefik issues the wildcard cert (~30s)

**Per-PR routing:**
prout injects Traefik labels into the exposed service in Compose:
```yaml
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.pr-42.rule=Host(`pr-42.preview.example.com`)"
  - "traefik.http.routers.pr-42.entrypoints=websecure"
  - "traefik.http.routers.pr-42.tls.certresolver=letsencrypt"
  - "traefik.http.services.pr-42.loadbalancer.server.port={port_from_panel}"
  - "traefik.docker.network=prout-traefik"
```

**Per-repo config in the panel:**
- Exposed service name (`web`, `app`, ...)
- Exposed port (`3000`, `8080`, ...)

**Intentional limitations:**
- Only one exposed service per PR in MVP. Multi-service previews (`api-pr-42` + `pr-42`) are postponed until after MVP.
- Cloudflare proxy for the operator's *main* domain can stay enabled - the restriction only applies to the preview subdomain zone.
- Local development without real DNS is awkward (Let's Encrypt will not issue a cert for localhost). The walking skeleton runs on the VPS from day one, not locally.

**Common docs mistake:**
"ACME challenge fails" -> 9 times out of 10, Cloudflare proxy is on for the preview zone. Troubleshooting documentation should start there.

---

## ADR-010: Database Migrations - goose CLI-only + Version Check on Startup

**Status:** Accepted (aligned with the PRD, refinement)

**Context:**
The PRD points to goose. The second decision is whether migrations should run automatically at application startup or manually through the CLI.

**Decision:**

- **Tooling:** [goose](https://github.com/pressly/goose) with embedded migrations (`embed.FS`)
- **Mode:** CLI-only - the operator manually runs `prout migrate up` before restart
- **Safety net:** on startup the application checks whether the current schema version is >= target and fails fast with a clear instruction if not

```go
func main() {
    db := connectDB()

    current, _ := goose.GetDBVersion(db)
    target := getEmbeddedTargetVersion()

    if current < target {
        log.Fatal("schema out of date",
            "current", current, "target", target,
            "action", "run 'prout migrate up' before starting")
    }
    if current > target {
        log.Warn("schema ahead of binary - running old binary against new schema")
    }
    startServer()
}
```

**River migrations:** included inside prout migrations as a Go-based goose migration:
```go
// migrations/00X_river_init.go
func upRiver(ctx context.Context, tx *sql.Tx) error {
    migrator := river.NewMigrator(...)
    return migrator.Migrate(ctx, river.MigrationDirectionUp, ...)
}
```
One command (`prout migrate up`) also migrates River's tables. Updating the River dependency -> a new migration automatically.

**Consequences:**

**Operator trade-off:** explicitness intentionally chosen over lower friction. Every prout update = SSH + 2 commands instead of 1.

**Plus:**
- Full control over migration timing
- Ability to test migrations on a DB snapshot before production
- No race condition when two processes start (an advisory lock would also solve this, but we do not need it)
- No surprise rollback (`docker compose pull` + restart of the old version does not silently start against the new schema)

**Minus, mitigated by the version check:**
- Without the application's safety net there would be silence + runtime SQL errors. With the safety net -> fail fast with instructions.

**Migration convention:**
- **Forward-only in production.** Down migrations are optional, used only in development during iteration.
- Fix broken migrations with a forward fix (the next migration), not with `down`.

**CLI:**
```
prout migrate up              # apply pending
prout migrate down            # one step (dev)
prout migrate status          # current vs target
prout migrate create <name>   # generate template
```

---

## ADR-011: Way of Working - Walking Skeleton

**Status:** Accepted (process, not technology)

**Context:**
prout has a large surface area (GitHub App, webhooks, OAuth, River, Docker, Traefik, templ, Postgres, Cloudflare). Possible approaches:
- Everything at once in full form (~6 weeks to MVP, no feedback loop during the build)
- Bottom-up (layers: schema -> store -> handlers -> workers -> panel) - for the first 3-4 weeks nothing works end to end
- **Walking skeleton** - end to end from day one, but with mocks; each iteration replaces mocks with a real implementation

**Decision:**
Walking skeleton. 7 weekly iterations:

| Week | Goal | Smoke test |
|---|---|---|
| 1 | End-to-end skeleton with mocks: HTTP server, one webhook endpoint, River with `EchoJob`, Postgres + goose, docker compose stack | curl webhook -> log in worker |
| 2 | GitHub App + real webhook (HMAC validation, persist events to `webhook_events`) | open PR on test repo -> event in DB |
| 3 | Tarball download (installation token, GitHub API, extract to workspace) | PR opened -> workspace with code |
| 4 | `docker compose up -d`, sanitization, hardcoded port | PR opened -> container runs under IP:port |
| 5 | Traefik + TLS + Cloudflare + PR comment with URL | PR opened -> GitHub comment with working https URL |
| 6 | Panel skeleton (templ + HTMX + GitHub OAuth + `allowed_users`) | login, preview list, manual teardown |
| 7+ | Build logs to Postgres + SSE, TTL cleanup, reconciliation, audit, per-repo config UI | refinement |

**Consequences:**

**Why walking skeleton:**
1. **Real integration from week one.** The first real webhook always uncovers something design did not predict (timing, ordering, retry edge cases).
2. **Every iteration is demo-able.** Motivating for an OSS project without a deadline.
3. **Go idioms are learned wide, not deep.** Week 1 touches HTTP + DB + worker + CLI in trivial form. Weeks 2-6 are *deepening*, not introducing brand-new concepts.
4. **Architectural decisions validate against real code**, not against hypotheses.

**Required discipline:**
- Do not go deep before going wide. Week 4 should not implement full Compose sanitization - only the absolute minimum needed for the smoke test to pass. Full sanitization belongs in the "refinement" iteration.
- Mocks must look like real interfaces (so substitution in the following week is only an implementation change). Mock GitHub API returns `*github.PullRequest`, not `map[string]any`.

**Intentional consequences:**
- In weeks 1-3 the product is "junk" in terms of features, but "complete" in terms of pipeline. That is a feature, not a bug.
- In week 7+ there will still be a "still to do" list. That is okay - most OSS tools live in that state permanently.

---

## ADR-012: Intentionally Postponed (Deferred)

**Status:** Deferred - not forgotten, intentionally postponed with migration paths

**Context:**
The PRD and our discussion identified several capabilities that would be valuable long term, but whose cost vs. MVP value is poor.

**Postponed list:**

### A. Plugin interface
- **Revisit trigger:** When there are 2-3 real plugins to abstract
- **Migration path:** Refactor the `internal/preview` package and new packages into a shared `internal/plugins/` abstraction extracted from real needs
- **See:** ADR-002

### B. Encrypted secret manager
- **Revisit trigger:** When previews require production-grade secrets (full Stripe live API key, real GitHub token)
- **Migration path:** `ALTER TABLE repository_env_vars ADD COLUMN is_secret BOOLEAN, encrypted_value BYTEA`. Master key in `server.yml`, NaCl `crypto/secretbox`. UI toggle "is secret".
- **See:** ADR-008

### C. Roles / RBAC
- **Revisit trigger:** When the team grows and requires separation between "viewer" and "operator"
- **Migration path:** `ALTER TABLE allowed_users ADD COLUMN role TEXT NOT NULL DEFAULT 'admin'`. Middleware checks role per endpoint.
- **See:** ADR-006

### D. Multi-VPS workers
- **Revisit trigger:** When build CPU contention becomes a real problem
- **Migration path:** River supports workers in separate processes. Split prout into processes: webhook/API/panel + workers (separate binary). Workers on other VPSes connect to the same Postgres. Requires a shared image registry or per-host rebuild.
- **See:** PRD "Scalability Considerations"

### E. Kubernetes runtime
- **Revisit trigger:** When the operator decides Docker Compose is not enough (multi-host, network policies, autoscaling)
- **Migration path:** Engine SDK + subprocess `docker compose` replaced with Kubernetes client (`client-go`) and an operator deploying previews as namespaced resources
- **See:** ADR-005, ADR-001 (Go makes this migration easier - `client-go` is native)

### F. Submodules / monorepo / Git LFS support
- **Revisit trigger:** A concrete project requires it
- **Migration path:** Per-repo config `clone_with_git: true` + `git clone --depth 1` as an alternative code path
- **See:** ADR-007

### G. Per-repo CPU / memory caps
- **Revisit trigger:** When server-wide caps are too coarse
- **Migration path:** Add columns to the `repositories` table, read them when injecting limits

### H. Audit log UI
- **Revisit trigger:** When inspection of "who changed what" is needed
- **Migration path:** `audit_log` table already exists (ADR-006 + ADR-008 generate entries). Add a view in the panel.

### I. Notification integrations (Slack, email)
- **Revisit trigger:** When a PR comment is not enough
- **Migration path:** Plugin-style, but implemented *after* ADR-002 is reactivated

### J. Multi-tenant / hosted offering
- **Revisit trigger:** Product pivot
- **Migration path:** The architecture already has `repository_id` foreign keys (mentioned in the PRD). Add `organization_id` as another layer + scoped queries. But realistically: this is a different product, not an incremental upgrade.

---

## Appendix A: Stack in One Table

| Category | Choice | ADR |
|---|---|---|
| Language | Go | ADR-001 |
| HTTP routing | `chi` or stdlib mux | - |
| Templates | `templ` | ADR-003 |
| Frontend interactivity | HTMX | ADR-003 |
| CSS | Tailwind standalone CLI | ADR-003 |
| Live updates | SSE | ADR-003 |
| DB | PostgreSQL 16 | - |
| DB access | sqlc + pgx | - |
| Migrations | goose, CLI-only | ADR-010 |
| Job queue | River | ADR-004 |
| GitHub client | `google/go-github` + `bradleyfalzon/ghinstallation` | - |
| Docker (build/deploy) | subprocess `docker compose` | ADR-005 |
| Docker (introspection) | Engine SDK | ADR-005 |
| CLI framework | `cobra` | - |
| Logging | `slog` (stdlib) | - |
| Reverse proxy | Traefik v3 | ADR-009 |
| TLS | Let's Encrypt DNS-01 | ADR-009 |
| DNS provider | Cloudflare | ADR-009 |
| Auth (panel) | GitHub OAuth + `allowed_users` | ADR-006 |
| Auth (triggers) | Live GitHub permission API | ADR-006 |
| Code retrieval | Tarball | ADR-007 |
| Env vars | Plain text DB + auto-injected `prout_*` | ADR-008 |
| Plugin interface | **None in MVP** | ADR-002 |

## Appendix B: Weekly Plan (walking skeleton)

| Week | Goal |
|---|---|
| 1 | Mock end-to-end skeleton |
| 2 | Real GitHub webhook |
| 3 | Tarball download + workspace |
| 4 | `docker compose up` |
| 5 | Traefik + TLS + PR comment |
| 6 | Panel skeleton + auth |
| 7+ | Refinement (logs, TTL, reconciliation, audit, config UI) |

See ADR-011.
