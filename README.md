# prout

Self-hosted automation for ephemeral GitHub Pull Request environments.

## What it does

`prout` listens for GitHub events and spins up ephemeral preview environments
for pull requests, using Docker Compose under the hood. Each PR gets its own
isolated environment that comes up when the PR is opened and goes away when
it's closed.

The core is event-driven, so PR previews are just the first automation —
the same machinery can be extended to handle other GitHub events and run
other kinds of jobs.

## Status

Pre-release. Actively developed.

## Quickstart

Copy the example config and fill in your own values:

```bash
cp server.example.yml server.yml
```

Edit `server.yml` — set `server.base_url`, `server.admin_secret`,
`db.dsn`, the `github.repository` section (owner / name / build settings),
and an `auth` username and password. GitHub App credentials live separately
in `app_config/` and are populated via the setup flow (see Configuration
below).

**Local:**

```bash
make build      # produces ./bin/prout
make run        # ./bin/prout server --config ./server.yml
```

**Docker:**

```bash
make docker-up      # build + start
make docker-logs    # follow logs
make docker-down    # stop
```

The server listens on `:8090` by default.

## Configuration

Configuration lives in `server.yml`. To wire prout up to GitHub you'll need
to register a GitHub App and drop its credentials into the config — see
[.ai/specs/github_app_setup_handoff.md](.ai/specs/github_app_setup_handoff.md)
for the full setup.

## Project layout

```
cmd/         CLI entry point (Cobra)
internal/    Core packages
  auth/        GitHub App auth + OAuth
  config/      server.yml loader
  event/       event family / automation registry
  github/      GitHub API client
  log/         structured logging
  queue/       job queue
  server/      HTTP server
  workspace/   per-PR workspace lifecycle
.ai/specs/   product & technical specs
app_config/  GitHub App config templates
```

## Extending it

prout is built around an event family registry (`internal/event`) backed
by a job queue (`internal/queue`). New automations — beyond PR previews —
are added by registering handlers for additional event families. See
[.ai/specs/event_family_registry_prd.md](.ai/specs/event_family_registry_prd.md)
and [.ai/specs/0001-event-family-automation-registry.md](.ai/specs/0001-event-family-automation-registry.md)
for the design.

## Docs

- Product overview: [.ai/specs/prd.md](.ai/specs/prd.md)
- Technical design: [.ai/specs/technical_prd.md](.ai/specs/technical_prd.md)
- Implementation phases: [.ai/specs/implementation_phases.md](.ai/specs/implementation_phases.md)
