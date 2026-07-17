#!/bin/sh
# Push a demo alert into Alertmanager (v2 API). Alertmanager groups it and
# webhooks it into Vigil; the incident then shows up in Vigil's API.
# Usage: docker compose up --build   then   sh scripts/demo-alert.sh
set -e

AM=${AM:-http://localhost:9093}
VIGIL=${VIGIL:-http://localhost:8080}

curl -fsS -X POST "$AM/api/v2/alerts" -H 'Content-Type: application/json' -d '[{
  "labels": {"alertname": "HighErrorRate", "service": "checkout", "severity": "critical"},
  "annotations": {"summary": "5xx rate above 5% for 5m"},
  "startsAt": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"
}]'

echo "alert accepted by Alertmanager; waiting out group_wait..."
sleep 8

echo "incidents in Vigil:"
curl -fsS "$VIGIL/api/incidents"
echo
