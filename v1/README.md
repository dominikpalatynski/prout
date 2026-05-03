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
- `curl -H 'Authorization: Bearer replace-me' http://localhost:8080/api/event-families`
- `curl -H 'Authorization: Bearer replace-me' http://localhost:8080/api/trigger-types`
- `curl -X POST http://localhost:8080/api/repositories -H 'Authorization: Bearer replace-me' -H 'Content-Type: application/json' -d '{"full_name":"owner/repo"}'`
- `curl -H 'Authorization: Bearer replace-me' http://localhost:8080/api/repositories/1/event-families`
- `curl -X PUT http://localhost:8080/api/repositories/1/runtime-settings -H 'Authorization: Bearer replace-me' -H 'Content-Type: application/json' -d '{"compose_file_path":"compose.yml","exposed_service_name":"app","exposed_service_port":8080}'`
- `curl -X PUT http://localhost:8080/api/repositories/1/environment-variables -H 'Authorization: Bearer replace-me' -H 'Content-Type: application/json' -d '{"environment_variables":[{"name":"APP_ENV","value":"preview"}]}'`
- `curl -X POST http://localhost:8080/api/repositories/1/triggers -H 'Authorization: Bearer replace-me' -H 'Content-Type: application/json' -d '{"type":"preview_on_pull_request_opened"}'`
- `payload='{"action":"opened","number":42,"repository":{"id":123456},"pull_request":{"head":{"sha":"abc123"}}}'`
- `sig=$(printf '%s' "$payload" | openssl dgst -sha256 -hmac 'replace-me' -binary | xxd -p -c 256)`
- `curl -X POST http://localhost:8080/webhooks/github -H 'Content-Type: application/json' -H 'X-GitHub-Event: pull_request' -H 'X-GitHub-Delivery: local-phase-2' -H "X-Hub-Signature-256: sha256=$sig" -d "$payload"`
- `curl -H 'Authorization: Bearer replace-me' 'http://localhost:8080/api/webhook-events?limit=5'`

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
