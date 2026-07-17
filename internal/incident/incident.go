// Package incident holds Vigil's core domain: the incident lifecycle state
// machine and its append-only timeline.
package incident

import (
	"fmt"
	"time"
)

// State is a stage in the incident lifecycle.
type State string

const (
	StateTriggered    State = "triggered"
	StateAcknowledged State = "acknowledged"
	StateMitigated    State = "mitigated"
	StateResolved     State = "resolved"
)

// transitions encodes the lifecycle: triggered -> acknowledged -> mitigated
// -> resolved, with direct resolution allowed from any active state (an
// alert clearing auto-resolves its incident). Resolved is terminal.
var transitions = map[State][]State{
	StateTriggered:    {StateAcknowledged, StateResolved},
	StateAcknowledged: {StateMitigated, StateResolved},
	StateMitigated:    {StateResolved},
	StateResolved:     {},
}

// Event is one entry on an incident's timeline.
type Event struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"` // created | alert_fired | state_changed
	Message string    `json:"message"`
}

// Incident is a correlated group of alerts moving through the lifecycle.
type Incident struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Severity     string    `json:"severity"`
	State        State     `json:"state"`
	Fingerprints []string  `json:"fingerprints"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Timeline     []Event   `json:"timeline"`
}

// TransitionTo moves the incident to next, or errors if the lifecycle does
// not allow it. Invalid transitions indicate a bug or a race — they are
// never silently absorbed.
func (i *Incident) TransitionTo(next State, at time.Time) error {
	for _, allowed := range transitions[i.State] {
		if next == allowed {
			i.Append(at, "state_changed", fmt.Sprintf("%s -> %s", i.State, next))
			i.State = next
			return nil
		}
	}
	return fmt.Errorf("invalid transition %s -> %s", i.State, next)
}

// Append records an event on the timeline. The timeline is append-only: it
// is the audit log and the raw material for postmortems.
func (i *Incident) Append(at time.Time, kind, msg string) {
	i.Timeline = append(i.Timeline, Event{At: at, Kind: kind, Message: msg})
	i.UpdatedAt = at
}
