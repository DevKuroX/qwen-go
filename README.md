# qwen-go

Multi-provider AI gateway. Aggregates free-tier accounts from Qwen, OpenCode Zen, Gemini Web, and Kiro behind a single OpenAI-compatible API + an embedded admin dashboard.

## What you get

- **OpenAI-compatible endpoints** — `/v1/chat/completions`, `/v1/responses`, `/v1/embeddings`, `/v1/models`
- **Claude-compatible endpoint** — `/v1/messages`
- **Gemini-compatible endpoint** — `/v1/models/*tail` (`generateContent` / `streamGenerateContent`)
- **Image generation** — Qwen-backed via OpenAI shape
- **Account pool** — round-robin across N accounts per provider, score-based reranking, automatic ban detection
- **Model aliases** — variants like `zen/deepseek-v4-flash-free-thinking` map to a base model + preset reasoning params
- **RTK + Caveman compression** — per-tool-output filter chain + system-prompt brevity injection (port of [9router](https://github.com/.../9router))
- **Dashboard** — embedded in the binary, served on `:1441`, lives at `/dashboard/...`

## Quick start

```bash
# 1. Build (one-time, after every code change)
bash scripts/build-frontend.sh                    # static-export Next.js → backend embed dir
cd backend && go build -o ../qwen-go ./cmd/qwen-go/

# 2. Run
cd .. && ./qwen-go start                          # API on :1440, dashboard on :1441
```

Logs: `tail -f /tmp/qwen-go.log`

## Rebuild + restart (the only way — do not manually kill or cp)

Never `pkill && cp newbin && nohup` — `cp` over a running binary fails with "Text file busy" and the manual ordering is error-prone. Always use the `restart` subcommand:

```bash
# Backend-only change
cd backend && go build -o ../qwen-go ./cmd/qwen-go/ && cd .. && ./qwen-go restart

# Frontend change (or both)
bash scripts/build-frontend.sh \
  && cd backend && go build -o ../qwen-go ./cmd/qwen-go/ \
  && cd .. && ./qwen-go restart

# Full prod redeploy (also runs python setup + nginx reload)
bash scripts/deploy.sh
```

The frontend is **statically exported** (`NEXT_OUTPUT=export`) and **embedded into the Go binary**. There is no separate Node server in production — one binary serves both `:1440` (API) and `:1441` (dashboard).

## Endpoints

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/chat/completions`, `/v1/chat/completions` | API key | OpenAI chat |
| POST | `/v1/responses` | API key | OpenAI Responses API |
| POST | `/v1/messages` | API key | Anthropic Claude |
| POST | `/v1/models/*tail` | API key | Google Gemini |
| POST | `/v1/embeddings` | API key | Embeddings |
| GET | `/v1/models`, `/api/v1/models` | API key | Aggregated model catalog |
| `*` | `/api/admin/*` | Admin key | Dashboard + admin ops |
| `*` | `/api/auth/*` | Cookie | Dashboard login |

Authentication header for `/v1/*`: `Authorization: Bearer <api_key>` OR `x-api-key: <api_key>` OR `?key=<api_key>` query param. Admin endpoints accept the same shapes but require the admin key (not pool keys).

## Configuration

- `data/auth.json` — admin key + dashboard cookie
- `data/api_keys.json` — pool of accepted `/v1/*` API keys
- `data/provider_configs.json` — per-provider endpoint, headers, models, aliases
- `data/settings.json` — RTK / Caveman flags, max-inflight, moemail
- `data/accounts.json` — provider accounts (cookies / tokens)
- `data/qwen-go.db` — SQLite for request logs + usage history

## Providers

| Provider | Prefix | Auth mode | Default models |
|----------|--------|-----------|----------------|
| Qwen | `qw/` | bearer | `qwen3.6-max-preview`, `qwen3.6-plus`, `qwen3.6-27b` |
| OpenCode Zen | `zen/` | ephemeral-session | `deepseek-v4-flash-free` + 3 more free models discovered live |
| Gemini Web | `gw/` | cookie-pair-scraped | `gemini-2.5-pro`, `gemini-2.5-flash` |
| Kiro | `kiro/` | refresh-access | `claude-3-5-sonnet-20241022`, `claude-3-7-sonnet-20250219` |

Model IDs are always `prefix/name` on the wire (e.g. `zen/deepseek-v4-flash-free`).

## For AI coding agents

Read `AGENTS.md` for repo conventions, `RULES.md` for hard rules (off-limits files, rebuild policy, no-commit-without-permission), and **`docs/skills/INDEX.md`** for per-topic cheat sheets covering common edits (add an endpoint, add a provider, account pool semantics, etc). Always check the skills index before coding.

## Repo layout

```
backend/
  cmd/qwen-go/         CLI entry (start, stop, restart, account, key, engine)
  internal/api/        HTTP handlers
  internal/core/       Pools, settings, registry, RTK, Caveman, batch
  internal/providers/  Per-provider clients
  internal/server/     Gin router + embed
frontend/
  app/(admin)/         Dashboard pages
  app/api/             Next.js API routes (proxied to backend at runtime, moved out for static export)
scripts/
  build-frontend.sh    Static export + copy into backend embed
  deploy.sh            Full prod rebuild + restart + nginx
data/                  Runtime state (gitignored)
docs/archive/          Phase docs, historic design notes, old READMEs
```

## License

Personal use only.
