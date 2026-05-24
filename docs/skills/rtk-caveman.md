# Skill: RTK + Caveman (request saver)

## When to use
- Debugging "why didn't compression fire on this request?"
- Adding a new filter (new tool-output shape worth compressing)
- Adding a new format facade (must call `applySaver` correctly)
- Tweaking caveman prompts or adding a level
- Verifying byte-for-byte parity with 9router

## Mental model

**Two independent transforms, applied once at request entry, on the raw JSON body before flatten:**

- **RTK** = compress `tool_result` / `role:"tool"` / `function_call_output` content with per-tool filters. Autodetect picks the filter from the first 1024 chars. Always-on per request when `rtk_enabled`. Skips `is_error:true` blocks. Safety: MIN=500B, RAW_CAP=10MB, never-grow, never-empty.
- **Caveman** = inject a terse-style instruction into the system prompt. Always-on per request when `caveman_enabled` and `level ∈ {lite, full, ultra}`. Format-aware injection (OpenAI messages, OpenAI Responses instructions, Anthropic `body.system`, Gemini `systemInstruction`).

**Cut-point is the raw map[string]any body**, not `models.ChatRequest`. By the time the request hits a handler's `ShouldBindJSON`, multi-block content shapes have collapsed. Both packages operate on `map[string]any` so the original `tool_result` / `parts[]` / `content[]` arrays survive.

## Files

| Symbol | File |
|--------|------|
| Settings flags `rtk_enabled`, `caveman_enabled`, `caveman_level` | `backend/internal/core/settings_manager.go` |
| Saver dispatch (`applySaver`, format constants) | `backend/internal/api/saver.go` |
| Bundled stats (`RequestSaverStats.CompactionMode`) | `backend/internal/core/saver_stats.go` |
| **RTK** package | `backend/internal/core/rtk/` |
| `CompressOpenAIChat/Responses/Anthropic/Gemini` entry points | `rtk/apply.go` |
| `compressText` (size guards + autodetect + never-grow) | `rtk/compress.go` |
| `AutoDetect` (regex chain, RE2-compatible) | `rtk/autodetect.go` |
| `LookupFilter`, filter `Stats`, `Hit` | `rtk/registry.go`, `rtk/stats.go` |
| Filter constants (caps + name strings) | `rtk/constants.go` |
| Per-filter logic | `rtk/filter_<name>.go` (git_diff, git_status, grep, find, tree, ls, search_list, read_numbered, dedup_log, smart_truncate) |
| **Caveman** package | `backend/internal/core/caveman/` |
| `Inject{OpenAIChat,OpenAIResponses,Anthropic,Gemini}` | `caveman/inject.go` |
| `Prompts` map, `Prompt(level)`, `Sep` | `caveman/prompts.go` |
| Frontend display | `frontend/app/(admin)/dashboard/console/components/RequestDetailModal.tsx` (violet RTK / fuchsia Caveman) |
| Log schema (`compaction_mode`, `caveman_level`, `rtk_bytes_before/after`, `rtk_filters`, `saver_reduction_pct`) | `backend/internal/core/request_log_tracker.go` |

## Public API

```go
// One-shot saver — call at handler entry, BEFORE ShouldBindJSON
mutated, stats := applySaver(rawBody, formatOpenAIChat)
rawBody = mutated
c.Request.Body = io.NopCloser(bytes.NewReader(mutated))

// Format constants (api/saver.go) — pass exactly one
formatOpenAIChat       // /v1/chat/completions
formatOpenAIResponses  // /v1/responses
formatAnthropic        // /v1/messages
formatGemini           // /v1/models/*tail (generateContent)

// Stats struct — bundled into recordRequestLog
type RequestSaverStats struct {
    RTK     rtk.Stats     // BytesBefore, BytesAfter, Hits[]
    Caveman caveman.Stats // Enabled, Level
}
stats.CompactionMode()  // "rtk+caveman:full", "rtk", "caveman:ultra", or ""
stats.RTK.FilterNames()  // ["git-diff", "grep"]
stats.RTK.ReductionPct() // 74
```

## How a request flows

```
POST /v1/messages
  ↓ readRawBody(c)
  ↓ applySaver(rawBody, formatAnthropic)
     ├─ SettingsManager.SaverFlags() → (rtkOn, cavOn, level)
     ├─ json.Decode → map[string]any   (UseNumber to preserve precision)
     ├─ if rtkOn:     rtk.CompressAnthropic(body)   ← mutates in place
     ├─ if cavOn:     caveman.InjectAnthropic(body, level)   ← mutates in place
     └─ json.Marshal → mutated bytes
  ↓ c.Request.Body = NopCloser(mutated)
  ↓ ShouldBindJSON(&req)   ← parses the mutated body; flatten collapses arrays
  ↓ provider dispatch
  ↓ defer recordRequestLog(..., stats, ...)   ← stats persisted on the log row
```

## RTK filter chain (autodetect order)

`AutoDetect(text)` (rtk/autodetect.go) tries patterns in this exact order, returns first match:

1. **git-diff** — `^diff --git`
2. **git-status** — porcelain headers
3. **git-log** — `^commit [a-f0-9]{40}`
4. **grep** — `path:line:content` ratio
5. **search-list** — bare paths + dir grouping
6. **find** — `^./` or `^/` line dominance
7. **ls** — short-form path/dir listing
8. **tree** — `├──` `└──` glyphs
9. **read-numbered** — `nn→ content` ratio above `ReadNumberedMinHitRatio` (0.7)
10. **dedup-log** — repetitive timestamped lines
11. **smart-truncate** — fallback (line count > `SmartTruncateMinLines`, keep head/tail)

If no filter fires, the text is returned unchanged (also bumps `BytesBefore == BytesAfter`).

## Invariants — DO NOT BREAK

1. **Saver runs BEFORE flatten** — call `applySaver` on raw bytes, mutate body, re-marshal, then `ShouldBindJSON`. Reversed order = arrays already collapsed = saver no-op.
2. **Never-grow / never-empty** (`compressText` in `rtk/compress.go`) — if filter output is longer than input or empty, original is returned. Don't bypass; it's the safety net for filter bugs.
3. **MIN_COMPRESS_SIZE = 500** — bodies under 500B skip compression entirely. Don't lower without thinking; tiny tool outputs aren't worth the regex pass.
4. **RAW_CAP = 10MB** — bodies over 10MB also skip (would block the goroutine). Real tool outputs above this are pathological.
5. **`is_error: true` blocks are skipped** (Claude tool_result invariant) — caller stack traces shouldn't be RTK'd.
6. **All `^...$` regexes use `(?m)` multiline flag** — Go RE2 requires this explicitly. Forgetting = whole-blob match attempts that don't fire.
7. **Caveman injection format-specific**:
   - OpenAI chat: append to first `role:"system"` / `"developer"` message; unshift new system if none
   - Responses: prefer `body.instructions` string; fallback to walking `body.input[]`
   - Anthropic: string → concat; array → insert NEW `{type:"text"}` BEFORE the last block carrying `cache_control` (so cache key stays stable on the last block)
   - Gemini: `systemInstruction` camelCase or `system_instruction` snake_case, under `body.*` or `body.request.*`
8. **Pointer-bool gating** in SettingsManager — `rtkOn` and `cavOn` are bools resolved via `SaverFlags()` which handles nil pointers. Don't deref directly.
9. **`saver_reduction_pct` is RTK bytes only** — Caveman adds bytes, excluded by design. Don't conflate.
10. **9router parity is a hard requirement** — filter logic ports line-for-line. Reference: `/home/ubuntu/ai_proxy/_ref/9router/open-sse/rtk/`. Drift = bug.

## Common edits

- **Add a new format facade**: extend `applySaver` switch with the new constant; implement `rtk.Compress<Format>` walking the format-specific message shape; implement `caveman.Inject<Format>` per the prompt-injection convention. Test against a 9router fixture.
- **Add a new filter**:
  1. New file `rtk/filter_<name>.go` with `func <name>(text string) string`
  2. Register in `rtk/registry.go` (`LookupFilter` table)
  3. Add detector in `rtk/autodetect.go` (place in the chain at the right priority)
  4. Add name constant in `rtk/constants.go`
  5. Golden test under `rtk/filter_<name>_test.go` with `*.in` / `*.out` fixtures from 9router
- **Tweak a caveman prompt**: edit `Prompts` map in `caveman/prompts.go`. **All three levels embed `sharedBoundaries`** — don't synthesize a separate constant, copy-and-modify the full string when 9router updates.
- **Add a new caveman level**: add constant in `prompts.go`, add map entry, frontend `settings/page.tsx` adds button to the segmented control.
- **Disable saver for a specific endpoint**: don't call `applySaver` in that handler. The middleware-style invocation is per-handler explicit.

## Verifying parity with 9router

```bash
# 1. Find or create input fixture (e.g. a 32k git diff)
# 2. Run through 9router locally:
node -e "const f = require('/home/ubuntu/ai_proxy/_ref/9router/open-sse/rtk/gitDiff.js'); console.log(f(require('fs').readFileSync('input.txt','utf8')))" > expected.txt
# 3. Run through qwen-go:
cd backend && go test ./internal/core/rtk/ -run TestGitDiff -v
# 4. Or via live HTTP:
curl -X POST /api/v1/chat/completions -d '...' && check request_log row's rtk_filters + saver_reduction_pct
md5sum input.txt expected.txt   # compare against captured Go output
```

Known parity fixtures live under `rtk/testdata/` (one dir per filter, `*.in` + `*.out`).

## Gotchas

- **`UseNumber()` matters** — `applySaver` uses it so integer fields (max_tokens, etc) don't get float-64'd through marshal/unmarshal. Don't drop this when refactoring.
- **`is_error` is the Anthropic guard, not OpenAI** — RTK only checks it on Claude tool_result blocks. OpenAI `role:"tool"` has no such field; all tool outputs compress unconditionally there.
- **`read-numbered` ratio threshold** — needs `>= 70%` of lines matching `^\s*\d+→`. Edge case: 3-line outputs with 2 matches = 67%, skipped. Adjust `ReadNumberedMinHitRatio` if needed.
- **Caveman + cache_control on Anthropic** — historical bug: appending caveman block AFTER the cache_control block invalidated the prompt cache key. Fix: insert BEFORE. If you refactor `injectClaudeSystem`, preserve this invariant explicitly.
- **`compaction_mode` column reuse** — schema kept the legacy name; values now encode both saver kinds (`"rtk+caveman:full"`, `"rtk"`, `"caveman:ultra"`, or `""`). Reader code must parse this string, not assume the legacy single-enum shape.
- **`saver_reduction_pct = 0` doesn't mean "saver off"** — could mean RTK ran but found nothing compressible (small body, no tool outputs). Check `compaction_mode` for the truth signal.
- **Stream paths can't re-read body** — saver MUST run before any streaming kicks off. The `defer recordRequestLog` pattern threads the stats struct through the handler return.
- **Frontend renders saver state from request_log row** — if you change `RequestSaverStats.CompactionMode()` output, update `RequestDetailModal.tsx` rendering. They're coupled by string.

## Cross-skill

- [api-endpoint](api-endpoint.md) — every facade handler must call `applySaver`
- [settings](settings.md) — `rtk_enabled` / `caveman_enabled` / `caveman_level` live there, including legacy `token_saver_mode` migration
- [provider-add](provider-add.md) — saver runs before provider dispatch; new providers get saver behavior automatically through their facade handler
