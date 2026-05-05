# 🧰 prout — Product Requirements Document

**GitHub Automation Bot — Self-Hosted, Personal-Scale**
_Version 2.0 · MVP Scope · April 2026_

---

## Problem Statement

Maintaining a personal open source project on GitHub involves a steady stream of small repetitive operational tasks: spinning up preview environments to review pull requests, applying labels, triaging issues, automating release notes. Each task individually is minor; collectively they consume time that should be going into the project itself.

The available solutions don't fit personal-scale OSS work:

- **Managed services** (Vercel, Render, Railway) solve preview environments well, but at a cost and lock-in profile that doesn't make sense for a side project. They also stop at preview environments — they're not a general automation surface.
- **GitHub Actions** can do a lot of automation work, but become complex to write, fragile to maintain, and have hard limitations: secret access restrictions for forked PRs make preview deployments fundamentally awkward, and long-running workflows (interactive automation, agent-driven tasks) don't fit the per-job execution model.
- **Self-hosted deployment platforms** (Dokploy, Coolify) are persistent deployment platforms, not automation bots. They're sized for production workloads with team management, RBAC, and multi-tenant features — overkill and wrong-shaped for "automate the small stuff on my own repos."
- **Probot-style bots** are closer in spirit but require building each bot from scratch as a separate Node.js application. There's no shared substrate for "stand up automation across my repos."

The result for a solo OSS maintainer: either skip the automation entirely and absorb the toil, or invest disproportionate engineering effort building one-off scripts and Actions workflows for each task.

---

## Solution

prout is a self-hosted GitHub automation bot built primarily for the maintainer's own open source projects, with an architectural path to evolve into a more general-purpose tool over time.

The MVP scope is narrow and concrete: **orchestrate ephemeral preview environments for pull requests**, triggered by GitHub events (PR open/synchronize/close, labels, comments). This is the first capability — the one with the highest day-to-day impact for OSS review workflows. Future capabilities (label sync, triage automation, agent-driven autofix, release-notes assist) are anticipated but explicitly out of MVP scope.

The product takes design inspiration from Probot (lightweight, GitHub-native bot pattern) and Prow (config-driven behavior with a built-in model for capabilities), positioning itself between them: more opinionated than a framework, lighter than a Kubernetes-scale CI system.

A maintainer installs prout on a single VPS using `docker-compose up`, points a GitHub App at it, and configures registered repositories through a minimal web panel. The first PR opened against a registered repository results in a unique HTTPS URL posted as a PR comment. When the PR closes, the environment is torn down automatically.

prout is deliberately not a public OSS product polished for arbitrary external operators. It is designed for the maintainer's own use, with the architectural shape that doesn't preclude broader use later.

---

## Target Users

### Primary — The Maintainer (Single Operator)

The author of this document, maintaining a personal TypeScript-based open source project. Comfortable with Docker, YAML, GitHub Apps, Postgres, and Go. Has access to a VPS, a domain at Cloudflare, and roughly 5–20 PRs open across personal repos at any time. Wants to invest a few weeks of focused work to reclaim recurring time spent on automation toil.

### Secondary — Co-Maintainers and Trusted Collaborators

Two to three people who collaborate on the maintainer's OSS projects. They need read/write access to the panel to monitor preview environments, redeploy on demand, or investigate failures. They don't need self-service onboarding — the maintainer adds them by GitHub username.

### Anticipated Later (Not MVP) — Other Personal-Scale Operators

Other solo or small-team OSS maintainers running their own infrastructure who might find prout useful for their own setups. The architecture leaves room for this, but no MVP effort is spent on polished onboarding, multi-tenant support, or marketplace concerns.

---

## User Stories

### Onboarding & Setup

1. As the operator, I want to deploy prout on my VPS using a single `docker-compose up` command, so that I can get the system running without complex infrastructure setup.
2. As the operator, I want to provide system-level configuration (wildcard domain, GitHub App credentials, Cloudflare API token, encryption keys, defaults) once at install time via a config file at `/etc/prout/server.yml`, so that bootstrapping is explicit and version-controllable on my host.
3. As the operator, I want to configure the bootstrap owner (my own GitHub username) in `server.yml`, so that on first start I'm automatically added as the panel owner without an out-of-band setup step.
4. As the operator, I want prout to register a repository automatically when I install the GitHub App on it, so that no separate registration step is required.
5. As the operator, I want repositories to start in a "configured but disabled" state after registration, so that no triggers fire until I deliberately enable them.

### Repository Configuration (Web Panel)

6. As the operator, I want to configure each repository's behavior — triggers, lifecycle (TTL, max concurrent), environment variables, exposed service/port, compose file path — through the web panel, so that I can iterate on configuration without git roundtrips.
7. As the operator, I want configuration changes to apply to subsequent triggers immediately without requiring a server restart, so that I can correct misconfigurations in seconds.
8. As the operator, I want to define which trigger types are active per repository (automatic on push, label-based, comment-based), so that I can apply different policies to different projects.
9. As the operator, I want the `docker-compose.preview.yml` file describing the preview stack to remain in the target repository, so that the build definition stays co-located with the code it builds.

### Trigger Authorization

10. As the operator, I want automatic preview deployments for PRs from non-fork branches to be opt-in per repository, so that I have explicit control over what runs without intervention.
11. As the operator, I want forked PRs to require either a label or a `/deploy` comment from an authorized user before deploying, regardless of the repository's automatic trigger setting, so that untrusted code is never auto-deployed.
12. As the operator, I want trigger authorization checks to use live GitHub permission data for each event, so that revoking someone's access on GitHub takes effect immediately.
13. As the operator, I want unauthorized trigger attempts to be silently ignored and recorded in the audit log, so that bad actors cannot trigger work and I have a record of attempts.

### Preview Environment Lifecycle

14. As a contributor opening a PR, I want a comment posted on my PR with the preview URL once the environment is ready, so that I and reviewers can immediately access the live preview.
15. As a contributor, I want a GitHub commit status check that reflects deployment state (pending, success, failure with link to logs), so that I can track the deployment without leaving the PR page.
16. As a reviewer, I want each PR to have its own subdomain (`pr-{number}.preview.example.com`), so that multiple PR previews can coexist.
17. As the operator, I want a new commit pushed to an open PR to trigger a redeploy that replaces the existing environment, so that the preview always reflects the latest code.
18. As the operator, I want environments to be torn down automatically when a PR is closed or merged, so that I don't accumulate orphans.
19. As the operator, I want environments to be torn down automatically after their TTL expires, so that idle environments don't consume host resources indefinitely.
20. As the operator, I want TTL-based cleanup to run on a schedule independent of GitHub webhook delivery, so that missed close events don't leave orphans.

### Build & Deployment

21. As the operator, I want prout to download the PR's code at the exact head commit (including from forks) and build the application using `docker compose`, so that the preview matches the project's own build definition.
22. As the operator, I want non-sensitive environment variables (configured per-repository in the panel) and auto-injected `prout_*` variables (PR number, preview URL, commit SHA) to be available to the preview application, so that the app can adapt its behavior without me hand-wiring each value.
23. As the operator, I want build logs streamed to the panel in real time and persisted in the database, so that I can diagnose failures from the web UI rather than SSH.
24. As the operator, I want failed builds to report failure status on the PR with a link to log output, so that contributors are immediately aware without polling.

### Web Panel — Day-to-Day Operations

25. As the operator, I want to sign in to the panel with my GitHub account, so that I don't manage a separate password.
26. As the operator, I want to add or remove additional users (by GitHub username) through the panel, so that co-maintainers can join without me editing config files over SSH.
27. As an authorized user (operator or co-maintainer), I want to see a list of registered repositories and their currently active preview environments, so that I have a single view of what's running.
28. As an authorized user, I want to view live build logs for in-progress deployments, so that I can monitor builds without polling the database.
29. As an authorized user, I want to manually redeploy a preview, tear it down, or extend its TTL from the panel, so that I can take immediate action without waiting for events.
30. As an authorized user, I want a public, signed, time-limited URL referenced from the PR comment that lets anyone with the link view build logs read-only, so that contributors can debug their own build failures without panel access.

### Forked Repository Handling

31. As the operator, I want forked PRs to never receive any of my secrets or credentials, so that fork code can't exfiltrate sensitive data.
32. As the operator, I want forked PR compose files to be parsed and rejected if they contain dangerous constructs (privileged mode, host bind mounts, host networking, capability adds), so that fork code can't escape the preview container.
33. As the operator, I want all preview containers to receive injected resource limits (CPU, memory, PIDs) regardless of compose file content, so that a runaway preview can't starve the host.

### Domain & TLS

34. As the operator, I want wildcard TLS certificates to be provisioned and renewed automatically via Let's Encrypt DNS-01 against Cloudflare, so that I never touch certificate management after initial setup.
35. As the operator, I want preview subdomains to be created and routed automatically as containers start and stop, so that I never configure DNS or routing per environment.

### Failure & Recovery

36. As the operator, I want prout to recover gracefully from a service restart — reconciling running containers against database state, removing orphans, marking missing environments as destroyed, and re-enqueuing interrupted jobs — so that a server reboot doesn't require manual cleanup.
37. As the operator, I want the application to fail fast with a clear instruction at startup if the database schema is out of date, so that I never silently run an old binary against a new schema or vice versa.
38. As the operator, I want webhook events to be deduplicated by delivery ID, so that GitHub's at-least-once delivery doesn't cause double deployments.
39. As the operator, I want all configuration changes, runtime actions, and trigger evaluations recorded in an audit log, so that I can reconstruct what happened when something behaves unexpectedly.

### Administration via CLI

40. As the operator, I want a CLI on the same binary for administrative operations (`prout migrate up`, `prout reconcile`, `prout env list`, `prout env destroy <id>`, `prout logs <pr-number>`), so that I can script and SSH-debug without depending on the panel.

---

## Functional Requirements

### GitHub Integration

- prout operates as a single GitHub App. All API access uses short-lived installation access tokens generated from the App's private key.
- Required permissions: `contents: read` (tarball download), `pull_requests: write` (PR comments), `statuses: write` (commit status), `metadata: read`.
- Webhook subscriptions: `pull_request` (opened, synchronize, closed, labeled), `issue_comment` (created — for `/deploy`-style commands), `installation` (created, deleted).
- All GitHub interactions are server-side; forks never receive any credentials.
- A separate GitHub OAuth flow (using the App's user-to-server token capability) is used for panel sign-in.

### Configuration System (Three Surfaces)

Each piece of configuration has exactly one source of truth.

- **Server config** (`/etc/prout/server.yml` plus environment variables) defines: wildcard domain, GitHub App credentials (App ID, private key path, webhook secret), GitHub OAuth credentials, Cloudflare API token, ACME email, default resource limits, bootstrap owner GitHub username. Loaded once at startup. Edited via SSH only.
- **Web panel** defines: per-repository enable/disable, triggers, lifecycle (TTL, max concurrent), environment variables (non-sensitive, plain text), exposed service name and port, compose file path, allowed users.
- **CLI** exposes administrative operations: `init`, `migrate up/down/status`, `reconcile`, `env list`, `env destroy`, `logs <pr-number>`. Same backing operations as the panel where applicable.

The panel does not edit server config. The CLI does not edit per-repo config in MVP.

### Trigger System

- Supported trigger types: `automatic` (on PR open / synchronize), `label` (on PR labeled with a configured label), `comment` (on PR comment matching a configured prefix, e.g. `/deploy`).
- Each trigger is configured per repository through the panel.
- Trigger evaluation is performed server-side using live GitHub permission data — webhook payload claims about the actor are never trusted.
- The `automatic` trigger type is hard-disabled for PRs from forks regardless of configuration. Forked PRs always require label or comment triggers initiated by an authorized user.
- A PR author with `write+` permission on the base repository is treated as non-fork for trigger evaluation, so maintainers contributing from personal forks aren't penalized.
- The mapping from GitHub permission level (`admin`, `maintain`, `write`, `triage`, `read`, `none`) to trigger authorization is fixed in the MVP.

### Build System

- MVP supports Docker Compose as the only build/run target.
- prout downloads PR head as a tarball via the GitHub REST API at the specific commit SHA (no `git clone`, no submodule support, no LFS in MVP).
- Tarball is extracted into an isolated workspace directory under `/var/lib/prout/workspaces/{job-id}/`.
- The user-provided `docker-compose.preview.yml` is parsed and sanitized — privileged mode, capability adds, host bind mounts, host PID/network/IPC, devices, are rejected. Only named volumes and relative-path volumes within the workspace are permitted.
- prout injects into the generated compose: resource limits (CPU, memory, PIDs from server defaults), Traefik routing labels, the project network, `prout_*` metadata environment variables, and per-repository configured environment variables.
- Build executes via subprocess `docker compose -f ... -p prout-pr-{n} up -d --build`.
- Container introspection (listing, inspect, network state, events) uses the Docker Engine SDK directly.
- The Docker daemon is accessed exclusively through `tecnativa/docker-socket-proxy` with a filtered API surface.
- Build logs (stdout and stderr) are streamed line-by-line to PostgreSQL and to active Server-Sent Events subscribers in real time.
- After successful deployment, the workspace directory is removed.

### Routing & TLS

- Traefik v3 runs as part of the prout compose stack and uses the Docker provider through the socket proxy.
- Each preview environment is assigned the subdomain `pr-{number}.preview.{wildcard-domain}` (configured wildcard).
- Routing is applied via Docker labels injected by prout onto the exposed service container.
- TLS certificates are provisioned via Let's Encrypt DNS-01 challenge through Cloudflare. A wildcard certificate covers all preview subdomains.
- The wildcard A record (`*.preview.{domain} → VPS_IP`) must be configured in Cloudflare with proxy disabled (DNS-01 incompatibility); this is documented as a one-time manual step.
- Only one exposed service per preview environment is supported in MVP.

### Lifecycle Management

- Environments are torn down on: PR close, PR merge, TTL expiry, manual operator action.
- A periodic job runs every 5 minutes to find environments past their TTL and enqueue teardown jobs.
- A new commit pushed to an open PR triggers a redeploy that destroys the existing environment and builds a fresh one at the new SHA.
- All state (registered repositories, configurations, environments, webhook events, audit log, build logs) is persisted in PostgreSQL.
- Database schema is the single source of truth; Docker container state is reconciled against the database on startup and detected drift is corrected.
- On startup, prout performs reconciliation: orphan containers are removed, missing containers are marked destroyed in the DB, jobs left in `running` state at the previous crash are re-enqueued.

### Authentication & Authorization

- Panel sign-in uses GitHub OAuth (via the GitHub App's user-to-server token capability).
- The set of allowed panel users is stored in PostgreSQL (`allowed_users` table keyed on GitHub username), editable through the panel. The bootstrap owner from `server.yml` is inserted with `is_owner=true` on first start.
- A panel user must be present in `allowed_users` to sign in; `is_owner` distinguishes the user able to add or remove other users.
- Sessions use signed cookies with short expirations.
- Trigger authorization (whether a given GitHub user's PR comment or label triggers a deploy) is evaluated separately and always uses live GitHub permission data for each event. Panel `allowed_users` membership has no bearing on trigger authorization.

### Environment Variables for Preview Environments

- Three layers, merged in this order at deploy time:
  1. Auto-injected `prout_*` (PR number, PR URL, commit SHA, preview URL, preview domain).
  2. Per-repository env vars configured in the panel — plain text, key/value list, stored in `repository_env_vars` table.
  3. Compose-defined values inside `docker-compose.preview.yml` (left intact).
- The panel UI displays a clear notice: "stored in plain text — use only test/sandbox API keys."
- No encryption-at-rest in MVP. Schema is shaped to allow `is_secret` / `encrypted_value` columns in a later migration without disrupting the existing model.

### Database Migrations

- Migrations are managed via `goose` with embedded SQL files (`embed.FS`).
- Migrations are not auto-applied at startup. The operator runs `prout migrate up` deliberately as part of an upgrade.
- At startup, the application checks the current schema version against the version embedded in the binary; if the DB is out of date, the application fails fast with a clear instruction. If the DB is ahead, it logs a warning but continues.
- River's own schema migrations are wrapped as Go-based `goose` migrations so a single `prout migrate up` covers everything.

---

## Non-Functional Requirements

### Reliability

- The webhook handler must respond within ~100 ms in the happy path; all heavy work is asynchronous via the River job queue.
- Job retries use exponential backoff. Permanent failures (sanitizer rejection, invalid Dockerfile) skip retry; transient failures (tarball timeout, GitHub API rate limit) retry up to a configured cap.
- Webhook handler is idempotent — events are deduplicated by GitHub delivery ID.
- TTL cleanup runs independently of webhook delivery.
- Reconciliation on startup detects and corrects drift between database state and Docker daemon state.

### Performance

- For typical Docker Compose builds, the time from PR event to PR comment with preview URL should be under 5 minutes.
- The prout control plane has a minimal resource footprint — it orchestrates containers, it does not run workloads.
- Build CPU and disk I/O are the realistic bottlenecks. Per-repository `max_concurrent` and server-wide resource caps mitigate contention.

### Security

- GitHub App private key, OAuth client secret, Cloudflare API token, and HMAC webhook secret are never stored in the database — they live in `server.yml` referenced by file path or environment variable.
- Webhook signatures (HMAC-SHA256) are verified on every delivery; invalid signatures are rejected before any processing.
- Forked PR code runs in a sanitized compose with injected resource limits, in an isolated network, accessed through a filtered Docker API surface.
- Trigger authorization is enforced server-side using live GitHub permission data for the relevant actor (sender, commenter, or PR author depending on event type).
- The panel is HTTPS-only (Traefik enforces TLS for the panel hostname as well as preview subdomains).
- The build log viewer URL referenced from PR comments uses signed, time-limited tokens for read-only access.

### Operational Footprint

- Single binary deployed via Docker. The binary contains the HTTP server, panel templates, static assets, CLI commands, worker pool, and embedded migrations.
- Single VPS deployment topology. prout and all preview environments share the host.
- One PostgreSQL database co-located in the compose stack.
- Application logs to stdout as structured JSON via `slog`.
- No metrics or alerting in MVP. The operator notices issues through missing PR comments or visible panel state.

### Portability

- Distributed as a Docker image. Deployment requires Docker, the Docker Compose plugin, and a host with at least 4 GB RAM and 2 vCPU.
- Architecture preserves a future migration path to:
  - Worker process separation (River supports out-of-process workers without code changes).
  - Multi-host workers (workers on additional VPSes pulling from the same PostgreSQL).
  - Managed PostgreSQL (database connection is the only external dependency).
  - Kubernetes runtime (the Docker Engine SDK and `docker compose` subprocess calls are kept behind a small runtime interface, replaceable by a Kubernetes client).
- No effort spent on these migrations in MVP — they are kept feasible, not realized.

---

## Success Metrics

prout is a personal-scale tool. Adoption-style metrics (GitHub stars, external installations) are not goals.

### Operator-Level Metrics

| Metric | Target |
|---|---|
| Time from PR open to preview URL posted (median) | < 5 minutes |
| Successful preview deployments per attempt | > 90% (after first valid configuration) |
| Operator time spent on preview-related toil per week | Approaching zero |
| Manual interventions required after deploy (SSH, container fix) | < 1 per week in steady state |

### Health Signals

- Reconciliation finds zero orphan containers in steady state.
- TTL cleanup finds expired environments before manual notice in 100% of cases.
- Audit log is consulted at least once per non-trivial incident — meaning the audit log is sufficient to reconstruct events.
- Postgres backup (via `pg_dump` cron) restores cleanly to a fresh VPS as a quarterly drill.

### Anti-Metrics (Things Not to Optimize For)

- GitHub stars on the prout repository.
- Number of external installations.
- Onboarding time for hypothetical new operators.
- Polish of public-facing documentation.

These are explicitly not goals in MVP. Pursuing them costs effort that should go into the operator's own OSS projects, not into prout itself.

---

## Out of Scope (MVP)

The following are deliberately excluded. Each is recoverable later through a clear migration path noted in the Architecture Decision Records.

- **Capabilities other than preview environments** (autofix, triage, label sync, release-notes assist).
- **Plugin interface and SDK.** Preview is implemented as part of core; abstraction is deferred until 2–3 capabilities exist.
- **Encrypted application secrets.** Plain-text env vars in DB only; no encryption-at-rest in MVP.
- **Role-based access control / granular permissions.** All allowed users have full panel access except for managing other users (owner-only).
- **Invitation flow with email.** Users are added by GitHub username through the panel; the operator shares the panel URL out of band.
- **Self-service signup or registration.**
- **Multi-VPS / multi-host worker architecture.**
- **Kubernetes deployment target.**
- **Managed cloud offering.**
- **Custom domains per repository** (only the operator's wildcard domain).
- **GitLab / Gitea / Bitbucket integration.**
- **Buildpacks or Dockerfile-less builds.**
- **Git submodules, Git LFS, monorepo selective deployments.**
- **Per-repository CPU / memory caps** (only server-wide defaults).
- **Notifications beyond GitHub PR comments** (Slack, email).
- **Audit log UI beyond a basic table.**
- **Metrics, alerting, observability stack** (Prometheus, OpenTelemetry, etc.).
- **Multi-tenant / organization model.**

---

## Assumptions

- The operator runs a single VPS with a public IP, ≥4 GB RAM, ≥2 vCPU, Docker, and the Docker Compose plugin installed.
- The operator owns a domain and uses Cloudflare as DNS provider, with API token access permitting `Zone:DNS:Edit` on the relevant zone.
- The operator has, or is willing to create, a GitHub App with the required permissions and webhook subscriptions.
- The target repositories use pull requests as the standard contribution workflow.
- The preview-target applications are containerizable with Docker Compose. Workloads requiring submodules, Git LFS, or selective monorepo builds are out of scope for the first user.
- Test/sandbox API keys (Stripe test mode, Resend sandbox, low-limit OpenAI keys) are sufficient for previewing features that depend on third-party services. Production-grade secret handling is not required in MVP.
- Two to three trusted co-maintainers may need panel access; full self-service onboarding is not required.

---

## Risks

### Product Risks

- **Untrusted code execution during build.** A malicious Dockerfile from a forked PR may attempt container escape. Mitigated by compose sanitization (rejecting privileged, host bind mounts, host networking, capabilities, devices), injected resource limits (CPU/memory/PIDs), per-PR network isolation, and the docker-socket-proxy filtering. Rootless build runtime (buildkit-rootless or Podman) is on the post-MVP list for stronger isolation.
- **Docker socket compromise.** Direct access grants root-equivalent control over the host. Mitigated by accessing the daemon through `tecnativa/docker-socket-proxy` with the minimum API surface enabled. Documented requirement: dedicated VPS, not shared with production workloads.
- **Resource contention.** A heavy build can degrade running previews. Mitigated by `max_concurrent` limits per repository and server-wide resource caps. Acknowledged trade-off of single-VPS topology; horizontal worker scaling is a post-MVP option.
- **Disk exhaustion from accumulated Docker layers.** Mitigated by a periodic `docker system prune` cron and documented in operations notes.
- **Configuration locked to database, no git history.** Trade-off of database-as-source-of-truth. Mitigated by an in-app audit log recording all configuration changes (who, what, when).

### Operational Risks

- **Single VPS is a single point of failure.** Acceptable trade-off for personal scale; documented backup strategy (`pg_dump` cron) and quarterly restore drill.
- **Panel and bot share one process.** A panel-only crash takes down event handling. Acceptable for MVP; process separation is a post-MVP option.
- **No metrics or alerting.** If something breaks silently, the operator notices through missing PR comments. Acceptable trade-off for MVP scope; observability stack is post-MVP.
- **Cloudflare API token in `server.yml`.** Compromise enables DNS manipulation on the configured zone. Mitigated by scoping the token to a single zone with `Zone:DNS:Edit` permission only, and by the documented requirement of treating the VPS as a dedicated host.

### Project Risks

- **Scope creep toward "public OSS product."** The personal-scale framing is the entire point — drifting toward polished onboarding, multi-tenant features, or marketplace concerns destroys the MVP's tractability. Mitigated by explicit "anti-metrics" in this document and by the ADRs that name what is deliberately deferred.
- **prout itself becoming the OSS project that demands maintenance.** Same root cause as above. Mitigated by deliberately not promoting the project externally during the MVP phase.

---

## Further Notes

prout is named for the metaphor of a shared space where tools are kept ready to use. The name reinforces the product's positioning: practical, local, and built for the person who works there.

The product's north star: **the operator's own OSS work spends less time on automation toil because prout exists.** Every architectural decision is evaluated against this. If a feature would help a hypothetical external user but doesn't measurably help the operator, it is deferred or rejected.

The architecture preserves an evolution path toward broader use — process separation, multi-host workers, Kubernetes runtime, encrypted secrets, RBAC, plugin interface, additional VCS providers — without realizing any of it in MVP. Each deferred capability has a documented migration path in the ADRs. This is the discipline of "personal-scale today, optionally more later" rather than "personal-scale today, permanently locked to personal-scale."

Implementation follows a walking skeleton approach: a minimal end-to-end pipeline (mock webhook → mock build → mock comment) is in place from the first iteration, and each subsequent iteration replaces a mock with real implementation. This keeps the system demo-able from week one and surfaces integration issues early, which matters more than the polish of any individual layer for a project on this trajectory.