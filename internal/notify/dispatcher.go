package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Dispatcher delivers events asynchronously so ingestion never blocks on a
// slow or down notification channel. The queue is bounded and lossy: when it
// is full, new events are dropped and counted. Dropping a Slack ping is
// acceptable; stalling alert ingestion during an alert storm is not — the
// same backpressure stance ADR-002 commits the event pipeline to.
type Dispatcher struct {
	log      *slog.Logger
	notifier Notifier
	queue    chan Event
	timeout  time.Duration
	done     chan struct{}

	mu      sync.Mutex
	dropped int
}

// NewDispatcher starts the delivery worker. Callers own the lifecycle:
// call Close to drain and stop, and do not Enqueue after Close.
func NewDispatcher(log *slog.Logger, n Notifier, queueSize int) *Dispatcher {
	d := &Dispatcher{
		log:      log,
		notifier: n,
		queue:    make(chan Event, queueSize),
		timeout:  10 * time.Second,
		done:     make(chan struct{}),
	}
	go d.run()
	return d
}

// Enqueue never blocks; when the queue is full the event is dropped.
func (d *Dispatcher) Enqueue(ev Event) {
	select {
	case d.queue <- ev:
	default:
		d.mu.Lock()
		d.dropped++
		total := d.dropped
		d.mu.Unlock()
		d.log.Warn("notification queue full, event dropped",
			"incident", ev.Incident.ID, "kind", string(ev.Kind), "dropped_total", total)
	}
}

// Dropped reports how many events were discarded because the queue was full.
func (d *Dispatcher) Dropped() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dropped
}

// Close stops accepting events and blocks until queued events are delivered.
func (d *Dispatcher) Close() {
	close(d.queue)
	<-d.done
}

func (d *Dispatcher) run() {
	defer close(d.done)
	for ev := range d.queue {
		ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
		if err := d.notifier.Send(ctx, ev); err != nil {
			// Best-effort: log and move on. Retry with backoff belongs to a
			// later iteration, recorded in the roadmap.
			d.log.Warn("notification delivery failed",
				"incident", ev.Incident.ID, "kind", string(ev.Kind), "err", err)
		}
		cancel()
	}
}
