# Agent rules for qwen-go

Read `README.md` for what this project is. Read `RULES.md` for the **hard** rules. This file is the quick-reference for working in the codebase.

## Before coding — check the skills index

**Always read `docs/skills/INDEX.md` first.** It lists per-topic cheat sheets (one file per area: API endpoints, providers, account pool, settings, request compression, …). Find the skill matching your task and read **that one file** instead of grepping the codebase blind. Missing your topic? Grep, then **add a new skill file** so the next agent doesn't repeat your search.

## Layout cheat-sheet

```
backend/internal/api/        HTTP handlers — chat.go, claude.go, gemini.go, responses.go, embeddings.go, models.go, admin.go, account.go, auth.go, …
backend/internal/core/       AccountPool, KeyManager, SettingsManager, ProviderRegistry, ProviderManager, RTK, Caveman, batch, request_log_tracker
backend/internal/providers/  qwen/, opencodezen/, geminiweb/, kiro/, openai/ — one package per upstream
backend/internal/models/     Shared structs (ChatRequest, ChatResponse, Account, Provider interface, …)
backend/internal/server/     run.go wires everything + serves the embedded dashboard
backend/cmd/qwen-go/         CLI entry; start/stop/restart subcommands live here
frontend/app/(admin)/        Dashboard — Next.js 16 with breaking changes (see frontend/AGENTS.md)
data/                        Runtime JSON + SQLite, gitignored
docs/archive/                Old phase plans + design docs — historical only, do not edit
```

## Rebuild + restart

**Use the script and the CLI. Never `pkill && cp && nohup`.**

```bash
# Backend only:
cd backend && go build -o ../qwen-go ./cmd/qwen-go/ && cd .. && ./qwen-go restart

# Frontend (or both):
bash scripts/build-frontend.sh \
  && cd backend && go build -o ../qwen-go ./cmd/qwen-go/ \
  && cd .. && ./qwen-go restart
```

The frontend is `next export`-ed into `backend/internal/server/dashboard/` and embedded into the Go binary at build time. One binary serves API (`:1440`) and dashboard (`:1441`).

Logs: `/tmp/qwen-go.log`.

## Common workflows

### Add a new endpoint
1. Handler in `backend/internal/api/<feature>.go` with a `Register<Feature>Routes(r *gin.Engine)` function
2. Wire it in `backend/internal/server/run.go` next to the other `api.Register*` calls
3. Auth: `g.Use(APIKeyMiddleware())` for `/v1/*`, `AdminMiddleware()` for `/api/admin/*`

### Add a provider model
- Static models: `data/provider_configs.json` → `available_models[]`
- Variants (alias → base + preset params): `data/provider_configs.json` → `model_aliases{}` — currently only `opencode-zen` consumes these
- Live-discovered (e.g. opencode-zen `-free` catalog): already auto-merged into `/v1/models` from `https://opencode.ai/zen/v1/models`

### Add a provider
1. New package under `backend/internal/providers/<name>/` implementing the `models.Provider` interface (`Name`, `Prefix`, `ResolveModel`, `ChatCompletion`, `ChatStream`, `Health`, `BuildAuthHeaders`, etc.)
2. Seed defaults in `backend/internal/core/provider_registry.go` `defaultProviderConfigs()`
3. Register in `backend/internal/server/run.go` (`pm.Register(...)`)

### Inspect a request after the fact
- `GET /api/admin/request-logs?limit=N` (admin auth) — recent log rows
- `GET /api/admin/request-logs/:id` — one row with full body + saver stats
- Dashboard: `/dashboard/console/request-log`

## Code style

- Go: standard `gofmt`. No emojis in source. Comments explain **why**, not what.
- Concurrency: account pool uses `sync.RWMutex`; provider config registry is read-mostly with `sync.RWMutex`. Don't bypass them.
- Error wrapping: `fmt.Errorf("context: %w", err)` — never bare `errors.New` for wrapping.
- HTTP: every handler must record one `recordRequestLog(...)` row via the `defer + closure` pattern (see `chat.go`, `claude.go`).

## Off-limits files

See `RULES.md` for the full list. Short version:
- `backend/internal/api/auth.go`, `backend/internal/api/middleware.go`
- `frontend/app/api/auth/*`, `frontend/app/(auth)/login/*`
- `data/auth.json`, `data/api_keys.json`, cookies named `qwenpi_key` / `qwenpi_apikey`

If you find a bug there, report it — do not patch.

## Frontend warning

`frontend/AGENTS.md` says: *"This is NOT the Next.js you know — has breaking changes — read the relevant guide in `node_modules/next/dist/docs/` before writing any code."* Take it seriously.
