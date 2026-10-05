package oapi

import (
	"encoding/json"
	"testing"
)

// normalizeJSON re-encodes JSON so two encodings of the same value compare
// equal regardless of key order or whitespace.
func normalizeJSON(t *testing.T, raw []byte) string {
	t.Helper()
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("normalizeJSON(%s): %v", raw, err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("normalizeJSON(%s): %v", raw, err)
	}
	return string(out)
}
