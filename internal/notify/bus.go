package notify

import "sync"

// subscriberBuffer bounds how far a subscriber may fall behind before it is
// dropped. SSE clients that lose their subscription reconnect and resync
// from a fresh snapshot, so the buffer only needs to absorb normal bursts.
const subscriberBuffer = 16

// Bus fans lifecycle events out to live in-process subscribers — today the
// SSE handlers feeding dashboard clients. Same backpressure stance as the
// Dispatcher: Publish never blocks. A subscriber whose buffer is full is
// closed and removed instead of stalling ingestion; the closed channel tells
// the subscriber it missed events and must resync.
type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBus() *Bus {
	return &Bus{subs: make(map[chan Event]struct{})}
}

// Subscribe registers a subscriber. The returned cancel is idempotent and
// safe to call after the Bus has already dropped the subscriber.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
	return ch, cancel
}

// Publish delivers ev to every subscriber that can accept it immediately and
// drops the ones that cannot.
func (b *Bus) Publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
			delete(b.subs, ch)
			close(ch)
		}
	}
}
