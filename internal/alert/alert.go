// Package alert defines Vigil's normalized alert model. Every monitoring
// source (Alertmanager, Grafana, Sentry, ...) is converted to this shape at
// the edge, so nothing downstream depends on where an alert came from.
package alert

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"
)

const (
	StatusFiring   = "firing"
	StatusResolved = "resolved"
)

// Alert is one normalized alert occurrence.
type Alert struct {
	Fingerprint string            `json:"fingerprint"`
	Name        string            `json:"name"`
	Severity    string            `json:"severity"`
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	Source      string            `json:"source"`
}

// Fingerprint hashes the sorted label set into a stable 16-hex-char identity.
// Labels are an alert's identity; annotations are display text and are
// deliberately excluded — the same alert re-firing with a reworded summary
// must keep the same fingerprint. Keys and values are NUL-delimited so that
// adjacent pairs cannot collide ({"a":"bc"} vs {"ab":"c"}).
func Fingerprint(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(labels[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
