package incident

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Store is the persistence seam (ADR-003): domain logic is written against
// this interface. MemoryStore serves Phase 1 and remains the test double
// once the Postgres implementation lands.
//
// Every method returns snapshot copies, never live pointers: callers hand
// incidents to loggers, JSON encoders, and the notification dispatcher's
// goroutine, none of which may observe later mutations.
type Store interface {
	// UpsertFromAlert attaches a firing alert to its open incident, or opens
	// a new incident if none exists. The bool reports whether one was created.
	UpsertFromAlert(fingerprint, title, severity string, now time.Time) (*Incident, bool)
	// ResolveByFingerprint auto-resolves the open incident for a fingerprint,
	// if any — the path taken when a source reports the alert as cleared.
	ResolveByFingerprint(fingerprint string, now time.Time) (*Incident, bool)
	// List returns all incidents, newest first.
	List() []*Incident
}

// snapshot returns a copy safe to share outside the store lock.
func (i *Incident) snapshot() *Incident {
	c := *i
	c.Fingerprints = append([]string(nil), i.Fingerprints...)
	c.Timeline = append([]Event(nil), i.Timeline...)
	return &c
}

// MemoryStore is the in-memory Store implementation.
type MemoryStore struct {
	mu     sync.Mutex
	seq    int
	byID   map[string]*Incident
	openFP map[string]string // fingerprint -> ID of the open incident for it
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byID: make(map[string]*Incident), openFP: make(map[string]string)}
}

func (s *MemoryStore) UpsertFromAlert(fingerprint, title, severity string, now time.Time) (*Incident, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.openFP[fingerprint]; ok {
		inc := s.byID[id]
		inc.Append(now, "alert_fired", title)
		return inc.snapshot(), false
	}

	s.seq++
	inc := &Incident{
		ID:           fmt.Sprintf("INC-%04d", s.seq),
		Title:        title,
		Severity:     severity,
		State:        StateTriggered,
		Fingerprints: []string{fingerprint},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	inc.Append(now, "created", title)
	s.byID[inc.ID] = inc
	s.openFP[fingerprint] = inc.ID
	return inc.snapshot(), true
}

func (s *MemoryStore) ResolveByFingerprint(fingerprint string, now time.Time) (*Incident, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.openFP[fingerprint]
	if !ok {
		return nil, false
	}
	inc := s.byID[id]
	if err := inc.TransitionTo(StateResolved, now); err != nil {
		return inc.snapshot(), false
	}
	delete(s.openFP, fingerprint)
	return inc.snapshot(), true
}

func (s *MemoryStore) List() []*Incident {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*Incident, 0, len(s.byID))
	for _, inc := range s.byID {
		out = append(out, inc.snapshot())
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.After(out[b].CreatedAt) })
	return out
}
