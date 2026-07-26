package api

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vandan08/vigil/internal/incident"
	"github.com/vandan08/vigil/internal/notify"
)

var t0 = time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)

func newTestHandler() (*Handler, *incident.MemoryStore) {
	store := incident.NewMemoryStore()
	h := NewHandler(slog.New(slog.DiscardHandler), store, notify.NewBus())
	h.Now = func() time.Time { return t0.Add(time.Minute) }
	return h, store
}

// mux mirrors the real routing so PathValue("id") works in tests.
func mux(h *Handler) *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("POST /api/incidents/{id}/ack", h.Ack)
	m.HandleFunc("POST /api/incidents/{id}/mitigate", h.Mitigate)
	m.HandleFunc("POST /api/incidents/{id}/resolve", h.Resolve)
	m.HandleFunc("GET /api/events", h.Events)
	return m
}

func postTransition(t *testing.T, m *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("POST", path, nil))
	return rec
}

func TestTransitionEndpoints(t *testing.T) {
	h, store := newTestHandler()
	inc, _ := store.UpsertFromAlert("fp1", "High error rate", "critical", t0)
	m := mux(h)

	var emitted []notify.Event
	h.Notify = func(ev notify.Event) { emitted = append(emitted, ev) }

	if rec := postTransition(t, m, "/api/incidents/INC-9999/ack"); rec.Code != 404 {
		t.Fatalf("unknown id: status = %d, want 404", rec.Code)
	}
	if rec := postTransition(t, m, "/api/incidents/"+inc.ID+"/mitigate"); rec.Code != 409 {
		t.Fatalf("triggered -> mitigated: status = %d, want 409", rec.Code)
	}

	rec := postTransition(t, m, "/api/incidents/"+inc.ID+"/ack")
	if rec.Code != 200 {
		t.Fatalf("ack: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got incident.Incident
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding ack response: %v", err)
	}
	if got.State != incident.StateAcknowledged {
		t.Fatalf("ack response state = %s, want acknowledged", got.State)
	}

	if rec := postTransition(t, m, "/api/incidents/"+inc.ID+"/resolve"); rec.Code != 200 {
		t.Fatalf("resolve: status = %d, want 200", rec.Code)
	}

	// Failed transitions must not emit; the two successful ones must.
	want := []notify.Kind{notify.KindAcknowledged, notify.KindResolved}
	if len(emitted) != len(want) {
		t.Fatalf("emitted %d events, want %d", len(emitted), len(want))
	}
	for i, kind := range want {
		if emitted[i].Kind != kind {
			t.Fatalf("event %d kind = %s, want %s", i, emitted[i].Kind, kind)
		}
	}
}

// readSSEEvent scans one "event:"/"data:" pair off the stream, skipping
// blank separators and comment heartbeats.
func readSSEEvent(t *testing.T, scanner *bufio.Scanner) (event, data string) {
	t.Helper()
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			return event, strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatalf("stream ended before a full event arrived: %v", scanner.Err())
	return "", ""
}

func TestEventStreamSendsSnapshotThenLiveEvents(t *testing.T) {
	h, store := newTestHandler()
	store.UpsertFromAlert("fp1", "High error rate", "critical", t0)
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	event, data := readSSEEvent(t, scanner)
	if event != "snapshot" {
		t.Fatalf("first event = %q, want snapshot", event)
	}
	var snapshot []incident.Incident
	if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
		t.Fatalf("decoding snapshot: %v", err)
	}
	if len(snapshot) != 1 || snapshot[0].State != incident.StateTriggered {
		t.Fatalf("snapshot = %+v, want the one triggered incident", snapshot)
	}

	// A published lifecycle event must reach the connected client.
	inc, err := store.Transition(snapshot[0].ID, incident.StateAcknowledged, t0.Add(time.Minute))
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	h.Bus.Publish(notify.Event{Kind: notify.KindAcknowledged, Incident: *inc})

	event, data = readSSEEvent(t, scanner)
	if event != "incident" {
		t.Fatalf("second event = %q, want incident", event)
	}
	var live struct {
		Kind     notify.Kind       `json:"kind"`
		Incident incident.Incident `json:"incident"`
	}
	if err := json.Unmarshal([]byte(data), &live); err != nil {
		t.Fatalf("decoding live event: %v", err)
	}
	if live.Kind != notify.KindAcknowledged || live.Incident.State != incident.StateAcknowledged {
		t.Fatalf("live event = %+v, want acknowledged", live)
	}
}
