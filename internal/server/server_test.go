package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vandan08/vigil/internal/alert"
	"github.com/vandan08/vigil/internal/api"
	"github.com/vandan08/vigil/internal/incident"
	"github.com/vandan08/vigil/internal/ingest"
	"github.com/vandan08/vigil/internal/metrics"
	"github.com/vandan08/vigil/internal/notify"
)

// TestRoutingTable exercises the wiring, not handler behavior (that lives in
// the ingest and api tests): every route in the API table must be reachable
// through the mux. Handlers that are tested but never registered ship a dead
// server — this test is what catches that.
func TestRoutingTable(t *testing.T) {
	store := incident.NewMemoryStore()
	in := ingest.NewHandler(slog.New(slog.DiscardHandler), alert.NewDeduper(5*time.Minute), store)
	m := metrics.New()
	in.Metrics = m
	ap := api.NewHandler(slog.New(slog.DiscardHandler), store, notify.NewBus())
	srv := httptest.NewServer(NewMux(in, ap, m))
	defer srv.Close()

	payload := `{"status":"firing","alerts":[{"status":"firing",
	  "labels":{"alertname":"HighErrorRate","service":"checkout","severity":"critical"},
	  "annotations":{"summary":"5xx rate above 5%"},
	  "startsAt":"2026-07-17T12:00:00Z"}]}`
	resp, err := http.Post(srv.URL+"/webhooks/alertmanager", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /webhooks/alertmanager: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /webhooks/alertmanager status = %d, want 202", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/incidents")
	if err != nil {
		t.Fatalf("GET /api/incidents: %v", err)
	}
	var incidents []incident.Incident
	if err := json.NewDecoder(resp.Body).Decode(&incidents); err != nil {
		t.Fatalf("decoding incidents: %v", err)
	}
	resp.Body.Close()
	if len(incidents) != 1 || incidents[0].State != incident.StateTriggered {
		t.Fatalf("incidents = %+v, want exactly 1 triggered incident", incidents)
	}
	id := incidents[0].ID

	// The three action routes, driven through the full lifecycle.
	for _, action := range []string{"ack", "mitigate", "resolve"} {
		resp, err = http.Post(srv.URL+"/api/incidents/"+id+"/"+action, "", nil)
		if err != nil {
			t.Fatalf("POST %s: %v", action, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %s status = %d, want 200", action, resp.StatusCode)
		}
	}

	// SSE stream: headers prove the route; stream mechanics live in api tests.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("GET /api/events Content-Type = %q, want text/event-stream", ct)
	}
	resp.Body.Close()

	// The embedded console and its assets.
	for path, wantType := range map[string]string{
		"/":                 "text/html",
		"/static/style.css": "text/css",
		"/static/app.js":    "text/javascript",
	} {
		resp, err = http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, wantType) {
			t.Fatalf("GET %s Content-Type = %q, want %s", path, ct, wantType)
		}
	}

	resp, err = http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want 200", resp.StatusCode)
	}

	// Self-metrics: the webhook POST at the top of this test must be visible
	// in the scrape — one created alert, one observed request duration.
	resp, err = http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	scrape, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Fatalf("GET /metrics Content-Type = %q, want text exposition format 0.0.4", ct)
	}
	for _, series := range []string{
		`vigil_alerts_ingested_total{result="created"} 1`,
		"vigil_webhook_duration_seconds_count 1",
	} {
		if !strings.Contains(string(scrape), series) {
			t.Fatalf("GET /metrics missing %q in:\n%s", series, scrape)
		}
	}
}
