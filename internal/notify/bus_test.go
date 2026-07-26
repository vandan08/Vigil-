package notify

import (
	"testing"
)

func TestBusFansOutToAllSubscribers(t *testing.T) {
	b := NewBus()
	ch1, cancel1 := b.Subscribe()
	ch2, cancel2 := b.Subscribe()
	defer cancel1()
	defer cancel2()

	b.Publish(Event{Kind: KindOpened, Incident: testIncident()})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Kind != KindOpened {
				t.Fatalf("subscriber %d got kind %s, want opened", i+1, ev.Kind)
			}
		default:
			t.Fatalf("subscriber %d received nothing", i+1)
		}
	}
}

func TestBusCancelStopsDelivery(t *testing.T) {
	b := NewBus()
	ch, cancel := b.Subscribe()
	cancel()
	cancel() // idempotent: second cancel must not panic on a closed channel

	b.Publish(Event{Kind: KindOpened, Incident: testIncident()})
	if _, ok := <-ch; ok {
		t.Fatal("cancelled subscriber must see a closed channel, not an event")
	}
}

func TestBusDropsSlowSubscriberInsteadOfBlocking(t *testing.T) {
	b := NewBus()
	slow, cancelSlow := b.Subscribe()
	defer cancelSlow()

	// Fill the slow subscriber's buffer and push one past it. Publish must
	// return (never block) and close the fallen-behind channel so the
	// subscriber knows to resync.
	for i := 0; i <= subscriberBuffer; i++ {
		b.Publish(Event{Kind: KindOpened, Incident: testIncident()})
	}

	delivered := 0
	for range subscriberBuffer {
		if _, ok := <-slow; !ok {
			t.Fatalf("channel closed after %d events, want the full buffer of %d first", delivered, subscriberBuffer)
		}
		delivered++
	}
	if _, ok := <-slow; ok {
		t.Fatal("overflowed subscriber must be closed after its buffered events drain")
	}

	// The bus must keep serving healthy subscribers afterwards.
	fresh, cancelFresh := b.Subscribe()
	defer cancelFresh()
	b.Publish(Event{Kind: KindResolved, Incident: testIncident()})
	select {
	case ev := <-fresh:
		if ev.Kind != KindResolved {
			t.Fatalf("fresh subscriber got kind %s, want resolved", ev.Kind)
		}
	default:
		t.Fatal("fresh subscriber received nothing after an overflow drop")
	}
}
