// Package api serves Vigil's incident action endpoints and the live event
// stream that feeds the dashboard. Ingest adapts alerts in; this package is
// how humans (and their browsers) act on the result.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/vandan08/vigil/internal/incident"
	"github.com/vandan08/vigil/internal/notify"
)

// heartbeatInterval keeps idle SSE connections alive through proxies that
// time out silent streams.
const heartbeatInterval = 25 * time.Second

// Handler exposes incident actions and the SSE event stream.
type Handler struct {
	Log   *slog.Logger
	Store incident.Store
	Bus   *notify.Bus      // subscription side: where the SSE stream listens
	Now   func() time.Time // injected so tests control the clock

	// Notify publishes lifecycle events produced by human actions. Wire it
	// to the same composite the ingest handler uses so Slack and the SSE
	// bus both hear about dashboard-driven transitions.
	Notify func(notify.Event)
}

func NewHandler(log *slog.Logger, store incident.Store, bus *notify.Bus) *Handler {
	return &Handler{Log: log, Store: store, Bus: bus, Now: time.Now}
}

// Ack handles POST /api/incidents/{id}/ack.
func (h *Handler) Ack(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, incident.StateAcknowledged, notify.KindAcknowledged)
}

// Mitigate handles POST /api/incidents/{id}/mitigate.
func (h *Handler) Mitigate(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, incident.StateMitigated, notify.KindMitigated)
}

// Resolve handles POST /api/incidents/{id}/resolve.
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, incident.StateResolved, notify.KindResolved)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, next incident.State, kind notify.Kind) {
	id := r.PathValue("id")
	inc, err := h.Store.Transition(id, next, h.Now())
	switch {
	case errors.Is(err, incident.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case err != nil:
		// The state machine rejected it: the incident moved on under the
		// caller's feet (e.g. auto-resolved before the ack landed).
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	h.Log.Info("incident transitioned", "id", inc.ID, "to", string(next))
	if h.Notify != nil {
		h.Notify(notify.Event{Kind: kind, Incident: *inc})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inc)
}

// Events handles GET /api/events: a Server-Sent Events stream. Every client
// first receives a `snapshot` event with all incidents, then one `incident`
// event per lifecycle change. When a client falls too far behind, the bus
// closes its subscription and the stream ends — EventSource reconnects and
// resyncs from the next snapshot, so a dropped client self-heals.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // tell buffering proxies not to

	// Subscribe before snapshotting: an event landing in between appears in
	// both, which the idempotent, ID-keyed client rendering absorbs. The
	// reverse order would lose it entirely.
	ch, cancel := h.Bus.Subscribe()
	defer cancel()

	if err := writeSSE(w, "snapshot", h.Store.List()); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return // dropped for falling behind; client reconnects
			}
			payload := struct {
				Kind     notify.Kind       `json:"kind"`
				Incident incident.Incident `json:"incident"`
			}{ev.Kind, ev.Incident}
			if err := writeSSE(w, "incident", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSE encodes v as one SSE event. json.Marshal never emits newlines,
// so the data fits a single `data:` line.
func writeSSE(w http.ResponseWriter, event string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}
