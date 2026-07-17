package alert

import (
	"testing"
	"time"
)

func TestDeduperWindow(t *testing.T) {
	d := NewDeduper(5 * time.Minute)
	t0 := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)

	if d.Seen("fp1", t0) {
		t.Fatal("first firing must not be deduped")
	}
	if d.Seen("fp2", t0) {
		t.Fatal("distinct fingerprints must be tracked independently")
	}
	if !d.Seen("fp1", t0.Add(time.Minute)) {
		t.Fatal("re-firing inside the window must be deduped")
	}
	if d.Seen("fp1", t0.Add(10*time.Minute)) {
		t.Fatal("firing after the window has passed must not be deduped")
	}
}
