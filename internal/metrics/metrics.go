// Package metrics is Vigil's self-observability surface: counters and a
// latency histogram exposed in Prometheus text format (0.0.4) at GET /metrics.
//
// Vigil pages people about other systems' failures, so it must publish the
// numbers it would page others for (Development.md §9) — starting with the
// webhook ingestion latency behind the "p99 < 500 ms" self-SLO. The
// exposition format is a stable line protocol that costs about a screen of
// code to render by hand, so the stdlib-only stance of ADR-001 holds; the
// trade-offs are recorded in ADR-005.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"
)

// webhookBuckets are the histogram upper bounds, in seconds. 0.5 is a bucket
// boundary on purpose: conformance to the 500 ms ingestion SLO is then read
// straight off two counters, with no interpolation error across the target.
var webhookBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5}

// ingestResults are the outcome labels the ingest pipeline reports. They are
// pre-seeded at zero so every series exists from the first scrape — dashboards
// and alert rules must not have to special-case absent series.
var ingestResults = []string{"attached", "created", "deduped", "resolved"}

// Metrics collects Vigil's own operational numbers and serves them as a
// Prometheus scrape target. Mutating methods are safe for concurrent use.
//
// State owned elsewhere (open incidents, dropped notifications) is polled at
// scrape time through the callback fields rather than double-booked here.
// Set callbacks before the server starts; a nil callback omits its family.
type Metrics struct {
	// OpenIncidents reports incidents currently in a non-resolved state.
	OpenIncidents func() int
	// DroppedNotifications reports events the dispatcher discarded because
	// its queue was full. Nil when no notifier is configured.
	DroppedNotifications func() int

	started time.Time

	mu        sync.Mutex
	ingested  map[string]int64
	whBuckets []int64 // observations per bucket, cumulated only at render
	whInf     int64   // observations above the last bucket
	whSum     float64
}

func New() *Metrics {
	m := &Metrics{
		started:   time.Now(),
		ingested:  make(map[string]int64, len(ingestResults)),
		whBuckets: make([]int64, len(webhookBuckets)),
	}
	for _, r := range ingestResults {
		m.ingested[r] = 0
	}
	return m
}

// AddIngested counts n alerts that left the ingest pipeline with the given
// outcome (created, attached, deduped, resolved).
func (m *Metrics) AddIngested(result string, n int) {
	if n == 0 {
		return
	}
	m.mu.Lock()
	m.ingested[result] += int64(n)
	m.mu.Unlock()
}

// ObserveWebhook records the wall time of one webhook request.
func (m *Metrics) ObserveWebhook(d time.Duration) {
	s := d.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.whSum += s
	for i, ub := range webhookBuckets {
		if s <= ub {
			m.whBuckets[i]++
			return
		}
	}
	m.whInf++
}

// InstrumentWebhook wraps a handler so every request it serves lands in the
// webhook duration histogram.
func (m *Metrics) InstrumentWebhook(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next(w, r)
		m.ObserveWebhook(time.Since(start))
	}
}

// ServeHTTP renders one scrape. Counter state is copied under the lock and
// rendered outside it, so a slow scrape reader never stalls ingestion.
func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	ingested := make(map[string]int64, len(m.ingested))
	for k, v := range m.ingested {
		ingested[k] = v
	}
	buckets := append([]int64(nil), m.whBuckets...)
	inf := m.whInf
	sum := m.whSum
	m.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	family(w, "vigil_alerts_ingested_total", "counter",
		"Alerts processed by the ingest pipeline, by outcome.")
	results := make([]string, 0, len(ingested))
	for res := range ingested {
		results = append(results, res)
	}
	sort.Strings(results)
	for _, res := range results {
		fmt.Fprintf(w, "vigil_alerts_ingested_total{result=%q} %d\n", res, ingested[res])
	}

	family(w, "vigil_webhook_duration_seconds", "histogram",
		"Wall time spent handling one alert webhook request.")
	var cum int64
	for i, ub := range webhookBuckets {
		cum += buckets[i]
		fmt.Fprintf(w, "vigil_webhook_duration_seconds_bucket{le=%q} %d\n", formatFloat(ub), cum)
	}
	cum += inf
	fmt.Fprintf(w, "vigil_webhook_duration_seconds_bucket{le=\"+Inf\"} %d\n", cum)
	fmt.Fprintf(w, "vigil_webhook_duration_seconds_sum %s\n", formatFloat(sum))
	fmt.Fprintf(w, "vigil_webhook_duration_seconds_count %d\n", cum)

	if m.OpenIncidents != nil {
		family(w, "vigil_incidents_open", "gauge",
			"Incidents currently in a non-resolved state.")
		fmt.Fprintf(w, "vigil_incidents_open %d\n", m.OpenIncidents())
	}
	if m.DroppedNotifications != nil {
		family(w, "vigil_notifications_dropped_total", "counter",
			"Lifecycle events discarded because the dispatcher queue was full.")
		fmt.Fprintf(w, "vigil_notifications_dropped_total %d\n", m.DroppedNotifications())
	}

	family(w, "go_goroutines", "gauge", "Number of goroutines that currently exist.")
	fmt.Fprintf(w, "go_goroutines %d\n", runtime.NumGoroutine())
	family(w, "process_start_time_seconds", "gauge",
		"Start time of the process since unix epoch in seconds.")
	fmt.Fprintf(w, "process_start_time_seconds %d\n", m.started.Unix())
}

func family(w io.Writer, name, kind, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
