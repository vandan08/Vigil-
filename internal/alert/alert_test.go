package alert

import "testing"

func TestFingerprintIsStableAndLabelOrderIndependent(t *testing.T) {
	a := Fingerprint(map[string]string{"alertname": "HighErrorRate", "service": "checkout"})
	b := Fingerprint(map[string]string{"service": "checkout", "alertname": "HighErrorRate"})
	if a != b {
		t.Fatalf("same labels produced different fingerprints: %s vs %s", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("fingerprint length = %d, want 16", len(a))
	}
}

func TestFingerprintDiffersAcrossLabelSets(t *testing.T) {
	a := Fingerprint(map[string]string{"alertname": "HighErrorRate", "service": "checkout"})
	b := Fingerprint(map[string]string{"alertname": "HighErrorRate", "service": "payments"})
	if a == b {
		t.Fatal("different label values must produce different fingerprints")
	}
}

func TestFingerprintDelimitsKeysAndValues(t *testing.T) {
	// Without delimiters these two would hash the same byte stream.
	a := Fingerprint(map[string]string{"a": "bc"})
	b := Fingerprint(map[string]string{"ab": "c"})
	if a == b {
		t.Fatal("adjacent key/value bytes must not collide")
	}
}
