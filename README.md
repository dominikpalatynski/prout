# Toolshed

GitHub-driven preview environment system for self-hosted PR deployments.

See `.ai/specs/` for product requirements and architecture decisions.

## First-day setup

```sh
task setup            # mise install, tailwind binary, go mod download
task dev:up           # start PostgreSQL
task build            # build the binary
./bin/toolshed migrate up
TOOLSHED_DB_DSN=postgres://toolshed:toolshed@localhost:5433/toolshed?sslmode=disable ./bin/toolshed server --config server.yml.example
```

`task migrate:*` wraps `go run ./cmd/toolshed migrate ...`. `task` auto-loads `.env`, so for host-run commands the DSN must use `localhost:5433`, not the Docker-only hostname `postgres:5432`.

Smoke checks:
- `curl http://localhost:8080/healthz`
- `curl http://localhost:8080/readyz`
- `curl -X POST http://localhost:8080/webhooks/github -H 'Content-Type: application/json' -H 'X-GitHub-Event: pull_request' -H 'X-GitHub-Delivery: local-phase-1' -d '{"action":"opened","number":42,"repository":{"id":123456},"pull_request":{"head":{"sha":"abc123"}}}'`
- `curl 'http://localhost:8080/debug/pings?limit=5'`

## Layout

| Path | Purpose |
|---|---|
| `cmd/toolshed/` | Single binary entry point (cobra root). |
| `internal/` | All application code. |
| `internal/runtime/` | Runtime escape-hatch interface (compose / k8s). |
| `internal/web/` | Templ templates + Tailwind input. |
| `migrations/` | Embedded Goose migrations (SQL + Go-based River init). |
| `deploy/` | Operator deployment artifacts (Dockerfile + secrets). |

See `.ai/specs/scaffold_tech.md` for the full technical scaffold spec.
