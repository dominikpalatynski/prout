# Toolshed Implementation Phases

This document translates the accepted architecture decisions from `adr_tech.md` into high-level implementation phases.

The intent is not to describe detailed tasks or low-level design. Each phase should:

- deliver a meaningful vertical slice,
- introduce only the entities and helpers that are needed at that stage,
- keep infrastructure work proportional to the phase goal,
- leave a clear test boundary before moving to the next phase.

## Phase 1: Walking Skeleton Foundation

**Goal**

Establish the technical backbone of the system: bootstrapping, configuration, database connectivity, migrations, River worker startup, and one mock webhook-to-job path.

**Potential entities**

- `_ping` or equivalent technical probe table
- River schema tables

**Potential helpers / packages**

- Application bootstrap and dependency wiring
- Config loading and validation
- Database pool initialization
- Migration command and schema version check on startup
- Mock webhook handler
- Simple River job and worker, such as `EchoJob`
- Health and readiness checks

**Infrastructure topics**

- Local development Postgres
- River UI in local development
- Base CLI commands for `server` and `migrate`
- Structured logging
- Local config template and startup flow

**Acceptance boundary**

A local HTTP request reaches the webhook endpoint, enqueues a job, and the worker consumes it successfully.

## Phase 2: Real GitHub Ingress

**Goal**

Replace the mock ingress with real GitHub webhook delivery, signature verification, delivery deduplication, and event persistence.

**Potential entities**

- `webhook_events`
- Minimal `repositories` anchor for GitHub App installation lifecycle

**Potential helpers / packages**

- HMAC signature verifier
- GitHub webhook parser and normalizer
- Delivery deduplication logic
- GitHub App authentication and installation token provider
- Event router
- Minimal trigger evaluation entry point

**Infrastructure topics**

- Real GitHub App setup
- File-mounted webhook secret and app private key
- Public webhook endpoint
- Test repository connected to the GitHub App

**Acceptance boundary**

A real GitHub event is accepted once, persisted once, and asynchronously forwarded for downstream processing.

## Phase 3: Tarball Retrieval and Workspace Materialization

**Goal**

Fetch repository code for an exact commit SHA and materialize it into an isolated workspace on disk.

**Potential entities**

- `repositories` in active use
- Minimal deployment attempt tracking if needed before full runtime state
- Early `environments` record only if it improves lifecycle clarity

**Potential helpers / packages**

- Installation-authenticated GitHub API client
- Tarball downloader
- Archive extractor
- Workspace manager
- Workspace cleanup helper

**Infrastructure topics**

- Workspace directory layout on disk
- Filesystem permissions
- Temporary storage and cleanup rules
- Disk usage visibility for local and VPS environments

**Acceptance boundary**

Given a PR event and commit SHA, the system produces a clean workspace containing the expected repository contents.

## Phase 4: Runtime Deployment via Docker Compose

**Goal**

Turn a prepared workspace into a running preview stack using the Docker Compose runtime path.

**Potential entities**

- `environments`
- Minimal per-repository runtime settings needed for deployment

**Potential helpers / packages**

- Runtime interface implementation
- Deploy worker
- Teardown worker
- Compose parser and sanitizer
- Environment variable merger
- Environment state transition helper

**Infrastructure topics**

- `docker compose` CLI availability
- Docker socket proxy
- Runtime networks
- Resource limits
- Generated compose artifacts and temporary files

**Acceptance boundary**

A PR-triggered deployment starts a preview stack successfully and a teardown path removes it cleanly.

## Phase 5: Public Routing, TLS, and GitHub Feedback

**Goal**

Expose running previews through stable HTTPS URLs and report the result back to GitHub.

**Potential entities**

- `environments` extended with final preview URL and publication metadata
- Optional storage for GitHub comment or status identifiers

**Potential helpers / packages**

- Preview URL builder
- Traefik label injector
- GitHub PR comment publisher/updater
- Commit status publisher
- Optional signed log-link builder

**Infrastructure topics**

- Traefik
- Wildcard DNS
- Let's Encrypt DNS-01
- Cloudflare token and zone configuration
- Panel hostname and preview subdomain topology

**Acceptance boundary**

A PR receives a working HTTPS preview URL and GitHub reflects deployment success or failure.

## Phase 6: Panel and Access Control

**Goal**

Introduce the operator panel, GitHub OAuth login, and minimal multi-user access control.

**Potential entities**

- `allowed_users`
- Full `repositories` configuration surface as needed by the panel
- `repository_env_vars`

**Potential helpers / packages**

- OAuth flow handlers
- Session middleware
- Bootstrap owner logic
- Allowed-user service
- Templ-based page and partial handlers
- HTMX endpoints for small panel actions

**Infrastructure topics**

- GitHub OAuth credentials
- Session and CSRF secrets
- Asset generation pipeline for `templ` and Tailwind
- Panel routing through Traefik

**Acceptance boundary**

The owner can log in, view repositories and environments, add an allowed user, and perform a manual teardown.

## Phase 7: Hardening and Operator Workflows

**Goal**

Make the system operationally reliable with logs, cleanup, reconciliation, auditability, and basic admin workflows.

**Potential entities**

- `build_logs`
- `audit_log`
- Extended `environments` fields for lifecycle and reconciliation

**Potential helpers / packages**

- Build log sink
- SSE broadcaster
- TTL cleanup worker
- Startup reconciler
- Audit writer
- Admin CLI handlers such as `reconcile`, `env list`, `env destroy`, and `logs`

**Infrastructure topics**

- River cron jobs
- Log retention
- Restart and crash recovery path
- Database backup and restore expectations
- Workspace and orphan cleanup

**Acceptance boundary**

After restart or failure, the system can reconcile runtime state, clean up expired previews, and give operators enough visibility to diagnose problems without direct host inspection.

## Notes

- These phases intentionally follow the walking skeleton approach from `adr_tech.md`.
- Each phase should introduce only the minimum persistent model required for the next real capability.
- If a helper or entity is not needed to make the phase testable, it should stay deferred.
