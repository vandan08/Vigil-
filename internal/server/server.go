// Package server wires Vigil's HTTP routing table.
package server

import (
	"net/http"

	"github.com/vandan08/vigil/internal/ingest"
)

// NewMux builds the routing table. Handlers are registered here and nowhere
// else, so the API surface is readable in one place.
func NewMux(h *ingest.Handler) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("POST /webhooks/alertmanager", h.Alertmanager)
	mux.HandleFunc("GET /api/incidents", h.ListIncidents)

	return mux
}
