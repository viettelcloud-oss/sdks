package oapi

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Named string types stand in for the generated enums, which reach
// StyleParamWithOptions as named types over string.
type (
	powerState   string
	flavorFamily string
)

// Every expected value below was captured from
// github.com/oapi-codegen/runtime@v1.6.0 and matched byte for byte, so these
// lock in compatibility with the code oapi-codegen generates against.
func TestStyleParamMatchesGeneratedCallSites(t *testing.T) {
	id := uuid.MustParse("3fa85f64-5717-4562-b3fc-2c963f66afa6")
	str := "web-01"
	// Exercises every escaping rule at once: space, '/', and '+'.
	messy := "web 01 / prod+dev"
	num := 25
	yes := true
	states := []powerState{"RUNNING", "STOPPED"}
	family := flavorFamily("general")
	none := []powerState{}

	cases := []struct {
		name    string
		style   string
		explode bool
		param   string
		value   any
		opts    StyleParamOptions
		want    string
	}{
		// form + explode, ParamLocationQuery — 105 generated call sites.
		{
			"query string", "form", true, "status", &str,
			StyleParamOptions{ParamLocation: ParamLocationQuery},
			"status=web-01",
		},
		{
			"query string is escaped", "form", true, "name", &messy,
			StyleParamOptions{ParamLocation: ParamLocationQuery},
			"name=web+01+%2F+prod%2Bdev",
		},
		{
			"query int", "form", true, "page_size", &num,
			StyleParamOptions{ParamLocation: ParamLocationQuery, Type: "integer"},
			"page_size=25",
		},
		{
			"query bool", "form", true, "bootable", &yes,
			StyleParamOptions{ParamLocation: ParamLocationQuery},
			"bootable=true",
		},
		{
			"query uuid", "form", true, "region_id", &id,
			StyleParamOptions{ParamLocation: ParamLocationQuery},
			"region_id=3fa85f64-5717-4562-b3fc-2c963f66afa6",
		},
		{
			"query enum", "form", true, "family", &family,
			StyleParamOptions{ParamLocation: ParamLocationQuery},
			"family=general",
		},
		// Exploded slices repeat the name; the generated client splits the
		// result on "&" to get one fragment per value.
		{
			"query enum slice repeats the name", "form", true, "power_state", &states,
			StyleParamOptions{ParamLocation: ParamLocationQuery},
			"power_state=RUNNING&power_state=STOPPED",
		},
		{
			"query empty slice", "form", true, "power_state", &none,
			StyleParamOptions{ParamLocation: ParamLocationQuery},
			"power_state=",
		},

		// simple, ParamLocationPath — 37 generated call sites.
		{
			"path uuid", "simple", false, "server_id", id,
			StyleParamOptions{ParamLocation: ParamLocationPath, Type: "string", Format: "uuid"},
			"3fa85f64-5717-4562-b3fc-2c963f66afa6",
		},
		// Path escaping differs from query escaping: space is %20, not '+',
		// and a literal '+' is left alone.
		{
			"path string is escaped", "simple", false, "name", messy,
			StyleParamOptions{ParamLocation: ParamLocationPath, Type: "string"},
			"web%2001%20%2F%20prod+dev",
		},

		// simple, ParamLocationHeader — 58 generated call sites.
		{
			"header uuid", "simple", false, "Project-ID", id,
			StyleParamOptions{ParamLocation: ParamLocationHeader, Type: "string", Format: "uuid"},
			"3fa85f64-5717-4562-b3fc-2c963f66afa6",
		},
		{
			"header value is not escaped", "simple", false, "X-Thing", messy,
			StyleParamOptions{ParamLocation: ParamLocationHeader, Type: "string"},
			"web 01 / prod+dev",
		},

		// Shapes the spec does not produce today, kept so a future spec change
		// cannot alter them unnoticed.
		{
			"form without explode joins on comma", "form", false, "power_state", &states,
			StyleParamOptions{ParamLocation: ParamLocationQuery},
			"power_state=RUNNING,STOPPED",
		},
		{
			"simple slice joins on comma", "simple", false, "ids", &states,
			StyleParamOptions{ParamLocation: ParamLocationPath},
			"RUNNING,STOPPED",
		},
		{
			"allowReserved keeps reserved characters", "form", true, "filter", &messy,
			StyleParamOptions{ParamLocation: ParamLocationQuery, AllowReserved: true},
			"filter=web%2001%20/%20prod+dev",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := StyleParamWithOptions(tc.style, tc.explode, tc.param, tc.value, tc.opts)
			if err != nil {
				t.Fatalf("returned error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Anything outside the supported subset must fail at the call, not produce
// something plausible-looking that the server will reject or misread.
func TestStyleParamRejectsUnsupportedShapes(t *testing.T) {
	str := "x"
	type point struct {
		X int `json:"x"`
	}

	cases := []struct {
		name    string
		style   string
		value   any
		wantErr string
	}{
		{"deepObject style", "deepObject", &str, "unsupported style"},
		{"label style", "label", &str, "unsupported style"},
		{"matrix style", "matrix", &str, "unsupported style"},
		{"spaceDelimited style", "spaceDelimited", &str, "unsupported style"},
		{"pipeDelimited style", "pipeDelimited", &str, "unsupported style"},
		{"object value", "form", point{X: 1}, "cannot serialize object"},
		{"map value", "form", map[string]string{"a": "b"}, "cannot serialize map"},
		{"nil pointer", "form", (*string)(nil), "nil pointer"},
		{"nil value", "form", nil, "is nil"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := StyleParamWithOptions(tc.style, true, "p", tc.value,
				StyleParamOptions{ParamLocation: ParamLocationQuery})
			if err == nil {
				t.Fatalf("succeeded, want an error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), "'p'") {
				t.Errorf("error = %q, want it to name the parameter", err)
			}
		})
	}
}

func TestStyleParamByteFormatIsBase64(t *testing.T) {
	got, err := StyleParamWithOptions("form", true, "blob", []byte("hello"),
		StyleParamOptions{ParamLocation: ParamLocationQuery, Type: "string", Format: "byte"})
	if err != nil {
		t.Fatalf("returned error: %v", err)
	}
	// Without Format: "byte" this would serialize as a list of numbers.
	if want := "blob=aGVsbG8%3D"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
