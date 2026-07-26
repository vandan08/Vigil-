package metrics

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Fatalf("Content-Type = %q, want text exposition format 0.0.4", ct)
	}
	return rec.Body.String()
}

// value returns the sample value of a series line ("name{labels} value").
func value(t *testing.T, body, series string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, series+" ") {
			return strings.TrimPrefix(line, series+" ")
		}
	}
	t.Fatalf("series %q not found in scrape:\n%s", series, body)
	return ""
}

func TestIngestSeriesPreSeededAtZero(t *testing.T) {
	body := scrape(t, New())
	for _, res := range []string{"attached", "created", "deduped", "resolved"} {
		if got := value(t, body, `vigil_alerts_ingested_total{result="`+res+`"}`); got != "0" {
			t.Fatalf("fresh %s counter = %s, want 0", res, got)
		}
	}
}

func TestAddIngestedAccumulates(t *testing.T) {
	m := New()
	m.AddIngested("created", 2)
	m.AddIngested("created", 1)
	m.AddIngested("deduped", 5)

	body := scrape(t, m)
	if got := value(t, body, `vigil_alerts_ingested_total{result="created"}`); got != "3" {
		t.Fatalf("created = %s, want 3", got)
	}
	if got := value(t, body, `vigil_alerts_ingested_total{result="deduped"}`); got != "5" {
		t.Fatalf("deduped = %s, want 5", got)
	}
	if got := value(t, body, `vigil_alerts_ingested_total{result="attached"}`); got != "0" {
		t.Fatalf("attached = %s, want 0", got)
	}
}

func TestWebhookHistogramBucketsAreCumulative(t *testing.T) {
	m := New()
	m.ObserveWebhook(3 * time.Millisecond)  // le=0.005
	m.ObserveWebhook(70 * time.Millisecond) // le=0.1
	m.ObserveWebhook(3 * time.Second)       // above the last bucket: +Inf only

	body := scrape(t, m)
	for series, want := range map[string]string{
		`vigil_webhook_duration_seconds_bucket{le="0.005"}`: "1",
		`vigil_webhook_duration_seconds_bucket{le="0.05"}`:  "1",
		`vigil_webhook_duration_seconds_bucket{le="0.1"}`:   "2",
		`vigil_webhook_duration_seconds_bucket{le="0.5"}`:   "2", // the SLO boundary
		`vigil_webhook_duration_seconds_bucket{le="2.5"}`:   "2",
		`vigil_webhook_duration_seconds_bucket{le="+Inf"}`:  "3",
		"vigil_webhook_duration_seconds_count":              "3",
	} {
		if got := value(t, body, series); got != want {
			t.Fatalf("%s = %s, want %s", series, got, want)
		}
	}

	sum, err := strconv.ParseFloat(value(t, body, "vigil_webhook_duration_seconds_sum"), 64)
	if err != nil {
		t.Fatalf("parsing sum: %v", err)
	}
	if math.Abs(sum-3.073) > 1e-9 {
		t.Fatalf("sum = %v, want ~3.073", sum)
	}
}

func TestCallbackFamiliesAppearOnlyWhenWired(t *testing.T) {
	m := New()
	body := scrape(t, m)
	if strings.Contains(body, "vigil_incidents_open") {
		t.Fatal("vigil_incidents_open rendered without a callback")
	}
	if strings.Contains(body, "vigil_notifications_dropped_total") {
		t.Fatal("vigil_notifications_dropped_total rendered without a callback")
	}

	m.OpenIncidents = func() int { return 4 }
	m.DroppedNotifications = func() int { return 7 }
	body = scrape(t, m)
	if got := value(t, body, "vigil_incidents_open"); got != "4" {
		t.Fatalf("vigil_incidents_open = %s, want 4", got)
	}
	if got := value(t, body, "vigil_notifications_dropped_total"); got != "7" {
		t.Fatalf("vigil_notifications_dropped_total = %s, want 7", got)
	}
}

func TestInstrumentWebhookObservesEveryRequest(t *testing.T) {
	m := New()
	served := 0
	h := m.InstrumentWebhook(func(w http.ResponseWriter, r *http.Request) { served++ })

	for i := 0; i < 3; i++ {
		h(httptest.NewRecorder(), httptest.NewRequest("POST", "/webhooks/alertmanager", nil))
	}
	if served != 3 {
		t.Fatalf("wrapped handler served %d requests, want 3", served)
	}
	if got := value(t, scrape(t, m), "vigil_webhook_duration_seconds_count"); got != "3" {
		t.Fatalf("histogram count = %s, want 3", got)
	}
}

func TestConcurrentMutation(t *testing.T) {
	m := New()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.AddIngested("created", 1)
				m.ObserveWebhook(time.Millisecond)
			}
		}()
	}
	wg.Wait()

	body := scrape(t, m)
	if got := value(t, body, `vigil_alerts_ingested_total{result="created"}`); got != "800" {
		t.Fatalf("created = %s, want 800", got)
	}
	if got := value(t, body, "vigil_webhook_duration_seconds_count"); got != "800" {
		t.Fatalf("histogram count = %s, want 800", got)
	}
}
