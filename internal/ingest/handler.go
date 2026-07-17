// Package ingest adapts external alert sources into Vigil's normalized
// model. One adapter per source; Alertmanager is the first.
package ingest

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/vandan08/vigil/internal/alert"
	"github.com/vandan08/vigil/internal/incident"
	"github.com/vandan08/vigil/internal/notify"
)

// amWebhook mirrors the fields Vigil needs from Alertmanager's webhook
// payload (version 4). Unknown fields are ignored on purpose.
type amWebhook struct {
	Status string `json:"status"`
	Alerts []struct {
		Status      string            `json:"status"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		StartsAt    time.Time         `json:"startsAt"`
	} `json:"alerts"`
}

// Handler turns incoming webhooks into incident store operations.
type Handler struct {
	Log   *slog.Logger
	Dedup *alert.Deduper
	Store incident.Store
	Now   func() time.Time // injected so tests control the clock

	// Notify, when non-nil, receives incident lifecycle events. It must not
	// block — hand it a Dispatcher's Enqueue, not a Notifier's Send.
	Notify func(notify.Event)
}

func NewHandler(log *slog.Logger, dedup *alert.Deduper, store incident.Store) *Handler {
	return &Handler{Log: log, Dedup: dedup, Store: store, Now: time.Now}
}

// Alertmanager handles POST /webhooks/alertmanager.
func (h *Handler) Alertmanager(w http.ResponseWriter, r *http.Request) {
	var payload amWebhook
	body := http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	now := h.Now()
	counts := map[string]int{"created": 0, "attached": 0, "resolved": 0, "deduped": 0}

	for _, a := range payload.Alerts {
		fp := alert.Fingerprint(a.Labels)

		name := a.Labels["alertname"]
		if name == "" {
			name = "unnamed alert"
		}
		title := name
		if summary := a.Annotations["summary"]; summary != "" {
			title = name + ": " + summary
		}

		if a.Status == alert.StatusResolved {
			if inc, ok := h.Store.ResolveByFingerprint(fp, now); ok {
				counts["resolved"]++
				h.Log.Info("incident auto-resolved", "id", inc.ID, "alert", name)
				h.emit(notify.KindResolved, inc)
			}
			continue
		}

		// Firing: drop re-fires inside the dedup window; past the window a
		// still-firing alert lands on the open incident's timeline.
		if h.Dedup.Seen(fp, now) {
			counts["deduped"]++
			continue
		}
		severity := a.Labels["severity"]
		if severity == "" {
			severity = "warning"
		}
		if inc, created := h.Store.UpsertFromAlert(fp, title, severity, now); created {
			counts["created"]++
			h.Log.Info("incident opened", "id", inc.ID, "alert", name, "severity", severity)
			h.emit(notify.KindOpened, inc)
		} else {
			counts["attached"]++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(counts)
}

// emit forwards a lifecycle event when notifications are configured. The
// incident is already a snapshot (the store never returns live pointers),
// so the event is safe to read from the dispatcher's goroutine.
func (h *Handler) emit(kind notify.Kind, inc *incident.Incident) {
	if h.Notify == nil {
		return
	}
	h.Notify(notify.Event{Kind: kind, Incident: *inc})
}

// ListIncidents handles GET /api/incidents.
func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.Store.List())
}
