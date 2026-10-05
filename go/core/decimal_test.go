package core

import (
	"encoding/json"
	"testing"
)

func TestDecimalUnmarshalAcceptsNumberAndString(t *testing.T) {
	// The spec types these fields as number|string because the backend accepts
	// either form; both must decode to the same exact text.
	cases := []struct {
		name string
		json string
		want string
	}{
		{"number", `12.34`, "12.34"},
		{"string", `"12.34"`, "12.34"},
		{"integer number", `7`, "7"},
		{"integer string", `"7"`, "7"},
		{"negative", `-0.5`, "-0.5"},
		{"exponent", `1.5e-8`, "1.5e-8"},
		{"null", `null`, "0"},
		{"high precision", `"12345678901234567890.123456789"`, "12345678901234567890.123456789"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d Decimal
			if err := json.Unmarshal([]byte(tc.json), &d); err != nil {
				t.Fatalf("Unmarshal(%s) returned error: %v", tc.json, err)
			}
			if got := d.String(); got != tc.want {
				t.Errorf("Unmarshal(%s).String() = %q, want %q", tc.json, got, tc.want)
			}
		})
	}
}

func TestDecimalRejectsNonDecimals(t *testing.T) {
	for _, input := range []string{`"abc"`, `"1/3"`, `"0x1p-2"`, `"12.3.4"`, `"1,234"`, `true`} {
		var d Decimal
		if err := json.Unmarshal([]byte(input), &d); err == nil {
			t.Errorf("Unmarshal(%s) succeeded with %q, want an error", input, d.String())
		}
	}
}

func TestDecimalRoundTripsWithoutPrecisionLoss(t *testing.T) {
	// The value that motivates carrying money as text rather than float64:
	// 0.1 + 0.2 is not 0.3 in binary floating point.
	const exact = "12345678901234567890.123456789"

	var d Decimal
	if err := json.Unmarshal([]byte(`"`+exact+`"`), &d); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if string(out) != `"`+exact+`"` {
		t.Errorf("Marshal = %s, want %q", out, exact)
	}

	// Struct round trip, the shape the generated models actually use.
	type payload struct {
		Amount Decimal `json:"amount"`
	}
	var p payload
	if err := json.Unmarshal([]byte(`{"amount": `+exact+`}`), &p); err != nil {
		t.Fatalf("struct Unmarshal returned error: %v", err)
	}
	if p.Amount.String() != exact {
		t.Errorf("struct round trip = %q, want %q", p.Amount.String(), exact)
	}
}

func TestDecimalZeroValue(t *testing.T) {
	var d Decimal
	if !d.IsZero() {
		t.Error("zero value: IsZero() = false, want true")
	}
	if d.String() != "0" {
		t.Errorf("zero value: String() = %q, want \"0\"", d.String())
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("zero value: Marshal returned error: %v", err)
	}
	if string(out) != `"0"` {
		t.Errorf("zero value: Marshal = %s, want \"0\"", out)
	}
}

func TestDecimalArithmeticHelpers(t *testing.T) {
	d := MustDecimal("2.5")

	f, err := d.Float64()
	if err != nil {
		t.Fatalf("Float64 returned error: %v", err)
	}
	if f != 2.5 {
		t.Errorf("Float64() = %v, want 2.5", f)
	}

	// Cmp is numeric, so differing text for the same value compares equal.
	cmp, err := d.Cmp(MustDecimal("2.50"))
	if err != nil {
		t.Fatalf("Cmp returned error: %v", err)
	}
	if cmp != 0 {
		t.Errorf("Cmp(2.5, 2.50) = %d, want 0", cmp)
	}

	cmp, err = d.Cmp(MustDecimal("10"))
	if err != nil {
		t.Fatalf("Cmp returned error: %v", err)
	}
	if cmp != -1 {
		t.Errorf("Cmp(2.5, 10) = %d, want -1", cmp)
	}
}

func TestNewDecimalRejectsInvalidInput(t *testing.T) {
	if _, err := NewDecimal("not a number"); err == nil {
		t.Error("NewDecimal(\"not a number\") succeeded, want an error")
	}
	// Empty input is the zero value, not an error.
	d, err := NewDecimal("")
	if err != nil {
		t.Fatalf("NewDecimal(\"\") returned error: %v", err)
	}
	if !d.IsZero() {
		t.Error("NewDecimal(\"\") is not the zero value")
	}
}
