# Skill: Add / modify a provider

## When to use
- Adding a new upstream (cookie / bearer / refresh / ephemeral-session)
- Adding model aliases (variants like `deepseek-v4-flash-free-thinking`)
- Plumbing new request params (reasoning_effort, etc) to a provider
- Debugging why a provider isn't getting routed

## Mental model
Providers are independent packages under `backend/internal/providers/<name>/`. Each implements `models.Provider` (required) and optionally `models.AuthProvider` (for non-static auth). The **registry** (`provider_registry.go`) is the source of truth for HTTP-level config (endpoint, headers, models, aliases) — defaults seeded in code but **disk wins on Load**, so editing `data/provider_configs.json` is the runtime knob. The **manager** routes `prefix/model` → provider instance.

## Files

| Symbol | File |
|--------|------|
| `Provider` interface | `backend/internal/models/provider.go` (Name/Prefix/ChatCompletion/ChatStream/Health/ResolveModel/Metadata) |
| `AuthProvider` interface | same file (BuildAuthHeaders/EnsureFresh/RefreshAuth — opt-in) |
| `ProviderConfig`, `ModelAlias`, `defaultProviderConfigs` | `backend/internal/core/provider_registry.go` |
| `ProviderManager.Route`, `pm.Register` | `backend/internal/core/provider_manager.go` |
| Provider registration | `backend/internal/server/run.go` (search `pm.Register`) |
| Existing providers (copy whichever matches your auth shape) | `backend/internal/providers/{qwen,opencodezen,geminiweb,kiro,openai}/provider.go` |

## Auth shape → which provider to copy

| Upstream auth | Copy this | Notes |
|--------------|-----------|-------|
| Static bearer / no rotation | `openai/` (stub) | Simplest |
| Cookie blob | `qwen/` | Cookie scrape + refresh |
| Bearer + refresh-token rotation | `kiro/` | AWS-style refresh flow |
| Ephemeral session UUID (client-generated) | `opencodezen/` | Per-session quota, retire-on-fail, lazy spawn |
| Cookie pair (multi-cookie scrape) | `geminiweb/` | Python capture script + scraped UUIDs |

## Public API a new provider must implement

```go
// Required: models.Provider
func (p *MyProvider) Name() string { return "myname" }
func (p *MyProvider) Prefix() string { return "my/" }
func (p *MyProvider) Type() models.ProviderType { return models.ProviderTypePublic }
func (p *MyProvider) ResolveModel(model string) string { return strings.TrimPrefix(model, "my/") }
func (p *MyProvider) Metadata() models.ProviderMetadata { ... }
func (p *MyProvider) ChatCompletion(ctx, req) (*models.ChatResponse, error) { ... }
func (p *MyProvider) ChatStream(ctx, req) (<-chan models.StreamChunk, error) { ... }
func (p *MyProvider) ImageGeneration(ctx, req) (*models.ImageResponse, error) { return nil, errors.New("unsupported") }
func (p *MyProvider) VideoGeneration(ctx, req) (*models.VideoResponse, error) { return nil, errors.New("unsupported") }
func (p *MyProvider) Health(ctx) error { ... }

// Optional: models.AuthProvider (only if not static-bearer)
func (p *MyProvider) BuildAuthHeaders(acc *models.Account) map[string]string { ... }
func (p *MyProvider) EnsureFresh(acc *models.Account) error { ... }
func (p *MyProvider) RefreshAuth(acc *models.Account) error { ... }
```

## End-to-end recipe — adding a provider

1. **Seed config** in `defaultProviderConfigs()` (`provider_registry.go`):
   ```go
   "myname": {
       Name: "myname",
       BaseEndpoint: "https://...",
       Headers: map[string]string{...},
       AuthMode: "bearer",  // or "refresh-access", "cookie-blob", "cookie-pair-scraped", "ephemeral-session"
       AvailableModels: []string{"model-a", "model-b"},
       Capabilities: models.ProviderCapabilities{SupportsChat: true, SupportsStream: true},
   },
   ```
2. **Implement** the package at `backend/internal/providers/myname/provider.go`
3. **Register** in `server/run.go`:
   ```go
   pm.Register(myname.NewProvider(pool, registry))
   ```
4. **Disk seed**: if `data/provider_configs.json` already exists, the in-memory seed is shadowed. Either delete that file (registry will rewrite from defaults) OR add your block to the file manually. **Load merges built-in defaults that aren't in the file**, so for *new* providers no manual edit needed; for *modifications* to an existing provider (e.g. adding `model_aliases`), edit the file.
5. **Smoke**: `curl -X POST /api/v1/chat/completions ... '{"model":"my/model-a", ...}'`

## Model aliases (variants)

Currently only **opencode-zen** consumes aliases. Pattern in `provider_configs.json`:

```jsonc
"model_aliases": {
  "deepseek-v4-flash-free-thinking": {
    "base": "deepseek-v4-flash-free",
    "params": { "reasoning_effort": "high", "verbosity": "low" }
  }
}
```

Resolution flow inside the provider (see `opencodezen/provider.go` `lookupAlias` + `singleAttempt`):
- `ResolveModel` strips prefix only — leaves variant name intact
- `singleAttempt` calls `lookupAlias(stripped)`; on hit, sets `upstreamModel = alias.Base` and grabs `aliasParams`
- `buildPayload` merges `aliasParams` (defaults) then overrides with request body fields — **request override wins**
- `validateModel` rejects unknown variants early with a clear 400-style error

To add variants to a NEW provider: copy the four hooks (`lookupAlias`, `isKnownModel`, `validateModel`, alias-aware `buildPayload`) from `opencodezen/provider.go`. Each provider decides its own param names — don't assume `reasoning_effort` is universal.

## `/v1/models` discoverability

`backend/internal/api/models.go` `handleListModels` already aggregates:
- `AvailableModels[]` from every provider
- `Models{}` map (Gemini Web UUIDs) from every provider
- `ModelAliases{}` keys from every provider
- Live-fetched `-free` models from opencode-zen catalog (5-min cache)

New providers get listed automatically. No code change in `models.go` needed unless you want custom dynamic discovery.

## Invariants — DO NOT BREAK

1. **Prefix must end with `/`** — `qw/`, `zen/`, not `qw`
2. **Registry seed and disk both supply config** — disk wins for existing entries, defaults fill gaps. So new providers ship via code; runtime tweaks ship via JSON
3. **`ChatRequest.Model` arrives prefix-stripped to the provider** — `chat.go` does `req.Model = resolvedModel` before calling provider. If you need the original (variant lookup), don't rely on `req.Model` — `ResolveModel` should NOT collapse the variant for you (see opencode-zen pattern)
4. **`ImageGeneration`/`VideoGeneration` must exist** even if unsupported — return `errors.New("unsupported")`, don't omit the method
5. **Account pool is shared** — your provider gets accounts via `p.pool.Acquire(ctx, "myname", exclude)`. Don't maintain a per-provider account list

## Common edits

- **Add a model to an existing provider**: append to `available_models[]` in `data/provider_configs.json`. No restart needed if dashboard reloads registry; full restart safer
- **Add a variant**: append to `model_aliases{}` in the same file. opencode-zen only
- **Forward a new request param** (e.g. `top_p`):
  1. Add field to `models.ChatRequest` (`backend/internal/models/request.go`)
  2. Read in provider's `buildPayload`
  3. Param flows through automatically — `c.ShouldBindJSON` picks it up
- **Reject unknown models early**: add `validateModel` helper, call in `ChatCompletion` + `ChatStream` first thing

## Gotchas

- **Disk shadows seed forever** — once `data/provider_configs.json` has an entry, code changes to the seed don't apply on restart. Edit the file, or delete it to regenerate
- **`ProviderManager.Route` calls `provider.ResolveModel`** before returning. By the time your provider sees `req.Model`, it's already been ResolveModel'd. Variant logic needs to handle this (don't collapse variant → base in ResolveModel; do it inside the request build)
- **`isKnownModel` should accept both base + aliases** — `validateModel` uses it; otherwise valid variants get rejected
- **Account pool's `MarkError` semantics** — see [account-pool](account-pool.md). Don't call it on ambiguous failures (network blips); only on confirmed-bad-account signals (401, 403, banned)
- **Stream cancellation** — providers must respect `ctx.Done()`. Forgetting this leaks goroutines on client disconnect

## Cross-skill

- [api-endpoint](api-endpoint.md) — how handlers dispatch to your provider
- [account-pool](account-pool.md) — how your provider acquires accounts
- [settings](settings.md) — global flags your provider may need to read
