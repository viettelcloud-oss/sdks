package core

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// Decimal is an exact decimal number, used for money and other quantities
// where binary floating point would lose precision.
//
// The API describes these fields as `number | string` because the backend
// accepts either form on input and serializes them as strings on output.
// Decimal absorbs that asymmetry: it unmarshals from a JSON number, a JSON
// string, or null, and always marshals back as a JSON string so no precision
// is lost on the round trip.
//
// The value is carried as its exact decimal text — no parsing to float64
// happens unless you ask for it via Float64 or Rat.
//
//	var c core.Decimal
//	_ = json.Unmarshal([]byte(`12.34`), &c)   // number
//	_ = json.Unmarshal([]byte(`"12.34"`), &c) // string — same result
//	c.String()                                // "12.34"
//
// The zero value is a valid zero.
type Decimal struct {
	// text is the exact decimal literal, or "" for the zero value.
	text string
}

// decimalPattern matches a JSON number: an optional sign, digits with an
// optional fractional part, and an optional exponent. big.Rat.SetString is
// laxer than this — it also accepts forms like "1/3" and "0x1p-2" — so the
// pattern is what decides validity.
var decimalPattern = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$`)

// NewDecimal returns a Decimal holding the given decimal literal. It returns
// an error if s is not a valid decimal number.
func NewDecimal(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Decimal{}, nil
	}
	if !decimalPattern.MatchString(s) {
		return Decimal{}, fmt.Errorf("core: %q is not a valid decimal", s)
	}
	return Decimal{text: s}, nil
}

// MustDecimal is NewDecimal for literals known to be valid at compile time.
// It panics if s is not a valid decimal number.
func MustDecimal(s string) Decimal {
	d, err := NewDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

// String returns the exact decimal text. The zero value renders as "0".
func (d Decimal) String() string {
	if d.text == "" {
		return "0"
	}
	return d.text
}

// IsZero reports whether d is the zero value. It is a cheap check on the
// unset value, not a numeric comparison — Decimal{text: "0.00"} is not zero
// by this test. Use Cmp for numeric comparison.
func (d Decimal) IsZero() bool {
	return d.text == ""
}

// Rat returns the value as an exact rational, the lossless way to do
// arithmetic on a Decimal. It returns an error only if d holds text that did
// not come through NewDecimal or UnmarshalJSON.
func (d Decimal) Rat() (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(d.String())
	if !ok {
		return nil, fmt.Errorf("core: %q is not a valid decimal", d.text)
	}
	return r, nil
}

// Float64 returns the value as a float64. This is lossy for values that
// cannot be represented exactly in binary floating point; prefer Rat when the
// result feeds further arithmetic, and String when it feeds a display.
func (d Decimal) Float64() (float64, error) {
	r, err := d.Rat()
	if err != nil {
		return 0, err
	}
	f, _ := r.Float64()
	return f, nil
}

// Cmp compares d and other numerically, returning -1, 0 or +1. It returns an
// error if either value holds invalid text.
func (d Decimal) Cmp(other Decimal) (int, error) {
	a, err := d.Rat()
	if err != nil {
		return 0, err
	}
	b, err := other.Rat()
	if err != nil {
		return 0, err
	}
	return a.Cmp(b), nil
}

// MarshalJSON implements json.Marshaler, emitting the value as a JSON string
// so the exact decimal text survives the round trip.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON implements json.Unmarshaler, accepting a JSON number, a JSON
// string or null. null decodes to the zero value.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" {
		*d = Decimal{}
		return nil
	}
	// A JSON string carries the decimal literal; a JSON number is already one.
	if strings.HasPrefix(text, `"`) {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("core: decoding decimal: %w", err)
		}
		text = strings.TrimSpace(s)
	}
	parsed, err := NewDecimal(text)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// MarshalText implements encoding.TextMarshaler so a Decimal can be used
// directly as a query parameter or path segment.
func (d Decimal) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Decimal) UnmarshalText(text []byte) error {
	parsed, err := NewDecimal(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
