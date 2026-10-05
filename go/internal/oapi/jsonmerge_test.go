package oapi

import (
	"encoding/json"
	"testing"
)

// Every expected value below was captured from
// github.com/oapi-codegen/runtime@v1.6.0 and matched, apart from the array
// case, which is a deliberate divergence documented on JSONMerge.
func TestJSONMerge(t *testing.T) {
	cases := []struct {
		name  string
		data  json.RawMessage
		patch json.RawMessage
		want  string
	}{
		{
			"disjoint keys are combined",
			raw(`{"kind":"volume"}`), raw(`{"volume_id":"abc"}`),
			`{"kind":"volume","volume_id":"abc"}`,
		},
		{
			"patch wins on conflict",
			raw(`{"kind":"volume","size":40}`), raw(`{"size":80}`),
			`{"kind":"volume","size":80}`,
		},
		// A zero-value union holds a nil json.RawMessage; this is what every
		// From*-then-Merge* sequence starts from.
		{
			"nil data is an empty object",
			nil, raw(`{"kind":"image"}`),
			`{"kind":"image"}`,
		},
		{
			"nil patch leaves data alone",
			raw(`{"kind":"image"}`), nil,
			`{"kind":"image"}`,
		},
		{
			"both nil",
			nil, nil,
			`{}`,
		},
		{
			"nested objects merge recursively",
			raw(`{"a":{"x":1,"y":2}}`), raw(`{"a":{"y":3}}`),
			`{"a":{"x":1,"y":3}}`,
		},
		{
			"explicit null overwrites",
			raw(`{"a":1}`), raw(`{"a":null}`),
			`{"a":null}`,
		},
		// Arrays are replaced, not merged element by element. This is the one
		// place the upstream implementation behaves differently.
		{
			"arrays are replaced wholesale",
			raw(`{"tags":["a","b","c"]}`), raw(`{"tags":["z"]}`),
			`{"tags":["z"]}`,
		},
		{
			"object replaces scalar",
			raw(`{"a":1}`), raw(`{"a":{"b":2}}`),
			`{"a":{"b":2}}`,
		},
		{
			"scalar replaces object",
			raw(`{"a":{"b":2}}`), raw(`{"a":1}`),
			`{"a":1}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := JSONMerge(tc.data, tc.patch)
			if err != nil {
				t.Fatalf("returned error: %v", err)
			}
			if normalizeJSON(t, got) != normalizeJSON(t, []byte(tc.want)) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// Decoding through interface{} would round this through float64 and lose the
// low-order digits, so the merge has to keep the original bytes.
func TestJSONMergePreservesNumericPrecision(t *testing.T) {
	const big = `123456789012345678901234567890.123456789`

	got, err := JSONMerge(raw(`{"amount":`+big+`}`), raw(`{"currency":"VND"}`))
	if err != nil {
		t.Fatalf("returned error: %v", err)
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if string(out["amount"]) != big {
		t.Errorf("amount = %s, want %s", out["amount"], big)
	}
}

func TestJSONMergeRejectsMalformedJSON(t *testing.T) {
	if _, err := JSONMerge(raw(`{"a":`), raw(`{"b":1}`)); err == nil {
		t.Error("malformed data merged without error")
	}
	if _, err := JSONMerge(raw(`{"a":1}`), raw(`not json`)); err == nil {
		t.Error("malformed patch merged without error")
	}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }
