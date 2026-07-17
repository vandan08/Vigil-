// Package server wires Vigil's HTTP routing table.
package server

import "net/http"

// NewMux builds the routing table. Handlers are registered here and nowhere
// else, so the API surface is readable in one place.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	return mux
}
