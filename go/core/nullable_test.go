package core

import (
	"encoding/json"
	"testing"
)

func TestNullableFromPtr(t *testing.T) {
	type body struct {
		Retries Nullable[int] `json:"retries,omitempty"`
	}

	value := 3
	cases := []struct {
		name string
		in   Nullable[int]
		want string
	}{
		{name: "nil pointer omits the field", in: NullableFromPtr[int](nil), want: `{}`},
		{name: "pointer sends its value", in: NullableFromPtr(&value), want: `{"retries":3}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(body{Retries: tc.in})
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Marshal = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestNullableToPtr(t *testing.T) {
	var unset Nullable[string]
	if got := NullableToPtr(unset); got != nil {
		t.Errorf("unset: got %q, want nil", *got)
	}

	var null Nullable[string]
	null.SetNull()
	if got := NullableToPtr(null); got != nil {
		t.Errorf("null: got %q, want nil", *got)
	}

	var set Nullable[string]
	set.Set("web")
	if got := NullableToPtr(set); got == nil || *got != "web" {
		t.Errorf("set: got %v, want pointer to %q", got, "web")
	}
}
