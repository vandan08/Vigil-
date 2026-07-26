# ADR-005: Hand-rolled Prometheus exposition for self-metrics

**Status:** Accepted — 2026-07-18

## Context

Vigil's self-SLOs (Development.md §9) — webhook ingestion p99 < 500 ms, alert-to-visible
< 2 s, 99.5 % availability — are commitments nothing in the codebase measured. The tech
stack table has always said "Vigil monitors itself" via Prometheus; the compose stack and
the pending k6 alert-storm test both need a scrape target to read those numbers from.

The canonical route is `prometheus/client_golang`. It is excellent and battle-tested, but
it pulls in a dependency tree (protobuf, expfmt, internal model packages) against a core
that ADR-001 deliberately keeps stdlib-only, and its power — dynamic registries, arbitrary
label cardinality, exemplars — buys nothing at Vigil's current surface: one histogram, a
handful of counters and gauges, all with fixed names and one bounded label.

## Decision

1. **Hand-roll the text exposition format (0.0.4)** in `internal/metrics` — roughly a
   screen of rendering code behind `GET /metrics`. The format is a stable line protocol,
   unchanged for a decade; the risk of "maintaining a serializer" is close to zero.
2. **Fixed metric families, no registry.** The package exports one `Metrics` struct with
   purpose-named methods (`AddIngested`, `ObserveWebhook`). Adding a metric is a code
   change, which at this scale is a feature: the scrape surface is reviewable in one file.
3. **Gauges owned elsewhere are polled at scrape time** via callbacks (`OpenIncidents`,
   `DroppedNotifications`) instead of being double-booked — one source of truth, no
   counter drift. A nil callback omits the family (no Slack notifier ⇒ no drop metric).
4. **The webhook histogram makes 0.5 s an explicit bucket boundary**, so conformance to
   the 500 ms SLO reads directly off two counters with no interpolation error across the
   target. Counter series are pre-seeded at zero so dashboards never see absent series.

## Alternatives considered

- **`prometheus/client_golang`** — deferred, not rejected. The adoption triggers are
  concrete: a metric that needs unbounded label cardinality, OpenMetrics/exemplar support,
  or the moment `internal/metrics` stops fitting on a screen. The `/metrics` route and
  metric names are the contract; swapping the implementation underneath is invisible to
  Prometheus.
- **OpenTelemetry SDK** — the heaviest option, and its value (traces, cross-service
  context propagation) belongs to a distributed system. Vigil is one process.
- **stdlib `expvar`** — free, but exposes JSON that Prometheus cannot scrape without a
  bridge, which is just the hand-rolled exporter with extra steps.

## Consequences

- Cardinality discipline is manual: every label value must come from a small closed set
  (the ingest `result` label has four). A reviewer seeing a user-controlled string used
  as a label value should block the change.
- No default Go runtime collectors; the two that matter operationally today (goroutine
  count — the SSE-subscriber leak canary — and process start time) are emitted by hand.
- The k6 load test (Phase 1 roadmap) reports p99 from this endpoint, making the SLO a
  measured number instead of a promise.
