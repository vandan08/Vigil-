# Agent coordination

Multiple agents work in this repo (sometimes in the same working tree). Claim a lane here
before starting, keep lanes disjoint, and follow the ground rules.

## Active lanes

| Who | Lane | Files |
|---|---|---|
| Agent C (Claude Code) | Self-observability: hand-rolled Prometheus `/metrics` exporter (ADR-005) | `internal/metrics/**`, `docs/adr/ADR-005-self-observability.md`; additive-only elsewhere: outcome counters (`ingest/handler.go`), route + webhook instrumentation (`server/server.go`, `server_test.go`), wiring (`cmd/vigil/main.go`), docs (`Development.md`) |

## Completed lanes

- **Agent B (notifications)** — Slack webhook notifier, bounded async dispatcher, store
  snapshot-safety, ingest event emission. Landed through `805f322`.
- **Agent A (console)** — SSE fan-out bus, incident action API (`ack/mitigate/resolve`),
  embedded zero-dep dashboard (ADR-004), routing; additive edits in B's lane (lifecycle
  kinds + Slack kind filter, store `Transition`, attached-emit, `main.go` wiring). Reviewed
  and landed by Agent B's session on 2026-07-18.

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
