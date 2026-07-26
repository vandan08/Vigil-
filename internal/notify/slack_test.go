package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vandan08/vigil/internal/incident"
)

func testIncident() incident.Incident {
	return incident.Incident{ID: "INC-0001", Title: "HighErrorRate: 5xx above 5%", Severity: "critical"}
}

func TestSlackWebhookSendsFormattedMessage(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decoding webhook body: %v", err)
		}
	}))
	defer srv.Close()

	if err := NewSlackWebhook(srv.URL).Send(context.Background(), Event{Kind: KindOpened, Incident: testIncident()}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	text := got["text"]
	for _, want := range []string{"INC-0001", "critical", "opened", "HighErrorRate"} {
		if !strings.Contains(text, want) {
			t.Errorf("message %q missing %q", text, want)
		}
	}
}

func TestSlackWebhookSkipsTimelineDetailKinds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP call expected for timeline-detail kinds")
	}))
	defer srv.Close()

	for _, kind := range []Kind{KindAttached, KindAcknowledged, KindMitigated} {
		if err := NewSlackWebhook(srv.URL).Send(context.Background(), Event{Kind: kind, Incident: testIncident()}); err != nil {
			t.Fatalf("Send(%s): %v, want silent skip", kind, err)
		}
	}
}

func TestSlackWebhookErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no_service", http.StatusNotFound)
	}))
	defer srv.Close()

	if err := NewSlackWebhook(srv.URL).Send(context.Background(), Event{Kind: KindResolved, Incident: testIncident()}); err == nil {
		t.Fatal("Send must surface non-2xx responses as errors")
	}
}
