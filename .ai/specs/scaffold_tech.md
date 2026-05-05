# prout — Scaffold Technical Specification

This document captures all technical decisions made during the project-scaffolding interview, prior to writing the first line of code. It complements `prd.md` (product scope) and `adr_tech.md` (architecture decisions). Where this document and `adr_tech.md` overlap, this document is the **operational refinement** — the concrete shape decisions take in the repository.

**Status:** Draft, ready for scaffolding.
**Date:** 2026-04-30.

---

## Executive Summary — Stack Table

| Concern | Choice | Source |
|---|---|---|
| Language | Go 1.26 (latest stable, pinned via `toolchain` in `go.mod`) | adr_tech.md ADR-001 |
| Project layout | Lightweight idiomatic (`cmd/` + `internal/`, no `pkg/`) | §1 |
| Tool versioning (Go-based) | `tool` directive in `go.mod` (Go 1.24+ feature) | §2 |
| Tool versioning (non-Go) | `mise` (`.mise.toml`) — Go toolchain, Tailwind CLI | §2 |
| Task orchestration | `Taskfile.yml` (taskfile.dev) | §3 |
| HTTP router | `go-chi/chi` v5 | §4 |
| Configuration | Custom struct + `gopkg.in/yaml.v3` + `caarlos0/env/v11` | §5 |
| Test assertions | `testify/require` + `google/go-cmp/cmp` | §6 |
| Test runner | `gotestsum` (in `go.mod tool`) | §6 |
| Integration tests | `testcontainers-go` (Postgres) | §6 |
| Test fakes | Hand-rolled, in `internal/<pkg>/<pkg>test/` packages | §6 |
| Dev live-reload | `air` + parallel `templ --watch` + `tailwindcss --watch` | §7 |
| Lint | `golangci-lint` v2 (curated set, ~20 linters) | §8 |
| Format | `gofumpt` + `gci` | §8 |
| Local dev infra | `compose.dev.yml` (Postgres 16-alpine + riverui) | §9 |
| Pre-commit hooks | None (skipped on operator's request) | §10 |
| Container runtime base | `alpine:3.19` (`docker-cli` + `docker-cli-compose` required, ADR-005) | §11 |
| Build pipeline | Multi-stage Dockerfile, generated code **not committed** | §11 |
| Production stack | `compose.yml` with secrets file-mounted, 2 networks (traefik + internal) | §11 |
| CI | GitHub Actions, split into `ci.yml` (PR gates) + `release.yml` (image push) | §12 |
| Image registry | `ghcr.io/<user>/prout`, tags `:latest` + `:sha-<short>` | §12 |
| Image architecture | `linux/amd64` only (multi-arch deferred) | §12 |
| Dependency bot | None (deferred) | §12 |
| Logging | `log/slog` + JSON/text via config + custom ctx-aware handler + `tint` (dev) | §13 |
| Data layer (queries) | `sqlc` v2 with `sql_package: pgx/v5` | §14 |
| Data layer (driver) | `jackc/pgx/v5` + `pgxpool` | §14 |
| Migrations | `goose` (CLI-only, embedded `*.sql` + Go-based River init) | §14 / ADR-010 |
| DB access pattern | Single `*Store` wrapper around `*sqlc.Queries`, with `Tx(ctx, fn)` helper | §14 |

---

## 1. Folder Structure

```
prout/
├── cmd/
│   └── prout/
│       └── main.go                  # cobra root, single binary
│
├── internal/
│   ├── config/                      # server.yml + env loader (§5)
│   ├── log/                         # slog wrapper, ctx-aware handler (§13)
│   ├── server/                      # HTTP listener, chi root (§4)
│   │   ├── server.go
│   │   ├── routes.go
│   │   ├── middleware/
│   │   │   ├── hmac.go              # GitHub webhook signature
│   │   │   ├── session.go           # cookie session, allowed_users
│   │   │   ├── logger.go
│   │   │   └── signedlink.go        # public log viewer token
│   │   └── handlers/
│   │       ├── webhook.go
│   │       ├── panel.go             # delegates to internal/panel
│   │       └── publiclogs.go
│   ├── webhook/                     # GitHub webhook deserialization
│   ├── worker/                      # River jobs: deploy, teardown, ttl, reconcile
│   ├── preview/                     # core preview logic (deploy/teardown orchestration)
│   ├── runtime/                     # ADR-005 escape-hatch interface
│   │   ├── runtime.go               # interface
│   │   └── dockercompose/           # default impl: subprocess + Engine SDK
│   ├── githubapp/                   # ghinstallation, tarball, statuses, comments
│   ├── auth/                        # OAuth + allowed_users
│   ├── store/                       # data layer (§14)
│   │   ├── store.go                 # *Store + Tx helper
│   │   ├── pool.go                  # pgxpool config
│   │   ├── queries/                 # SOURCE: *.sql — committed
│   │   │   ├── repositories.sql
│   │   │   ├── environments.sql
│   │   │   ├── webhook_events.sql
│   │   │   ├── audit_log.sql
│   │   │   └── allowed_users.sql
│   │   └── sqlc/                    # GENERATED — gitignored
│   ├── audit/                       # audit log emit helpers
│   ├── testdb/                      # testcontainers helper (§6)
│   └── web/                         # panel frontend (§7)
│       ├── static/                  # gitignored: tailwind output, htmx.js (vendored or fetched)
│       ├── styles/
│       │   └── input.css            # Tailwind entry
│       ├── templates/               # *.templ — committed; *.templ.go — gitignored
│       └── embed.go                 # //go:embed static/* templates/*
│
├── migrations/                      # goose (§14, ADR-010)
│   ├── 00001_initial_schema.sql
│   ├── 00002_*.sql
│   └── 00003_river_init.go          # Go-based: runs River's own migrator
│
├── deploy/                          # operator deployment artifacts
│   ├── Dockerfile                   # production image
│   └── secrets/                     # gitignored, file-based docker secrets
│       ├── postgres_user
│       ├── postgres_password
│       └── cf_token
│
├── .github/
│   └── workflows/
│       ├── ci.yml                   # lint + test + build (PR gates)
│       └── release.yml              # image build + push (main + tags)
│
├── compose.yml                      # operator production stack
├── compose.dev.yml                  # local dev infra (Postgres + riverui)
├── server.yml.example               # template for /etc/prout/server.yml
│
├── Taskfile.yml                     # task orchestration (§3)
├── .mise.toml                       # non-Go tool versions (§2)
├── .air.toml                        # live reload config (§7)
├── .golangci.yml                    # lint config (§8)
├── .editorconfig
├── .gitignore
├── .gitattributes                   # *.go.tmpl text=auto eol=lf etc.
├── sqlc.yaml                        # sqlc config (§14)
├── go.mod
├── go.sum
├── README.md
└── LICENSE                          # MIT
```

**Rationale:** All application code under `internal/` so the Go compiler enforces zero external import surface. `cmd/prout/` is the single binary entry point. `web/` lives inside `internal/` because templ-generated `.templ.go` files are part of the Go package graph anyway. `migrations/` and `deploy/` live at root because they are non-Go artifacts referenced by external tooling (goose CLI, `docker compose`).

The `internal/runtime/` interface materializes the K8s escape-hatch from ADR-005 — swap implementation, not restructure system.

---

## 2. Toolchain & Tool Versioning

### 2.1 Go version

Pinned via `toolchain` directive in `go.mod`:

```go
module github.com/dominikpalatynski/prout

go 1.26

toolchain go1.26.0
```

CI uses `actions/setup-go@v5` with `go-version-file: go.mod` — single source of truth.

### 2.2 Go-based tooling — `tool` directive in `go.mod` (Go 1.24+)

Replaces the legacy `tools.go` blank-import pattern. All Go-based tools pinned in module:

```go
// go.mod (excerpt)
tool (
    github.com/a-h/templ/cmd/templ
    github.com/sqlc-dev/sqlc/cmd/sqlc
    github.com/pressly/goose/v3/cmd/goose
    github.com/golangci/golangci-lint/v2/cmd/golangci-lint
    mvdan.cc/gofumpt
    github.com/daixiang0/gci
    github.com/air-verse/air
    gotest.tools/gotestsum
    github.com/lmittmann/tint                  // dev-only logger
)
```

Invocation: `go tool templ generate`, `go tool sqlc generate`, etc. No global `go install`.

### 2.3 Non-Go tooling — `mise` (`.mise.toml`)

```toml
[tools]
go = "1.26"
"npm:tailwindcss" = "3.4"          # via tailwindcss standalone OR npx wrapper
task = "3"

[env]
prout_LOG_FORMAT = "text"        # dev default
prout_DB_DSN = "postgres://prout:prout@localhost:5432/prout?sslmode=disable"
```

Tailwind CLI is fetched as a standalone binary in production (Dockerfile, §11); locally `mise` manages version. Operator does not need `mise`; only developers do.

---

## 3. Task Orchestration — `Taskfile.yml`

Single source for `setup`, `generate`, `build`, `test`, `lint`, `dev`, `migrate:*`, `tailwind:*`. Choice driven by the codegen pipeline (`templ` + `sqlc` + tailwind) benefiting from `sources:`/`generates:` change detection, plus the parallel dev-loop need.

```yaml
# Taskfile.yml (skeleton)
version: "3"

vars:
  BIN_DIR: ./bin
  TAILWIND: "{{.BIN_DIR}}/tailwindcss"
  PKG: github.com/dominikpalatynski/prout

tasks:
  setup:
    desc: One-time local setup (download Tailwind binary, mise install, go mod download)
    cmds:
      - mise install
      - mkdir -p {{.BIN_DIR}}
      - task: tools:tailwind
      - go mod download

  tools:tailwind:
    status:
      - test -x {{.TAILWIND}}
    cmds:
      - curl -sSL "https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-{{OS}}-{{ARCH}}" -o {{.TAILWIND}}
      - chmod +x {{.TAILWIND}}

  generate:
    desc: Run all codegen
    deps: [generate:templ, generate:sqlc]

  generate:templ:
    sources: ["internal/web/templates/**/*.templ"]
    generates: ["internal/web/templates/**/*_templ.go"]
    cmds:
      - go tool templ generate

  generate:sqlc:
    sources: ["internal/store/queries/*.sql", "migrations/*.sql", "sqlc.yaml"]
    generates: ["internal/store/sqlc/**/*.go"]
    cmds:
      - go tool sqlc generate

  tailwind:build:
    sources:
      - "internal/web/styles/input.css"
      - "internal/web/templates/**/*.templ"
      - "tailwind.config.js"
    generates: ["internal/web/static/output.css"]
    cmds:
      - "{{.TAILWIND}} -i ./internal/web/styles/input.css -o ./internal/web/static/output.css --minify"

  build:
    desc: Build binary
    deps: [generate, tailwind:build]
    cmds:
      - go build -o {{.BIN_DIR}}/prout ./cmd/prout

  test:
    desc: Full test suite (incl. testcontainers)
    deps: [generate]
    cmds:
      - go tool gotestsum -- -race -count=1 ./...

  test:short:
    desc: Unit tests only (no testcontainers)
    cmds:
      - go test -short -race ./...

  lint:
    desc: golangci-lint full repo
    deps: [generate]
    cmds:
      - go tool golangci-lint run

  fmt:
    desc: Format code
    cmds:
      - go tool gofumpt -w .
      - go tool gci write -s standard -s default -s "prefix({{.PKG}})" .

  dev:
    desc: Live-reload dev loop (templ + tailwind + air in parallel)
    deps: [generate, tailwind:build]
    cmds:
      - task: _dev:parallel

  _dev:parallel:
    internal: true
    deps: [_dev:templ, _dev:tailwind, _dev:air]

  _dev:templ:
    cmds:
      - go tool templ generate --watch --path ./internal/web/templates
  _dev:tailwind:
    cmds:
      - "{{.TAILWIND}} -i ./internal/web/styles/input.css -o ./internal/web/static/output.css --watch"
  _dev:air:
    cmds:
      - go tool air -c .air.toml

  dev:up:
    desc: Start local DB + riverui
    cmds: ["docker compose -f compose.dev.yml up -d"]
  dev:down:
    cmds: ["docker compose -f compose.dev.yml down"]
  dev:reset:
    cmds:
      - docker compose -f compose.dev.yml down -v
      - task: dev:up
      - sleep 2
      - task: migrate:up

  migrate:up:
    cmds: ["go tool goose -dir ./migrations postgres \"$prout_DB_DSN\" up"]
  migrate:down:
    cmds: ["go tool goose -dir ./migrations postgres \"$prout_DB_DSN\" down"]
  migrate:status:
    cmds: ["go tool goose -dir ./migrations postgres \"$prout_DB_DSN\" status"]
  migrate:create:
    desc: Create new migration file. Usage: task migrate:create -- add_user_role
    cmds:
      - "go tool goose -dir ./migrations -s create {{.CLI_ARGS}} sql"
```

---

## 4. HTTP Router — `chi` v5

Three distinct route groups, each with its own middleware chain:

| Group | Path prefix | Middleware chain |
|---|---|---|
| Public webhook | `/webhooks/github` | `RequestID`, `Logger`, `Recoverer`, `HMACVerify` |
| Public log viewer | `/logs/{token}` | `RequestID`, `Logger`, `Recoverer`, `SignedLinkVerify` |
| Health | `/healthz`, `/readyz` | `Heartbeat` only |
| Static | `/static/*` | `CleanPath`, immutable cache headers |
| Panel | `/panel/*` | `RequestID`, `Logger`, `Recoverer`, `Compress`, `SessionLoad`, `RequireAllowedUser`, `CSRF` |
| Panel SSE | `/panel/streams/*` | `SessionLoad`, `RequireAllowedUser`, **no compress, no buffer** |

`chi.Route("/panel", ...)` with local `Use(...)` materializes each group cleanly. SSE routes excluded from `Compress` per-route.

```go
// internal/server/routes.go (sketch)
func (s *Server) mount(r chi.Router) {
    r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
    r.Use(slogmw.Logger(s.logger))

    r.Get("/healthz", s.healthz)
    r.Get("/readyz", s.readyz)

    r.Route("/webhooks", func(r chi.Router) {
        r.Use(middleware.AllowContentType("application/json"))
        r.Post("/github", hmacVerify(s.cfg.GitHubApp.WebhookSecret)(s.handleGitHubWebhook))
    })

    r.Route("/logs", func(r chi.Router) {
        r.Get("/{token}", s.handlePublicLogs)
    })

    r.Route("/panel", func(r chi.Router) {
        r.Use(middleware.Compress(5))
        r.Use(s.session.Load)
        r.Group(func(r chi.Router) {
            r.Get("/login", s.panelLogin)
            r.Get("/oauth/callback", s.panelOAuthCallback)
        })
        r.Group(func(r chi.Router) {
            r.Use(s.session.RequireAllowedUser)
            r.Use(csrf.Protect(s.cfg.Server.CSRFKey))
            r.Get("/", s.panelHome)
            r.Get("/repositories", s.panelRepositories)
            // ...
        })
    })

    r.Route("/panel/streams", func(r chi.Router) {
        // SSE: no compress, no auth-CSRF
        r.Use(s.session.Load, s.session.RequireAllowedUser)
        r.Get("/builds/{id}", s.streamBuildLogs)
    })

    r.Handle("/static/*", http.StripPrefix("/static/", staticHandler()))
}
```

---

## 5. Configuration

### 5.1 Approach: custom struct + YAML + env

Three-layer precedence: defaults → `server.yml` → env vars. Loaded once at startup. No hot-reload (consistent with ADR-010 explicit-restart model).

```go
// internal/config/config.go
package config

type Config struct {
    Server         ServerConfig    `yaml:"server"`
    BootstrapOwner string          `yaml:"bootstrap_owner" env:"prout_BOOTSTRAP_OWNER"`
    GitHubApp      GitHubAppConfig `yaml:"github_app"`
    OAuth          OAuthConfig     `yaml:"oauth"`
    ACME           ACMEConfig      `yaml:"acme"`
    Defaults       DefaultsConfig  `yaml:"defaults"`
    DB             DBConfig        `yaml:"db"`
    Log            LogConfig       `yaml:"log"`
}

type ServerConfig struct {
    Bind      string `yaml:"bind" env:"prout_BIND"`
    Domain    string `yaml:"domain" env:"prout_DOMAIN"`           // wildcard
    PanelHost string `yaml:"panel_host" env:"prout_PANEL_HOST"`
    CSRFKey   []byte `yaml:"-" env:"prout_CSRF_KEY"`              // 32 bytes
}

type GitHubAppConfig struct {
    AppID             int64  `yaml:"app_id" env:"prout_GH_APP_ID"`
    PrivateKeyPath    string `yaml:"private_key_path" env:"prout_GH_PK_PATH"`
    WebhookSecretPath string `yaml:"webhook_secret_path" env:"prout_GH_WEBHOOK_SECRET_PATH"`
}

type OAuthConfig struct {
    ClientID         string `yaml:"client_id" env:"prout_OAUTH_CLIENT_ID"`
    ClientSecretPath string `yaml:"client_secret_path" env:"prout_OAUTH_CLIENT_SECRET_PATH"`
}

type ACMEConfig struct {
    Email                  string `yaml:"email" env:"prout_ACME_EMAIL"`
    CloudflareAPITokenPath string `yaml:"cloudflare_api_token_path" env:"prout_CF_TOKEN_PATH"`
}

type DefaultsConfig struct {
    TTL                  time.Duration `yaml:"ttl"`
    MaxConcurrentPerRepo int           `yaml:"max_concurrent_per_repo"`
    CPULimit             string        `yaml:"cpu_limit"`
    MemoryLimit          string        `yaml:"memory_limit"`
    PIDsLimit            int           `yaml:"pids_limit"`
}

type DBConfig struct {
    DSN string `yaml:"dsn" env:"prout_DB_DSN"`
}

type LogConfig struct {
    Level     string `yaml:"level" env:"prout_LOG_LEVEL"`         // debug|info|warn|error
    Format    string `yaml:"format" env:"prout_LOG_FORMAT"`       // json|text
    AddSource bool   `yaml:"add_source" env:"prout_LOG_ADD_SOURCE"`
}

func Load(path string) (*Config, error) {
    c := defaults()
    if path != "" {
        b, err := os.ReadFile(path)
        if err != nil {
            return nil, fmt.Errorf("read config: %w", err)
        }
        if err := yaml.Unmarshal(b, c); err != nil {
            return nil, fmt.Errorf("parse yaml: %w", err)
        }
    }
    if err := env.Parse(c); err != nil {
        return nil, fmt.Errorf("parse env: %w", err)
    }
    return c, c.Validate()
}

// Methods that load secrets lazily — never stored as plain strings in Config.
func (c *Config) LoadGitHubPrivateKey() ([]byte, error) {
    return os.ReadFile(c.GitHubApp.PrivateKeyPath)
}
func (c *Config) LoadCloudflareToken() (string, error) {
    b, err := os.ReadFile(c.ACME.CloudflareAPITokenPath)
    return strings.TrimSpace(string(b)), err
}
```

### 5.2 `server.yml.example` template

```yaml
# /etc/prout/server.yml — operator-edited at install time
server:
  bind: ":8080"
  domain: "preview.example.com"
  panel_host: "panel.preview.example.com"

bootstrap_owner: "dominikpalatynski"

github_app:
  app_id: 123456
  private_key_path: /etc/prout/private-key.pem
  webhook_secret_path: /etc/prout/webhook-secret

oauth:
  client_id: "Iv1...."
  client_secret_path: /etc/prout/oauth-secret

acme:
  email: "ops@example.com"
  cloudflare_api_token_path: /etc/prout/cf-token

defaults:
  ttl: 72h
  max_concurrent_per_repo: 2
  cpu_limit: "2"
  memory_limit: "2g"
  pids_limit: 512

db:
  dsn: "postgres://prout:CHANGEME@postgres:5432/prout?sslmode=disable"

log:
  level: info
  format: json
  add_source: false
```

---

## 6. Testing Strategy

| Layer | Tooling |
|---|---|
| Test runner | `go tool gotestsum` (pretty output, JUnit XML for CI) |
| Assertions (flow) | `testify/require` — `require.NoError(t, err)` |
| Diffs (struct equality) | `google/go-cmp/cmp` — `cmp.Diff(want, got)` |
| Integration DB | `testcontainers-go` Postgres module, helper in `internal/testdb/` |
| Fakes | Hand-rolled in `internal/<pkg>/<pkg>test/`, e.g. `runtimetest.Fake{}` |
| Coverage gate | **Deferred** to post-MVP |

`testify/assert` and `testify/suite` are intentionally **not used** (suite is anti-idiomatic in modern Go; assert encourages "test continues after failure" anti-pattern in setup).

### 6.1 `internal/testdb/` helper sketch

```go
// internal/testdb/testdb.go
package testdb

import (
    "context"
    "testing"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Start spins up a Postgres testcontainer scoped to the test, returns a connected pool.
func Start(t *testing.T) *pgxpool.Pool {
    t.Helper()
    ctx := context.Background()
    pgC, err := postgres.Run(ctx, "postgres:16-alpine",
        postgres.WithDatabase("prout"),
        postgres.WithUsername("prout"),
        postgres.WithPassword("prout"),
        postgres.BasicWaitStrategies(),
    )
    if err != nil { t.Fatalf("start postgres: %v", err) }
    t.Cleanup(func() { _ = pgC.Terminate(ctx) })

    dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
    if err != nil { t.Fatalf("dsn: %v", err) }

    pool, err := pgxpool.New(ctx, dsn)
    if err != nil { t.Fatalf("pool: %v", err) }
    t.Cleanup(pool.Close)

    if err := goose.UpContext(ctx, stdlib.OpenDBFromPool(pool), "migrations"); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    return pool
}
```

Tests using testdb start with `if testing.Short() { t.Skip("integration") }` — so `task test:short` (and `pre-push` in any future hook) skip them.

### 6.2 Hand-rolled fake example

```go
// internal/runtime/runtimetest/fake.go
package runtimetest

import "github.com/dominikpalatynski/prout/internal/runtime"

type Fake struct {
    DeployFn   func(ctx context.Context, p runtime.DeployParams) error
    TeardownFn func(ctx context.Context, prID int) error
    Calls      struct {
        Deploy   []runtime.DeployParams
        Teardown []int
    }
}

func (f *Fake) Deploy(ctx context.Context, p runtime.DeployParams) error {
    f.Calls.Deploy = append(f.Calls.Deploy, p)
    if f.DeployFn != nil { return f.DeployFn(ctx, p) }
    return nil
}
func (f *Fake) Teardown(ctx context.Context, prID int) error {
    f.Calls.Teardown = append(f.Calls.Teardown, prID)
    if f.TeardownFn != nil { return f.TeardownFn(ctx, prID) }
    return nil
}

var _ runtime.Runtime = (*Fake)(nil)
```

---

## 7. Dev Loop — `air` + parallel watchers

`task dev` starts three processes via Taskfile's parallel deps:

1. `go tool templ generate --watch` — produces `*_templ.go` on `*.templ` change.
2. `bin/tailwindcss --watch` — produces `internal/web/static/output.css`.
3. `go tool air -c .air.toml` — watches `*.go` (incl. generated `_templ.go`), rebuilds + restarts binary.

**No `templ --proxy` mode in MVP** — would conflict with SSE for build logs. Browser refresh = manual F5 until the panel grows enough to warrant the complication (refinement iteration, week 7+).

`.air.toml`:

```toml
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/prout ./cmd/prout"
  bin = "./tmp/prout"
  full_bin = "./tmp/prout server"
  include_ext = ["go"]
  exclude_dir = ["tmp", "bin", "vendor", ".task", "internal/web/static", "node_modules"]
  exclude_regex = [".*_templ\\.go$"]   # don't watch generated; templ-watch already handles
  delay = 500

[log]
  time = true

[color]
  main = "magenta"
  watcher = "cyan"
  build = "yellow"
  runner = "green"
```

`exclude_regex` for `_templ.go` files is intentional — the watch chain is: `.templ` → templ-watch writes `.templ.go` → if air also watched `.templ.go` we'd double-trigger.

---

## 8. Lint & Format — `golangci-lint` v2

Curated ~20-linter set. No `enable-all`. Focused on bug-detection (`bodyclose`, `sqlclosecheck`, `rowserrcheck`, `noctx`), error correctness (`errorlint`, `nilerr`, `wastedassign`), concurrency (`copyloopvar`, `contextcheck`), tests (`tparallel`, `testifylint`), and security (`gosec`).

**Excluded by design:** `varnamelen`, `funlen`, `gocyclo`, `gomnd`, `wsl`, `nlreturn`, `whitespace`, `godox`, `exhaustruct`, `lll`. See ADR-rationale in §8 of `prd.md` discussions.

`.golangci.yml`:

```yaml
version: "2"

run:
  timeout: 5m

linters:
  default: none
  enable:
    - govet
    - staticcheck
    - errcheck
    - ineffassign
    - unused
    - bodyclose
    - sqlclosecheck
    - rowserrcheck
    - noctx
    - errorlint
    - nilerr
    - wastedassign
    - copyloopvar
    - contextcheck
    - gosec
    - tparallel
    - testifylint
    - revive
    - misspell
    - unconvert
    - unparam

  settings:
    gosec:
      excludes:
        - G104    # already covered by errcheck
        - G304    # false positive on embed.FS
    revive:
      rules:
        - name: var-naming
        - name: error-return
        - name: error-naming
        - name: if-return
        - name: increment-decrement
        - name: errorf
        # `exported` rule deliberately disabled until end of MVP

formatters:
  enable:
    - gofumpt
    - gci
  settings:
    gci:
      sections:
        - standard
        - default
        - prefix(github.com/dominikpalatynski/prout)

issues:
  max-issues-per-linter: 0
  max-same-issues: 0
  exclude-rules:
    - path: _test\.go
      linters: [errcheck, gosec, revive, unparam]
    - path: ".+_templ\\.go$"
      linters: [revive, unused, unparam, gosec]
    - path: "internal/store/sqlc/.*"
      linters: [revive, unused, unparam]
```

Run via:
- IDE (`gopls` + golangci-lint LSP integration)
- `task lint` (full repo)
- CI (`ci.yml`)

---

## 9. Local Dev Infrastructure — `compose.dev.yml`

Postgres + River UI, started independently of `task dev` so the DB persists across binary restarts.

```yaml
# compose.dev.yml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: prout
      POSTGRES_PASSWORD: prout
      POSTGRES_DB: prout
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "prout"]
      interval: 2s
      timeout: 5s
      retries: 10

  riverui:
    image: ghcr.io/riverqueue/riverui:latest
    environment:
      DATABASE_URL: postgres://prout:prout@postgres:5432/prout?sslmode=disable
    ports: ["8081:8080"]
    depends_on:
      postgres: { condition: service_healthy }

  # Uncomment from week 4 onward (ADR-005 testing path).
  # docker-socket-proxy:
  #   image: tecnativa/docker-socket-proxy:0.3
  #   environment:
  #     CONTAINERS: 1
  #     NETWORKS: 1
  #     IMAGES: 1
  #     VOLUMES: 1
  #     EXEC: 1
  #     BUILD: 1
  #     EVENTS: 1
  #   volumes: ["/var/run/docker.sock:/var/run/docker.sock:ro"]
  #   ports: ["2375:2375"]   # ONLY for local dev — never expose in prod

volumes:
  pgdata: {}
```

Workflow:
- `task dev:up` once per work session.
- `task dev` (fast iteration loop).
- `task dev:reset` to wipe DB and re-migrate.

---

## 10. Pre-commit Hooks

**None.** Operator chose to skip git hooks. Lint and format run in IDE (live) and CI (gate). Operator may add `lefthook` later by dropping a `lefthook.yml` — no other code changes required.

---

## 11. Production Build

### 11.1 Generated code is **not committed**

`.gitignore` includes:
```
internal/web/templates/**/*_templ.go
internal/store/sqlc/
internal/web/static/output.css
tmp/
bin/
```

CI and Docker build run `task generate` + `task tailwind:build` before `go build`.

### 11.2 `deploy/Dockerfile` — multi-stage

```dockerfile
# syntax=docker/dockerfile:1.7

# ============================================
# Stage 1: tailwind binary
# ============================================
FROM alpine:3.19 AS tailwind
ARG TAILWIND_VERSION=v3.4.17
ARG TARGETARCH
RUN apk add --no-cache curl \
 && curl -sSL "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${TARGETARCH}" -o /tailwindcss \
 && chmod +x /tailwindcss

# ============================================
# Stage 2: build
# ============================================
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY --from=tailwind /tailwindcss /usr/local/bin/tailwindcss
COPY . .

ARG VERSION=dev
RUN go tool templ generate \
 && go tool sqlc generate \
 && tailwindcss \
      -i ./internal/web/styles/input.css \
      -o ./internal/web/static/output.css \
      --minify

RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/prout \
      ./cmd/prout

# ============================================
# Stage 3: runtime
# ============================================
FROM alpine:3.19 AS runtime
RUN apk add --no-cache \
      docker-cli \
      docker-cli-compose \
      ca-certificates \
      tzdata \
 && addgroup -S prout \
 && adduser -S prout -G prout -G docker

COPY --from=build /out/prout /usr/local/bin/prout

USER prout
WORKDIR /var/lib/prout
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/prout"]
CMD ["server"]
```

### 11.3 `compose.yml` — operator production stack

```yaml
# compose.yml — runs on the VPS
services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER_FILE: /run/secrets/postgres_user
      POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password
      POSTGRES_DB: prout
    volumes: ["pgdata:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $$(cat /run/secrets/postgres_user)"]
      interval: 5s
    secrets: [postgres_user, postgres_password]
    networks: [prout-internal]

  docker-socket-proxy:
    image: tecnativa/docker-socket-proxy:0.3
    restart: unless-stopped
    environment:
      CONTAINERS: 1
      NETWORKS: 1
      IMAGES: 1
      VOLUMES: 1
      EXEC: 1
      BUILD: 1
      EVENTS: 1
    volumes: ["/var/run/docker.sock:/var/run/docker.sock:ro"]
    networks: [prout-internal]

  traefik:
    image: traefik:v3
    restart: unless-stopped
    command:
      - --providers.docker=true
      - --providers.docker.endpoint=tcp://docker-socket-proxy:2375
      - --providers.docker.exposedbydefault=false
      - --entrypoints.websecure.address=:443
      - --entrypoints.web.address=:80
      - --entrypoints.web.http.redirections.entrypoint.to=websecure
      - --entrypoints.web.http.redirections.entrypoint.scheme=https
      - --certificatesresolvers.letsencrypt.acme.dnschallenge=true
      - --certificatesresolvers.letsencrypt.acme.dnschallenge.provider=cloudflare
      - --certificatesresolvers.letsencrypt.acme.email=${ACME_EMAIL}
      - --certificatesresolvers.letsencrypt.acme.storage=/acme/acme.json
    environment:
      CF_DNS_API_TOKEN_FILE: /run/secrets/cf_token
    ports: ["80:80", "443:443"]
    volumes: ["acme:/acme"]
    secrets: [cf_token]
    depends_on: [docker-socket-proxy]
    networks: [prout-traefik, prout-internal]

  prout:
    image: ghcr.io/dominikpalatynski/prout:latest
    restart: unless-stopped
    environment:
      DOCKER_HOST: tcp://docker-socket-proxy:2375
      prout_CONFIG: /etc/prout/server.yml
    volumes:
      - "./server.yml:/etc/prout/server.yml:ro"
      - "./secrets/github-private-key.pem:/etc/prout/private-key.pem:ro"
      - "workspaces:/var/lib/prout/workspaces"
    depends_on:
      postgres: { condition: service_healthy }
      docker-socket-proxy: { condition: service_started }
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.panel.rule=Host(`panel.preview.example.com`)"
      - "traefik.http.routers.panel.entrypoints=websecure"
      - "traefik.http.routers.panel.tls.certresolver=letsencrypt"
      - "traefik.http.services.panel.loadbalancer.server.port=8080"
      - "traefik.docker.network=prout-traefik"
    networks: [prout-traefik, prout-internal]

volumes:
  pgdata: {}
  acme: {}
  workspaces: {}

networks:
  prout-traefik: {}
  prout-internal: {}

secrets:
  postgres_user: { file: ./deploy/secrets/postgres_user }
  postgres_password: { file: ./deploy/secrets/postgres_password }
  cf_token: { file: ./deploy/secrets/cf_token }
```

**Security-relevant choices:** Postgres lives on `prout-internal` only — never reachable by Traefik or preview containers. Docker socket reaches the host only via the filtered proxy. Secrets are file-mounted (`docker secret`-style) so Postgres reads `_FILE` env vars without ever logging plaintext.

---

## 12. CI — GitHub Actions

Two workflows: `ci.yml` runs on every PR and push (PR-gate); `release.yml` runs on `main` and tags (image push). `amd64` only. No dependabot in MVP.

### 12.1 `.github/workflows/ci.yml`

```yaml
name: CI
on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - run: go tool templ generate
      - run: go tool sqlc generate
      - run: go tool golangci-lint run --timeout=5m

  test:
    runs-on: ubuntu-latest      # native Docker daemon for testcontainers
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - run: go tool templ generate
      - run: go tool sqlc generate
      - run: go tool gotestsum -- -race -count=1 ./...

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - run: go tool templ generate
      - run: go tool sqlc generate
      - name: Download Tailwind
        run: |
          curl -sSL https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-linux-x64 -o /tmp/tailwindcss
          chmod +x /tmp/tailwindcss
      - run: /tmp/tailwindcss -i ./internal/web/styles/input.css -o ./internal/web/static/output.css --minify
      - run: go build -o /tmp/prout ./cmd/prout
```

### 12.2 `.github/workflows/release.yml`

```yaml
name: Release
on:
  push:
    branches: [main]
    tags: ["v*"]

permissions:
  contents: read
  packages: write

jobs:
  image:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/dominikpalatynski/prout
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=sha,prefix=sha-
            type=semver,pattern={{version}}
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: deploy/Dockerfile
          push: true
          platforms: linux/amd64
          tags: ${{ steps.meta.outputs.tags }}
          build-args: |
            VERSION=${{ github.sha }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

### 12.3 Required PR checks

To be configured in GitHub branch protection on `main`:
- `lint`
- `test`
- `build`

---

## 13. Logging — `log/slog`

Three layers:

1. **Format selectable** by config: `json` (prod) or `text` (dev, via `lmittmann/tint`).
2. **Source attribution** opt-in via `add_source: true` (dev only).
3. **Context-aware handler** — extracts `request_id`, `pr_number`, `repository_id`, `river_job_id` from `ctx` automatically.

```go
// internal/log/log.go
package log

import (
    "context"
    "io"
    "log/slog"
    "os"

    "github.com/go-chi/chi/v5/middleware"
    "github.com/lmittmann/tint"
)

type Format string
const (
    FormatJSON Format = "json"
    FormatText Format = "text"
)

type Config struct {
    Level     slog.Level
    Format    Format
    AddSource bool
    Output    io.Writer
}

func New(cfg Config) *slog.Logger {
    if cfg.Output == nil { cfg.Output = os.Stdout }
    var base slog.Handler
    switch cfg.Format {
    case FormatText:
        base = tint.NewHandler(cfg.Output, &tint.Options{
            Level: cfg.Level, AddSource: cfg.AddSource, TimeFormat: "15:04:05.000",
        })
    default:
        base = slog.NewJSONHandler(cfg.Output, &slog.HandlerOptions{
            Level: cfg.Level, AddSource: cfg.AddSource,
        })
    }
    return slog.New(&ctxHandler{Handler: base})
}

type ctxHandler struct{ slog.Handler }

func (h *ctxHandler) Handle(ctx context.Context, r slog.Record) error {
    if reqID := middleware.GetReqID(ctx); reqID != "" {
        r.AddAttrs(slog.String("request_id", reqID))
    }
    if pr, ok := ctx.Value(prNumberKey{}).(int); ok {
        r.AddAttrs(slog.Int("pr_number", pr))
    }
    if repo, ok := ctx.Value(repoIDKey{}).(int64); ok {
        r.AddAttrs(slog.Int64("repository_id", repo))
    }
    if jobID, ok := ctx.Value(jobIDKey{}).(int64); ok {
        r.AddAttrs(slog.Int64("river_job_id", jobID))
    }
    return h.Handler.Handle(ctx, r)
}

func (h *ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &ctxHandler{Handler: h.Handler.WithAttrs(attrs)}
}
func (h *ctxHandler) WithGroup(name string) slog.Handler {
    return &ctxHandler{Handler: h.Handler.WithGroup(name)}
}

type (
    prNumberKey struct{}
    repoIDKey   struct{}
    jobIDKey    struct{}
)

func WithPRNumber(ctx context.Context, n int) context.Context  { return context.WithValue(ctx, prNumberKey{}, n) }
func WithRepoID(ctx context.Context, id int64) context.Context { return context.WithValue(ctx, repoIDKey{}, id) }
func WithJobID(ctx context.Context, id int64) context.Context  { return context.WithValue(ctx, jobIDKey{}, id) }
```

Usage convention: **always** `slog.InfoContext(ctx, ...)` (with `Context` suffix), never `slog.Info(...)`. Linter rule should enforce.

---

## 14. Data Layer — `sqlc` + `pgx` + `goose`

**Open question flagged by operator:** preference for a richer ORM (ent or GORM) instead of sqlc. See discussion at the end of this section. For now, this scaffold reflects the ADR-tech.md choice (sqlc + pgx). Switching to ent later is a localized refactor inside `internal/store/`.

### 14.1 Layout

```
prout/
├── migrations/
│   ├── 00001_initial_schema.sql
│   ├── 00002_repository_env_vars.sql
│   ├── 00003_river_init.go        # Go-based migrator (River v0.x)
│   └── 00004_*.sql
├── internal/store/
│   ├── store.go
│   ├── pool.go
│   ├── queries/                   # SOURCE — committed
│   │   ├── repositories.sql
│   │   ├── environments.sql
│   │   ├── webhook_events.sql
│   │   ├── audit_log.sql
│   │   └── allowed_users.sql
│   └── sqlc/                      # GENERATED — gitignored
└── sqlc.yaml
```

Migration numbering is **sequential** (`00001_*`), not timestamped. Solo project, no merge collision risk in MVP, easier to read in `goose status`.

### 14.2 `sqlc.yaml`

```yaml
version: "2"
sql:
  - engine: postgresql
    schema: migrations
    queries: internal/store/queries
    gen:
      go:
        package: sqlc
        out: internal/store/sqlc
        sql_package: pgx/v5
        emit_pointers_for_null_types: true
        emit_prepared_queries: false
        emit_interface: false
        emit_json_tags: false
        emit_db_tags: false
        emit_exact_table_names: true
        rename:
          pr_number: PRNumber
          pr_url: PRURL
          sha: SHA
          ttl: TTL
          url: URL
```

### 14.3 `*Store` wrapper + transaction helper

ADR-004 transactional coupling (state insert + River job insert in one transaction) is materialized in this single file:

```go
// internal/store/store.go
package store

import (
    "context"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/dominikpalatynski/prout/internal/store/sqlc"
)

type Store struct {
    pool    *pgxpool.Pool
    queries *sqlc.Queries
}

func New(pool *pgxpool.Pool) *Store {
    return &Store{pool: pool, queries: sqlc.New(pool)}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }
func (s *Store) Q() *sqlc.Queries    { return s.queries }

// Tx executes fn in a transaction with automatic rollback on error.
// For ADR-004 coupling, use this — never s.pool.BeginTx directly.
func (s *Store) Tx(ctx context.Context, fn func(*sqlc.Queries, pgx.Tx) error) error {
    tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer func() { _ = tx.Rollback(ctx) }()
    if err := fn(s.queries.WithTx(tx), tx); err != nil {
        return err
    }
    return tx.Commit(ctx)
}
```

Composite operations (cross-table or state+job) live as methods on `*Store`. Single-query reads use `s.Q().GetXByY(ctx, ...)` directly in handlers/workers.

### 14.4 `pgxpool` config

```go
// internal/store/pool.go
package store

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, err
    }
    cfg.MaxConns = 20
    cfg.MinConns = 2
    cfg.MaxConnIdleTime = 5 * time.Minute
    cfg.MaxConnLifetime = 1 * time.Hour
    cfg.HealthCheckPeriod = 30 * time.Second
    return pgxpool.NewWithConfig(ctx, cfg)
}
```

### 14.5 Goose + River migrator integration

`migrations/00003_river_init.go` (per ADR-010):

```go
// migrations/00003_river_init.go
package migrations

import (
    "context"
    "database/sql"

    "github.com/pressly/goose/v3"
    "github.com/riverqueue/river/riverdriver/riverdatabasesql"
    "github.com/riverqueue/river/rivermigrate"
)

func init() {
    goose.AddMigrationContext(upRiverInit, downRiverInit)
}

func upRiverInit(ctx context.Context, tx *sql.Tx) error {
    migrator, err := rivermigrate.New(riverdatabasesql.New(nil), nil)
    if err != nil { return err }
    _, err = migrator.MigrateTx(ctx, tx, rivermigrate.DirectionUp, nil)
    return err
}

func downRiverInit(ctx context.Context, tx *sql.Tx) error {
    migrator, err := rivermigrate.New(riverdatabasesql.New(nil), nil)
    if err != nil { return err }
    _, err = migrator.MigrateTx(ctx, tx, rivermigrate.DirectionDown, nil)
    return err
}
```

Single command (`task migrate:up`) covers both prout-owned and River-owned tables.

### 14.6 ent / GORM revisit (open)

The operator expressed preference for a richer ORM (ent or GORM) instead of sqlc. The scaffold currently reflects ADR-tech.md (sqlc + pgx). A future revisit should weigh:

- **For sqlc:** native pgx → clean LISTEN/NOTIFY for SSE build logs (ADR-003/004); River expects pgx; Postgres-specific features (JSONB, SKIP LOCKED) are first-class; sqlc is *not* raw inline SQL — queries live in `*.sql` files with name annotations and are codegen'd into typed Go methods.
- **For ent:** schema-first Go DSL; relationship traversal; mature codegen; same type-safety guarantee as sqlc.
- **Cost of ent on this stack:** ent uses `database/sql` driver, requiring River's `riverdatabasesql` driver path; LISTEN/NOTIFY drops down to raw `pgconn`; ent's preferred migration tool is `atlas`, not `goose` (ADR-010); for ~10 tables the schema/codegen volume is disproportionate.

**Decision until further notice:** sqlc. Switching to ent post-scaffold means rewriting `internal/store/` and `migrations/` tooling — localized, but non-trivial.

---

## Appendix A — Files to Create on First Scaffold

```
.editorconfig
.gitattributes
.gitignore
.air.toml
.golangci.yml
.mise.toml
LICENSE                                    # MIT
README.md
Taskfile.yml
sqlc.yaml
go.mod
server.yml.example
compose.dev.yml
compose.yml
deploy/Dockerfile
deploy/secrets/.gitkeep                    # actual secrets gitignored
.github/workflows/ci.yml
.github/workflows/release.yml
cmd/prout/main.go                        # cobra root, "server" subcommand
internal/config/config.go
internal/config/defaults.go
internal/config/validate.go
internal/log/log.go
internal/server/server.go
internal/server/routes.go
internal/server/middleware/.gitkeep
internal/server/handlers/.gitkeep
internal/store/store.go
internal/store/pool.go
internal/store/queries/.gitkeep
internal/runtime/runtime.go                 # interface
internal/runtime/dockercompose/.gitkeep
internal/runtime/runtimetest/.gitkeep
internal/githubapp/.gitkeep
internal/auth/.gitkeep
internal/preview/.gitkeep
internal/worker/.gitkeep
internal/webhook/.gitkeep
internal/audit/.gitkeep
internal/testdb/testdb.go
internal/web/embed.go
internal/web/styles/input.css
internal/web/templates/.gitkeep
migrations/00001_initial_schema.sql         # placeholder (week 1: just `CREATE TABLE _ping ()`)
migrations/00003_river_init.go              # numbered to leave room
```

---

## Appendix B — `.gitignore` minimum

```
# Build artifacts
/bin/
/tmp/

# Generated code (regenerated by `task generate`)
/internal/web/templates/**/*_templ.go
/internal/store/sqlc/
/internal/web/static/output.css

# Dev local
.env
.env.local

# Secrets
/deploy/secrets/postgres_user
/deploy/secrets/postgres_password
/deploy/secrets/cf_token
/deploy/secrets/*.pem
/server.yml

# IDE
.vscode/
.idea/
*.swp

# Mise / Task runtime
.task/
.mise.toolversions.lock

# OS
.DS_Store
```

---

## Appendix C — Open Questions / Future Revisits

| # | Topic | Trigger to revisit |
|---|---|---|
| 1 | sqlc → ent migration | If query patterns become relational-traversal heavy or if the operator's intuition still holds after week 4. |
| 2 | Multi-arch image (amd64+arm64) | First time a target VPS is ARM (Hetzner CAX, AWS Graviton). |
| 3 | Semver tags + `release-please` | When a second collaborator joins (PRD § Target Users) or external users start pulling specific versions. |
| 4 | Dependabot / Renovate | After first security advisory or after MVP ship. |
| 5 | `templ --proxy` mode | When panel has >10 views and F5-fatigue measurably hurts iteration speed. |
| 6 | Pre-commit hooks (`lefthook`) | If lint failures in CI become a recurring cost. |
| 7 | Coverage gating | Post-MVP, when test suite stabilizes. |
| 8 | OpenTelemetry tracing | Post-MVP (PRD § Out of Scope). |
| 9 | `revive.exported` rule | Post-MVP cleanup pass for public docs. |
| 10 | Encrypted secrets manager | When prod-grade secrets enter the system (ADR-008 deferred path). |

---

## Appendix D — First-Day Smoke Workflow

After `task setup && task dev:up && task migrate:up && task dev`:

1. `curl http://localhost:8080/healthz` → `{"status":"ok"}`
2. `curl -X POST http://localhost:8080/webhooks/github -H "X-GitHub-Event: ping" -d '{}'` → 401 (HMAC missing — expected)
3. River UI visible at `http://localhost:8081` (no jobs in queue).
4. Edit `internal/web/templates/home.templ` → templ-watch regenerates → air rebuilds → browser F5 shows change.

This is the "walking skeleton week 1" exit criterion at the scaffold layer.
