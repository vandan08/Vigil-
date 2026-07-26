# ADR-004: Embedded zero-dependency dashboard over SSE

**Status:** Accepted — 2026-07-17

## Context

Phase 2 needs a human-facing view: incidents appearing live, with ack/mitigate/resolve
actions. The roadmap penciled in Next.js + TypeScript + Tailwind for "Phase 2b". But the
product's one-line pitch is *"incident.io you can run on a €5 VPS"* — a single static Go
binary. A Node build pipeline in the critical path contradicts that story before the
platform has earned the complexity, and it doubles the toolchain a contributor must install
to see the product work.

## Decision

1. **The dashboard ships inside the binary**: three static files (HTML/CSS/JS, no
   framework, no external fonts or CDN assets) embedded with `go:embed` and served at `/`.
   `go build` remains the entire toolchain.
2. **Live updates use Server-Sent Events** (`GET /api/events`), not WebSockets: the flow is
   strictly server→browser, SSE is plain HTTP (works through proxies, `EventSource`
   auto-reconnects for free), and the stdlib serves it with a `Flusher` — no dependency
   (ADR-001 holds).
3. **The stream protocol is snapshot-then-deltas**: on connect the client gets a `snapshot`
   event with all incidents, then one `incident` event per lifecycle change. Clients render
   idempotently from an ID-keyed map.
4. **Backpressure follows ADR-002's stance**: the in-process `notify.Bus` fans events out to
   subscribers and never blocks ingestion — a subscriber that falls behind is dropped
   (channel closed), which ends its stream; `EventSource` reconnects and resyncs from a
   fresh snapshot. Slow dashboards self-heal instead of stalling alert intake.
5. **Human actions are first-class API**: `POST /api/incidents/{id}/ack|mitigate|resolve`
   drive the same validated state machine as auto-resolve. The dashboard is just a client
   of this API — so will be the Slack app's buttons.

## Alternatives considered

- **Next.js now** — the market-default DX, but it adds a second toolchain, a build
  pipeline, and a deploy artifact that is no longer "one binary". Deferred, not rejected:
  when auth, multi-page flows, and the RCA-proposal UI arrive (Phase 3+), the embedded
  console can be superseded — the SSE + REST contract is the part designed to outlive it.
- **WebSockets** — bidirectional, which this flow does not need; costs a handshake
  protocol, ping/pong bookkeeping, and reconnect logic SSE gives for free.
- **Polling `GET /api/incidents`** — simplest, but "alert → incident visible < 2 s" is a
  self-SLO (§9), and 2-second polling from every open tab is the expensive way to fail it.
- **HTMX/Alpine from a CDN** — small, but a CDN reference breaks air-gapped self-hosting
  and adds a supply-chain surface to an incident tool. Vanilla JS at this scope is ~200
  lines.

## Consequences

- The console must stay deliberately small (list, timeline, actions). Feature pressure
  (filters, search, postmortem editing) is the signal to start the real frontend, not to
  grow this one.
- No client build step means no TypeScript; discipline comes from keeping the JS thin and
  the contract (snapshot/delta shapes) tested on the Go side.
- Every lifecycle event now flows through one `notify.Event` feed consumed by Slack and
  the bus — the RCA agent (Phase 3) subscribes to the same feed, which is the ADR-002
  migration seam: swap the in-process bus for NATS without touching producers.
