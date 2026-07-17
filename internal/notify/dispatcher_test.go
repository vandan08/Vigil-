package notify

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

// blockingNotifier holds every Send until released, and records deliveries.
type blockingNotifier struct {
	release chan struct{}

	mu   sync.Mutex
	sent []Event
}

func (b *blockingNotifier) Send(_ context.Context, ev Event) error {
	<-b.release
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, ev)
	return nil
}

func (b *blockingNotifier) delivered() []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Event(nil), b.sent...)
}

func TestDispatcherDeliversInOrderAndDrainsOnClose(t *testing.T) {
	n := &blockingNotifier{release: make(chan struct{})}
	close(n.release) // never block in this test
	d := NewDispatcher(slog.New(slog.DiscardHandler), n, 8)

	d.Enqueue(Event{Kind: KindOpened})
	d.Enqueue(Event{Kind: KindResolved})
	d.Close() // must block until both are delivered

	got := n.delivered()
	if len(got) != 2 || got[0].Kind != KindOpened || got[1].Kind != KindResolved {
		t.Fatalf("delivered = %+v, want opened then resolved", got)
	}
	if d.Dropped() != 0 {
		t.Fatalf("dropped = %d, want 0", d.Dropped())
	}
}

func TestDispatcherDropsInsteadOfBlockingWhenFull(t *testing.T) {
	n := &blockingNotifier{release: make(chan struct{})}
	d := NewDispatcher(slog.New(slog.DiscardHandler), n, 1)

	// First event occupies the worker (blocked in Send), second fills the
	// queue, third must be dropped — and crucially, Enqueue must return
	// instead of blocking (this test hangs if it doesn't).
	d.Enqueue(Event{Kind: KindOpened})
	d.Enqueue(Event{Kind: KindOpened})
	d.Enqueue(Event{Kind: KindOpened})

	if d.Dropped() < 1 {
		t.Fatalf("dropped = %d, want >= 1", d.Dropped())
	}

	close(n.release)
	d.Close()

	if got := len(n.delivered()); got+d.Dropped() != 3 {
		t.Fatalf("delivered %d + dropped %d, want 3 total", got, d.Dropped())
	}
}
