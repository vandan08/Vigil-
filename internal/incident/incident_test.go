package incident

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)

func TestLifecycleHappyPath(t *testing.T) {
	inc := &Incident{State: StateTriggered}
	for _, next := range []State{StateAcknowledged, StateMitigated, StateResolved} {
		if err := inc.TransitionTo(next, t0); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	if len(inc.Timeline) != 3 {
		t.Fatalf("timeline entries = %d, want 3 (one per transition)", len(inc.Timeline))
	}
}

func TestInvalidTransitionsAreRejected(t *testing.T) {
	cases := []struct{ from, to State }{
		{StateTriggered, StateMitigated},    // can't mitigate before ack
		{StateResolved, StateAcknowledged},  // resolved is terminal
		{StateMitigated, StateAcknowledged}, // no going backwards
		{StateTriggered, StateTriggered},    // no self-loops
	}
	for _, c := range cases {
		inc := &Incident{State: c.from}
		if err := inc.TransitionTo(c.to, t0); err == nil {
			t.Errorf("%s -> %s should be rejected", c.from, c.to)
		}
	}
}

func TestStoreUpsertAttachResolveReopen(t *testing.T) {
	s := NewMemoryStore()

	a, created := s.UpsertFromAlert("fp1", "High error rate", "critical", t0)
	if !created {
		t.Fatal("first alert must create an incident")
	}

	b, created := s.UpsertFromAlert("fp1", "High error rate", "critical", t0.Add(time.Minute))
	if created || b.ID != a.ID {
		t.Fatal("re-fire while open must attach to the same incident, not create")
	}

	if _, ok := s.ResolveByFingerprint("fp1", t0.Add(2*time.Minute)); !ok {
		t.Fatal("open incident must resolve when its alert clears")
	}
	if _, ok := s.ResolveByFingerprint("fp1", t0.Add(2*time.Minute)); ok {
		t.Fatal("resolving twice must be a no-op")
	}

	c, created := s.UpsertFromAlert("fp1", "High error rate", "critical", t0.Add(3*time.Minute))
	if !created || c.ID == a.ID {
		t.Fatal("firing again after resolution must open a fresh incident")
	}

	if got := len(s.List()); got != 2 {
		t.Fatalf("List() = %d incidents, want 2", got)
	}
	if first := s.List()[0]; first.ID != c.ID {
		t.Fatalf("List() must be newest-first, got %s first", first.ID)
	}
}

func TestStoreReturnsSnapshotsNotLivePointers(t *testing.T) {
	s := NewMemoryStore()

	a, _ := s.UpsertFromAlert("fp1", "High error rate", "critical", t0)
	if len(a.Timeline) != 1 {
		t.Fatalf("new incident timeline = %d entries, want 1", len(a.Timeline))
	}

	// A later attach must not reach into the snapshot handed out earlier —
	// snapshots go to loggers and the notification dispatcher's goroutine.
	s.UpsertFromAlert("fp1", "High error rate", "critical", t0.Add(time.Minute))
	if len(a.Timeline) != 1 {
		t.Fatal("earlier snapshot grew when the store mutated the incident")
	}

	// Nor may a caller mutate store state through a returned incident.
	a.Title = "tampered"
	if s.List()[0].Title != "High error rate" {
		t.Fatal("mutating a returned snapshot leaked into the store")
	}
}
