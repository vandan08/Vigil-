# ADR-003: In-memory store behind an interface, then Postgres

**Status:** Accepted — 2026-07-17

## Context

Incidents, timelines, and (later) tenants and audit logs need durable storage. Alert history
could grow large, which raises the "should this be a columnar store?" question early.

## Decision

1. **Phase 1a (now):** an in-memory implementation behind the `incident.Store` interface.
   All domain logic — state machine, dedup, timeline — is written against the interface and
   fully unit-tested without any database.
2. **Phase 1b:** **Postgres** with **sqlc** (typed SQL, no ORM), migrations via golang-migrate.
   Timeline events are an append-only table; incidents are row-per-incident with optimistic
   state transitions enforced in SQL as well as in the domain layer.

## Alternatives considered

- **ClickHouse for alert history** — genuinely the right tool at millions of alerts/day; at
  Vigil's target scale (a 5–50 dev team) it is an operational tax with no user-visible
  benefit. Rejected *for now*; the ingest path keeps raw alerts append-only so a later
  backfill is possible.
- **SQLite** — excellent for the single-binary demo story, but the Phase 4 multi-tenant SaaS
  target and concurrent writers make Postgres the safer end-state; running two engines is
  worse than running one.
- **ORM (GORM/ent)** — sqlc chosen instead: the SQL stays visible and reviewable, which
  matches how the schema will be defended in design discussions.

## Consequences

- Tests run in milliseconds today; the Postgres implementation lands with a
  testcontainers-based integration suite against the same interface contract.
- The in-memory store is not thrown away — it remains the test double forever.
- Risk accepted: interface designed before the second implementation exists may need
  adjustment when Postgres lands (mitigated by keeping the interface minimal).
