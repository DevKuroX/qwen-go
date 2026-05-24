# Hard rules

These are non-negotiable. They override any other instruction.

## 0. Read the skills index before coding

`docs/skills/INDEX.md` is the agent's entry point. Pick the skill matching your task, read **that one file**, then code. Do not grep the codebase blind for things skills already document. If your task has no skill, grep + read, then **write a new skill** before moving on — leaving the corpus better than you found it is part of every task.

## 1. Rebuild + restart — never manual

**Never** kill processes, copy binaries by hand, or background a fresh process with `nohup`. The `qwen-go` binary is a CLI:

```bash
./qwen-go start      # background-spawn API+dashboard
./qwen-go stop       # graceful shutdown
./qwen-go restart    # graceful — handles "Text file busy" ordering correctly
```

After a code change:

```bash
# Backend only:
cd backend && go build -o ../qwen-go ./cmd/qwen-go/ && cd .. && ./qwen-go restart

# Frontend (or both):
bash scripts/build-frontend.sh \
  && cd backend && go build -o ../qwen-go ./cmd/qwen-go/ \
  && cd .. && ./qwen-go restart
```

`cp` over the live binary fails. `pkill -f qwen-go` then `setsid ./qwen-go start &` is wasted tokens. **Use `./qwen-go restart`.** Logs at `/tmp/qwen-go.log`.

## 2. Off-limits — auth surface

Do not edit. If broken, report — do not patch:

- `backend/internal/api/auth.go`
- `backend/internal/api/middleware.go`
- `frontend/app/api/auth/login/route.ts`
- `frontend/app/api/auth/logout/route.ts`
- `frontend/app/(auth)/login/page.tsx`
- Cookies: `qwenpi_key`, `qwenpi_apikey`
- `data/auth.json`, `data/api_keys.json`

**Split-auth invariant**: AdminKey authorises `/api/admin/*` only. Pool API keys authorise `/v1/*` only. Login/check self-heals stale `qwenpi_apikey` cookies. Don't widen either scope.

## 3. No commits without explicit permission

Do not run `git add`, `git commit`, `git push`, `git reset --hard`, `git checkout --`, `git restore`, `git clean -f`, `git branch -D`, or any other state-modifying git command unless the user explicitly says "commit", "push", or names the operation.

Reading git state (`git status`, `git diff`, `git log`) is fine.

## 4. Minimal fix scope

When fixing a bug:
- Change the broken line(s) only
- Do not refactor surrounding code
- Do not "improve" unrelated things you noticed
- Do not remove logic you don't understand

If a flagged bug is part of a known issue cluster, **stop after one fix** and ask. Fixing one tends to surface another, and cascading changes lose the user's plot.

## 5. Provider-pool internals are off-limits unless asked

The account pool, key manager, and provider registry have subtle invariants (inflight tracking, score reranking, ban detection). Don't touch them for tangential reasons. Read `docs/archive/MULTI_PROVIDER_DESIGN.md` if you need context.

## 6. Configuration scope for variants/aliases

`model_aliases` in `provider_configs.json` is **per-provider**. Currently only `opencode-zen` consumes the field. Don't propagate variant logic to other providers without confirming the upstream API actually accepts the equivalent params.

## 7. Tooling preferences

- **Editing files**: `Edit` / `Write` tool. Never `sed` / `awk` / `echo >>`.
- **Reading**: `Read` tool. Never `cat` / `head` / `tail`.
- **File search**: `Glob` / `Grep`. `find` and `grep` are acceptable for one-offs but prefer the tools.
- **Long-running builds**: `run_in_background: true`, then read the output when notified.

## 8. No emojis in source

Don't add emojis to Go files, TS files, JSON, or other code-adjacent files. UI strings shown to users are also no-emoji unless the user has already used them in the same component.

## 9. Frontend — Next.js 16, not the one you know

`frontend/AGENTS.md`: *"This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code."* Believe it. Check the docs first.

## 10. When in doubt

Ask. The user prefers a clarifying question over a confidently wrong implementation.
