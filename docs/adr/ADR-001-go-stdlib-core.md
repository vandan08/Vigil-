# ADR-001: Go with a stdlib-only core

**Status:** Accepted — 2026-07-17

## Context

Vigil is an infrastructure product: it sits next to Prometheus, Alertmanager, and Kubernetes,
and must be trivial to self-host on cheap hardware. The author's other flagship project
(ChainSentry) is Java/Spring Boot, and prior tooling (AutoHawk) is Python.

## Decision

Write Vigil in **Go**, and keep the core free of third-party dependencies until a dependency
earns its place. HTTP uses `net/http` with Go 1.22+ method-pattern routing; JSON, hashing, and
concurrency come from the standard library.

## Alternatives considered

- **Java/Spring Boot** — already demonstrated in ChainSentry; JVM memory footprint works
  against the "runs on a €5 VPS" positioning.
- **Python/FastAPI** — fast to write, but weaker fit for a long-running concurrent ingest
  service, and packaging/deploy is heavier than a static binary.
- **Go with a framework (chi/echo/gin)** — fine choices, but at this size a router framework
  adds nothing `net/http` doesn't already do; dependencies can be added when a real need
  appears (e.g. sqlc for Postgres).

## Consequences

- Single static binary; `FROM scratch`-class container images; fast cold starts.
- Zero supply-chain surface in the core today — relevant for a product pitched at
  security-conscious self-hosters.
- Costs: no ORM/DI conveniences; some boilerplate (accepted); contributors must know Go.
- The portfolio becomes deliberately polyglot: Java for enterprise SaaS, Python for
  data/LLM pipelines, Go for infrastructure.
