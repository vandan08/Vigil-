// Package server wires Vigil's HTTP routing table.
package server

import (
	"net/http"

	"github.com/vandan08/vigil/internal/api"
	"github.com/vandan08/vigil/internal/ingest"
	"github.com/vandan08/vigil/internal/metrics"
	"github.com/vandan08/vigil/internal/web"
)

// NewMux builds the routing table. Handlers are registered here and nowhere
// else, so the API surface is readable in one place.
func NewMux(in *ingest.Handler, ap *api.Handler, m *metrics.Metrics) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("GET /metrics", m)

	mux.HandleFunc("POST /webhooks/alertmanager", m.InstrumentWebhook(in.Alertmanager))

	mux.HandleFunc("GET /api/incidents", in.ListIncidents)
	mux.HandleFunc("POST /api/incidents/{id}/ack", ap.Ack)
	mux.HandleFunc("POST /api/incidents/{id}/mitigate", ap.Mitigate)
	mux.HandleFunc("POST /api/incidents/{id}/resolve", ap.Resolve)
	mux.HandleFunc("GET /api/events", ap.Events)

	mux.HandleFunc("GET /{$}", web.Index)
	mux.Handle("GET /static/", web.Static())

	return mux
}
