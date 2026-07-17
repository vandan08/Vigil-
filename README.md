# Vigil

[![CI](https://github.com/vandan08/Vigil-/actions/workflows/ci.yml/badge.svg)](https://github.com/vandan08/Vigil-/actions/workflows/ci.yml)

**Vigil is an incident response platform for engineering teams.** It ingests alerts from your
monitoring stack (Prometheus Alertmanager first), deduplicates and correlates them into
incidents, and drives the incident lifecycle — triage, acknowledge, mitigate, resolve — with a
full audit timeline. An AI root-cause agent (provider-pluggable, local-first via Ollama) is on
the roadmap, always with a human in the loop.

> Full product vision, architecture, roadmap, and engineering log: **[Development.md](Development.md)**

## Status

Early development — Phase 1 (alert ingestion → incident lifecycle) is in progress.
See the [roadmap](Development.md#7-roadmap) for what exists today vs. what is planned.

## What works today

- `POST /webhooks/alertmanager` — accepts Prometheus Alertmanager webhook payloads
- Alert normalization + stable fingerprinting (identity = sorted label set)
- Dedup window so a re-firing alert doesn't open a duplicate incident
- Incident state machine (`triggered → acknowledged → mitigated → resolved`) with an
  append-only timeline of everything that happened
- `GET /api/incidents` — list incidents with their timelines
- `GET /healthz` — liveness

## Quickstart

```sh
go run ./cmd/vigil            # starts on :8080 (PORT env to override)

# fire a fake alert at it
curl -s -X POST localhost:8080/webhooks/alertmanager \
  -H 'Content-Type: application/json' \
  -d '{"status":"firing","alerts":[{"status":"firing","labels":{"alertname":"HighErrorRate","service":"checkout","severity":"critical"},"annotations":{"summary":"5xx rate above 5%"},"startsAt":"2026-07-17T12:00:00Z"}]}'

curl -s localhost:8080/api/incidents | python -m json.tool
```

Or run the local demo stack (Vigil + Alertmanager wired together):

```sh
docker compose up --build
sh scripts/demo-alert.sh      # pushes an alert into Alertmanager -> webhooks into Vigil
```

## Architecture (current slice)

```
 Alertmanager ──webhook──▶ ingest ──normalize+fingerprint──▶ dedup ──▶ incident store
                                                                          │
                              REST API  ◀── incidents + timelines ────────┘
```

Design decisions are recorded as ADRs in [docs/adr](docs/adr).

## License

[MIT](LICENSE)
