// Package notify delivers incident lifecycle events to external channels
// (Slack first). Delivery is best-effort by design: losing a notification is
// acceptable, blocking or failing alert ingestion is not.
package notify

import (
	"context"

	"github.com/vandan08/vigil/internal/incident"
)

// Kind labels what happened to an incident.
type Kind string

const (
	KindOpened   Kind = "opened"
	KindResolved Kind = "resolved"
)

// Event is one notification-worthy incident change. Incident is a snapshot
// copy (the store never hands out live pointers), so an Event is safe to
// read from any goroutine.
type Event struct {
	Kind     Kind
	Incident incident.Incident
}

// Notifier delivers one event to one destination.
type Notifier interface {
	Send(ctx context.Context, ev Event) error
}
