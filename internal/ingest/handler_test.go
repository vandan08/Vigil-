package ingest

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vandan08/vigil/internal/alert"
	"github.com/vandan08/vigil/internal/incident"
	"github.com/vandan08/vigil/internal/notify"
)

const firingPayload = `{"status":"firing","alerts":[{"status":"firing",
  "labels":{"alertname":"HighErrorRate","service":"checkout","severity":"critical"},
  "annotations":{"summary":"5xx rate above 5%"},
  "startsAt":"2026-07-17T12:00:00Z"}]}`

const resolvedPayload = `{"status":"resolved","alerts":[{"status":"resolved",
  "labels":{"alertname":"HighErrorRate","service":"checkout","severity":"critical"},
  "annotations":{"summary":"5xx rate above 5%"},
  "startsAt":"2026-07-17T12:00:00Z"}]}`

func newTestHandler(now *time.Time) *Handler {
	h := NewHandler(slog.New(slog.DiscardHandler), alert.NewDeduper(5*time.Minute), incident.NewMemoryStore())
	h.Now = func() time.Time { return *now }
	return h
}

func post(t *testing.T, h *Handler, body string) map[string]int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/webhooks/alertmanager", strings.NewReader(body))
	h.Alertmanager(rec, req)
	if rec.Code != 202 {
		t.Fatalf("status = %d, want 202 (body: %s)", rec.Code, rec.Body.String())
	}
	var counts map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&counts); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return counts
}

func TestWebhookDrivesIncidentLifecycle(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	h := newTestHandler(&now)

	if c := post(t, h, firingPayload); c["created"] != 1 {
		t.Fatalf("first firing: want 1 created, got %v", c)
	}

	now = now.Add(time.Minute) // inside dedup window
	if c := post(t, h, firingPayload); c["deduped"] != 1 || c["created"] != 0 {
		t.Fatalf("re-fire inside window: want deduped, got %v", c)
	}

	now = now.Add(10 * time.Minute) // past the window, incident still open
	if c := post(t, h, firingPayload); c["attached"] != 1 || c["created"] != 0 {
		t.Fatalf("re-fire past window: want attached to open incident, got %v", c)
	}

	now = now.Add(time.Minute)
	if c := post(t, h, resolvedPayload); c["resolved"] != 1 {
		t.Fatalf("clearing alert: want 1 resolved, got %v", c)
	}

	incidents := h.Store.List()
	if len(incidents) != 1 {
		t.Fatalf("want exactly 1 incident, got %d", len(incidents))
	}
	if incidents[0].State != incident.StateResolved {
		t.Fatalf("incident state = %s, want resolved", incidents[0].State)
	}
}

func TestLifecycleEventsAreEmitted(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	h := newTestHandler(&now)

	var events []notify.Event
	h.Notify = func(ev notify.Event) { events = append(events, ev) }

	post(t, h, firingPayload)
	now = now.Add(time.Minute)
	post(t, h, firingPayload) // deduped: must not emit
	now = now.Add(10 * time.Minute)
	post(t, h, firingPayload) // past the window: attaches, must emit
	now = now.Add(time.Minute)
	post(t, h, resolvedPayload)

	want := []notify.Kind{notify.KindOpened, notify.KindAttached, notify.KindResolved}
	if len(events) != len(want) {
		t.Fatalf("emitted %d events, want %d (%v)", len(events), len(want), want)
	}
	for i, kind := range want {
		if events[i].Kind != kind {
			t.Fatalf("event %d kind = %s, want %s", i, events[i].Kind, kind)
		}
		if events[i].Incident.ID != events[0].Incident.ID {
			t.Fatalf("events must carry the same incident, got %q and %q",
				events[0].Incident.ID, events[i].Incident.ID)
		}
	}
}

func TestMalformedPayloadIsRejected(t *testing.T) {
	now := time.Now()
	h := newTestHandler(&now)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/webhooks/alertmanager", strings.NewReader("{not json"))
	h.Alertmanager(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
