# Skills index

Pre-digested cheat sheets — read **one** of these instead of grepping 20 files.
Each skill follows the same template: When to use → Mental model → Files → API → Invariants → Common edits → Gotchas.

## Workflow

1. Before coding, scan this index — find the skill that matches your task
2. Read that **one** skill file
3. Cross-skill links inside each file point to dependencies (e.g. provider-add → request-log)
4. Line numbers go stale; **symbol names** (`Acquire`, `recordRequestLog`) are stable — prefer those

## Skills

| Skill | When to use |
|-------|-------------|
| [api-endpoint](api-endpoint.md) | Adding a new HTTP route; wiring middleware; recording request logs |
| [provider-add](provider-add.md) | Adding a new upstream (Qwen-like, OpenAI-like, Anthropic-like, ephemeral session); model aliases |
| [account-pool](account-pool.md) | Touching pool acquire/release; ban/recovery logic; inflight tracking; snapshot loop |
| [settings](settings.md) | Adding a runtime setting; migration on Load; dashboard wiring |
| [rtk-caveman](rtk-caveman.md) | Request compression (RTK tool-output filters + Caveman prompt injection); 9router parity; saver dispatch |

## When NOT in this index

If the task isn't covered: grep, read the relevant file, then **add a skill file** under `docs/skills/` so the next agent doesn't repeat your search.
