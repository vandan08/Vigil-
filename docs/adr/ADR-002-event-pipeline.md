# ADR-002: In-process event flow now, NATS JetStream later

**Status:** Accepted — 2026-07-17

## Context

The target architecture is event-driven: ingest → correlate → notify → (later) AI analysis.
It is tempting to start with a message broker on day one. But in Phase 1 there is exactly one
producer (ingest) and one consumer (the incident store), running in one process.

## Decision

**Phase 1 uses direct, synchronous calls between components, separated by interfaces.**
A broker is introduced only when the second *asynchronous* consumer appears (the Slack
notifier in Phase 2). The chosen broker at that point is **NATS JetStream**.

## Alternatives considered

- **Kafka** — the default answer in interviews, and the wrong one here: ZooKeeper/KRaft ops,
  JVM heap, and partition management are unjustified below thousands of events/sec.
- **Redis Streams** — viable and already proven in ChainSentry; rejected to avoid running
  Redis as a hard dependency and because NATS gives subjects + replay + persistence in a
  single ~15 MB binary, matching Vigil's self-host positioning.
- **Broker from day one** — infrastructure without a consumer is résumé-driven development;
  it slows every local dev loop and test run for zero current benefit.

## Consequences

- Phase 1 stays trivially testable (no containers needed for unit tests) and deploys as one
  process.
- The interface seams (`incident.Store`, future `Notifier`) mark exactly where JetStream
  slots in; the migration is additive, not a rewrite.
- Delivery semantics are documented now: at-least-once with **idempotent consumers**
  (fingerprint-keyed), so moving to a broker does not change correctness assumptions.
