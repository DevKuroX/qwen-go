# Skill: Account pool

## When to use
- Adding ban/recovery logic for a new provider
- Tweaking acquire/release semantics
- Debugging "no account available" errors
- Adjusting score reranking or inflight cap
- Touching the snapshot loop or persistence

## Mental model
**One pool, shared across all providers.** Accounts are tagged by `Provider` field; `Acquire(providerName, exclude)` filters by tag. The pool is a **min-heap by negative score** (highest score wins) plus a flat map keyed by email. Selection is greedy on score; bans/rate-limits filter out at `isAvailable` time. Recovery runs in a background goroutine. Snapshot to disk runs in another goroutine — critical transitions (Banned, CircuitOpen) flush immediately, other writes batch every N seconds.

Status state machine:
```
VALID ──auth→ SOFT_ERROR ──5×→ CIRCUIT_OPEN ──30s→ HALF_OPEN ──ok→ VALID
      ──429→ RATE_LIMITED (30min) ──→ VALID
      ──banned→ BANNED (terminal, manual unban only)
```

## Files

| Symbol | File |
|--------|------|
| `AccountPool`, `Acquire`, `Release`, `Touch`, `MarkError`, `MarkSuccess` | `backend/internal/core/account_pool.go` |
| `AccountHeap` (sorted by `-Score`) | same file, line 22 vicinity |
| `isAvailable` (filter logic — bans, rate-limit window, inflight cap, exclude set) | same file |
| `StartRecoveryLoop`, `recoverAccounts` | same file |
| `StartSnapshotLoop`, `snapshotOnce`, `triggerFlush` | same file |
| `Account` struct (status, inflight, score, refresh material) | `backend/internal/models/account.go` |
| `AccountStatus` constants | same file (VALID/RATE_LIMITED/SOFT_ERROR/CIRCUIT_OPEN/HALF_OPEN/BANNED) |
| `Account.ComputeScore` | same file |
| Settings hook for max-inflight | `core/settings_manager.go` (`MaxInflightPerAccount`) |

## Public API

```go
// Acquire: blocks until an account matching providerName is available.
// `exclude` is a per-attempt set of emails to skip (caller-managed retry deduping).
acc, err := pool.Acquire(ctx, providerName, exclude)
defer pool.Release(acc)

// Mark outcomes — call exactly one per request, after Release
pool.MarkSuccess(acc)
pool.MarkError(email, errorType, errMsg)
// errorType ∈ {"rate_limit", "auth", "banned", "transient"}

// Pool admin
pool.AddAccount(acc), pool.RemoveAccount(email), pool.SoftDeleteAccount(email)
pool.Touch(acc)  // recompute score after manual mutation
pool.ListAccounts(), pool.GetStatus(), pool.CountByProvider(name), pool.CountByStatus(provider, status)

// Lifecycle (called by server.Run; don't re-call)
pool.Load([]*Account)
pool.StartRecoveryLoop() / StopRecoveryLoop()
pool.StartSnapshotLoop(interval, store) / StopSnapshotLoop()
```

## Invariants — DO NOT BREAK

1. **Acquire → Release is balanced** — `Inflight` decrements only via Release. Skip Release on early return and the account is stuck at +1 inflight forever
2. **Never hold `p.mu` across HTTP calls** — pool mutex is for in-memory state only; calling provider HTTP under the lock = thread death
3. **`MarkError("transient")` increments `ConsecutiveFailures`** — only call on confirmed-bad-account signals. Network blips and upstream 500s are NOT account-bad; do not MarkError on those
4. **`MarkError("banned")` is terminal** — only set when upstream explicitly bans the account (e.g. Qwen `account_disabled` response). Do not infer ban from 401s
5. **`Score` is recomputed on every state change** — `acc.Score = acc.ComputeScore()` after mutating fields, otherwise heap ordering is stale
6. **Critical status transitions trigger immediate flush** — `MarkError` already does this for Banned/CircuitOpen. New terminal states must do the same or you lose state on crash
7. **`exclude` set is per-Acquire call** — not persisted. Use it for in-request retry deduping; for permanent skips, MarkError the account

## Common edits

- **Add a new error classification**: add case to `MarkError` switch. Decide: bump `ConsecutiveFailures`? Set status? Set `RateLimitedUntil`? Critical flush?
- **Tune circuit-open threshold**: search `ConsecutiveFailures >= 5` in `MarkError`. The `30 * time.Second` next to it is the half-open window
- **Change scoring**: edit `Account.ComputeScore` in `models/account.go`. Heap will rerank on next `Touch`/`MarkSuccess`/`MarkError`
- **Per-provider max-inflight**: see `getMaxInflight(acc)` — currently global via settings. To customize per-provider, branch on `acc.Provider` here
- **Force-recover an account**: `acc.Status = VALID; pool.Touch(acc)` (rebuild heap automatically)

## Acquire flow (annotated)

```
Acquire(ctx, providerName, exclude):
  for {
    p.mu.Lock()
    candidate := top-of-heap matching providerName
    if isAvailable(candidate, now, exclude):
      candidate.Inflight++
      candidate.Score = ComputeScore()
      heap.Fix(...)
      p.mu.Unlock()
      return candidate
    p.mu.Unlock()
    select { case <-ctx.Done(): return err; case <-time.After(backoff): }
  }
```

`isAvailable` rejects:
- `Status ∈ {BANNED, RATE_LIMITED (within window), CIRCUIT_OPEN}`
- `Inflight >= maxInflight`
- `email ∈ exclude`
- `DeletedAt != nil`

## Snapshot loop (persistence)

Every `interval` (config), `snapshotOnce()` writes the full pool to `data/accounts.json` via `AccountStore`. **Critical transitions short-circuit this and flush immediately** so a crash mid-cycle doesn't revert a Banned account to Valid.

Don't bypass — if you change account state directly (e.g. dashboard endpoint), call `pool.Touch(acc)` or `pool.MarkError/Success` so the flush trigger fires correctly.

## Gotchas

- **Heap order is `Less = Score > Score`** — counter-intuitive: higher score = "smaller" in heap terms = popped first. Don't flip it
- **`Acquire` will block forever if no matching account** — pass a `ctx` with timeout; the function respects cancellation
- **`Inflight` can desync** if a goroutine panics between Acquire and Release. Audit for any path that returns before `defer Release`
- **Provider auth refresh** (`AuthProvider.EnsureFresh`) is called by some providers before dispatch — make sure your provider doesn't double-refresh under contention. The account itself isn't mutex'd; rely on pool inflight as the serialization point
- **`MarkSuccess` resets `ConsecutiveFailures` to 0 and `LastError` to ""** — so one good request rehabilitates an account out of SOFT_ERROR
- **`StatusHalfOpen`** is set by `recoverAccounts` (background goroutine), not by Acquire. After half-open, next success → VALID, next failure → CIRCUIT_OPEN again

## Cross-skill

- [provider-add](provider-add.md) — providers consume the pool; don't fork it per-provider
- [settings](settings.md) — `max_inflight_per_account` is read here
- [api-endpoint](api-endpoint.md) — handlers don't touch the pool; providers do
