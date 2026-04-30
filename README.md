# Toolshed

GitHub-driven preview environment system for self-hosted PR deployments.

See `.ai/specs/` for product requirements and architecture decisions.

## First-day setup

```sh
task setup            # mise install, tailwind binary, go mod download
task dev:up           # start Postgres + River UI in compose.dev.yml
task migrate:up       # apply schema
task dev              # live-reload loop (templ + tailwind + air)
```

Smoke checks:
- `curl http://localhost:8080/healthz` → `{"status":"ok"}`
- River UI at `http://localhost:8081`

## Layout

| Path | Purpose |
|---|---|
| `cmd/toolshed/` | Single binary entry point (cobra root). |
| `internal/` | All application code. |
| `internal/runtime/` | Runtime escape-hatch interface (compose / k8s). |
| `internal/web/` | Templ templates + Tailwind input. |
| `migrations/` | Goose migrations (SQL + Go-based River init). |
| `deploy/` | Operator deployment artifacts (Dockerfile + secrets). |

See `.ai/specs/scaffold_tech.md` for the full technical scaffold spec.
