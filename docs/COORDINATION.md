# Agent coordination

Multiple agents work in this repo (sometimes in the same working tree). Claim a lane here
before starting, keep lanes disjoint, and follow the ground rules.

## Active lanes

| Who | Lane | Files |
|---|---|---|
| Agent A (observed: routing tests in progress) | HTTP server layer & routing tests | `internal/server/**` |
| Agent B (Claude Code, this session) | Outbound notifications: Slack webhook notifier + bounded async dispatcher; store snapshot-safety; ingest wiring | `internal/notify/**`, `internal/incident/store.go`, `internal/ingest/**`, `cmd/vigil/main.go` |

## Ground rules

1. **Stage by explicit path** (`git add <file>...`), never `git add .` — the tree may contain
   the other agent's uncommitted work. Never commit a file you didn't author or review.
2. **Every commit builds and passes `go test ./...`** scoped at least to the packages you
   touched. Keep commits ≤ 3–4 files with meaningful messages.
3. **`git pull --rebase` before every push**; re-run tests after rebasing.
4. Shared files (`cmd/vigil/main.go`, `Development.md`, this file) may be touched by either
   lane — keep edits minimal and additive so rebases stay trivial.
5. When your lane is done, update the roadmap + dev log in `Development.md` and clear your
   row here.
