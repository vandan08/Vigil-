package alert

import (
	"sync"
	"time"
)

// Deduper suppresses re-firings of the same alert inside a sliding window.
// It is the first line of defense against alert storms: a flapping alert
// evaluated every 30s must not spam the incident timeline on every interval.
type Deduper struct {
	window   time.Duration
	mu       sync.Mutex
	lastSeen map[string]time.Time
}

func NewDeduper(window time.Duration) *Deduper {
	return &Deduper{window: window, lastSeen: make(map[string]time.Time)}
}

// Seen reports whether fingerprint fp already fired inside the window, and
// records this firing. The caller supplies now so tests control the clock.
func (d *Deduper) Seen(fp string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	last, ok := d.lastSeen[fp]
	d.lastSeen[fp] = now
	if ok && now.Sub(last) < d.window {
		return true
	}

	// Opportunistic prune on the slow path keeps the map bounded without a
	// background goroutine.
	for k, t := range d.lastSeen {
		if now.Sub(t) >= d.window {
			delete(d.lastSeen, k)
		}
	}
	return false
}
