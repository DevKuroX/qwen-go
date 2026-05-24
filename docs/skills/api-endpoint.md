# Skill: API endpoint

## When to use
- Adding a new HTTP route under `/v1/*`, `/api/v1/*`, or `/api/admin/*`
- Wiring middleware (auth, CORS, body capture)
- Recording a request_log row from a handler (every chat-like handler must)

## Mental model
Every facade handler does the same dance: parse raw body → apply RTK+Caveman saver → route via `ProviderManager` → dispatch to provider → record one log row. The **defer + closure pattern** is the only correct way to record a log because the function has many early returns. Streaming handlers must return a `chatResult` struct so the parent can record one consolidated row.

## Files

| Symbol | File |
|--------|------|
| `RegisterChatRoutes` | `backend/internal/api/chat.go` (canonical pattern — copy this) |
| `RegisterClaudeRoutes`, `RegisterGeminiRoutes`, `RegisterResponsesRoutes` | `backend/internal/api/{claude,gemini,responses}.go` |
| `RegisterModelsRoutes` | `backend/internal/api/models.go` (read-only example) |
| `APIKeyMiddleware`, `AdminMiddleware` | `backend/internal/api/middleware.go` |
| `chatResult` struct, `recordRequestLog`, `readRawBody` | `backend/internal/api/chat.go` |
| `applySaver` | `backend/internal/api/saver.go` |
| Route wiring | `backend/internal/server/run.go` |

## Public API

```go
// Auth middleware — pick exactly one per route group
APIKeyMiddleware()   // /v1/* — accepts pool API keys
AdminMiddleware()    // /api/admin/* — accepts admin key only

// Body capture before consuming c.Request.Body
rawBody := readRawBody(c)

// Saver — run on raw body BEFORE you ShouldBindJSON
mutated, saverStats := applySaver(rawBody, formatOpenAIChat) // or formatAnthropic, formatGemini, formatOpenAIResponses
rawBody = mutated
c.Request.Body = io.NopCloser(bytes.NewReader(mutated))

// Log row — call exactly once per request via defer
recordRequestLog(feature, model, providerName, rawBody, status, errMsg, promptToks, completionToks, saverStats, start)
```

## Canonical handler skeleton

```go
func handleX(c *gin.Context) {
    start := time.Now()
    rawBody := readRawBody(c)
    mutated, saverStats := applySaver(rawBody, formatOpenAIChat)
    rawBody = mutated
    c.Request.Body = io.NopCloser(bytes.NewReader(mutated))

    result := chatResult{HTTPStatus: http.StatusOK}
    model := ""
    provName := ""
    defer func() {
        recordRequestLog("feature_name", model, provName, rawBody, result.HTTPStatus, result.ErrorMsg, result.PromptTokens, result.CompletionTokens, saverStats, start)
    }()

    var req ParsedRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": ...})
        result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: err.Error()}
        return
    }
    model = req.Model

    provider, resolved, err := providerManager.Route(req.Model)
    if err != nil { result = chatResult{HTTPStatus: 400, ErrorMsg: err.Error()}; return }
    req.Model = resolved
    provName = provider.Name()

    if req.Stream {
        result = streamX(c, provider, &req)  // streamX returns chatResult
        return
    }

    resp, err := provider.ChatCompletion(c.Request.Context(), &req)
    if err != nil { result = chatResult{HTTPStatus: 500, ErrorMsg: err.Error()}; return }
    // ... write JSON response ...
    result = chatResult{PromptTokens: p, CompletionTokens: c, Success: true, HTTPStatus: 200}
}
```

## Wiring

In `backend/internal/server/run.go`, next to the other `api.Register*` calls:

```go
api.RegisterXRoutes(r)
```

In your `RegisterXRoutes`:

```go
func RegisterXRoutes(r *gin.Engine) {
    g := r.Group("/")
    g.Use(APIKeyMiddleware())     // or AdminMiddleware
    g.POST("/v1/x", handleX)
}
```

## Invariants — DO NOT BREAK

1. **Every chat-like handler records exactly one log row** — never zero, never two
2. **Streaming sub-handlers return `chatResult`** — the SSE writer can't re-read the body, so stats must thread up
3. **`readRawBody` rewinds `c.Request.Body`** — after calling, `ShouldBindJSON` still works
4. **Saver runs on raw body before parsing** — RTK mutates message content shapes that disappear after flatten
5. **`req.Model = resolved` overwrites with the prefix-stripped name** — providers that need the original (e.g. variant alias) must capture it before this line, OR resolve again inside the provider (see opencode-zen pattern)

## Common edits

- **Add a new format facade**: copy `responses.go`, swap `applySaver`'s format constant, wire `RegisterFooRoutes` in `run.go`
- **Add an admin endpoint**: use `g.Use(AdminMiddleware())`, mount under `/api/admin/x`; no recordRequestLog needed
- **Forward request params upstream**: extend `models.ChatRequest` (`backend/internal/models/request.go`), then read in provider's `buildPayload`
- **Reject unknown model variant early**: validate in provider's `ChatCompletion`/`ChatStream` entry (see `opencodezen/provider.go` `validateModel`)

## Gotchas

- **`/v1/models/*tail` is owned by Gemini** — its catch-all swallows `/v1/models/x` GETs. Our `/v1/models` GET is on a different group, so it works, but don't add new `/v1/models/...` POST routes outside Gemini's group
- **Provider routing requires `prefix/model`** — `qwen3.6-plus` alone fails; must be `qw/qwen3.6-plus`. Errors look like `invalid model format` or `provider not found for prefix`
- **`X-Admin-Key` header is NOT supported** — admin auth goes through `extractAuth` which reads `Authorization: Bearer`, `x-api-key`, or `?key=` query. Same shape as `/v1/*`
- **Gemini auth quirk** — checks `Authorization → x-api-key → ?key=`, NOT `x-goog-api-key`. Documented divergence from Google's spec
- **Don't skip hooks** when committing handler changes — pre-commit hook lints route registration

## Cross-skill

- [provider-add](provider-add.md) — to dispatch to a new provider from your handler
- [account-pool](account-pool.md) — providers acquire accounts; handlers don't touch the pool directly
- [settings](settings.md) — to gate a handler behind a dynamic setting
