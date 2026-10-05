// Package oapi is the runtime surface oapi-codegen's generated clients call
// into: parameter styling and the JSON merge used by union types.
//
// It replaces github.com/oapi-codegen/runtime, the SDK's last dependency
// outside google/uuid. Only what the generated clients actually reach is kept;
// unsupported parameter styles and formats return an error rather than guess,
// so a future spec change fails at the call, not on the wire. Identifiers match
// upstream exactly, so generation only repoints the import (aliased `runtime`);
// see pipeline/main.py.
//
// Portions are derived from styleparam.go and encoder.go in
// github.com/oapi-codegen/runtime, Copyright 2019 DeepMap, Inc., used under the
// Apache License 2.0.
package oapi

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ParamLocation is where a serialized parameter is headed. It selects the
// escaping rule, and nothing else.
type ParamLocation int

const (
	ParamLocationUndefined ParamLocation = iota
	ParamLocationQuery
	ParamLocationPath
	ParamLocationHeader
	ParamLocationCookie
)

// StyleParamOptions carries the OpenAPI metadata for one parameter.
//
// The field set mirrors the upstream struct so the generated code compiles
// against it unchanged, but only ParamLocation and AllowReserved affect the
// result on the client path. Type is never read (upstream does not read it
// either), Required is a server-side binding concern, and Format matters only
// for `format: byte`.
type StyleParamOptions struct {
	// ParamLocation controls URL escaping behavior.
	ParamLocation ParamLocation
	// Type is the OpenAPI type of the parameter (e.g. "string", "integer").
	Type string
	// Format is the OpenAPI format of the parameter (e.g. "byte", "date-time").
	Format string
	// Required indicates whether the parameter is required.
	Required bool
	// AllowReserved leaves RFC 3986 reserved characters unencoded in query
	// parameter values.
	AllowReserved bool
}

// StyleParamWithOptions serializes a Go value into an OpenAPI-styled parameter
// string.
//
// Supported styles are "form" (query parameters) and "simple" (path and header
// parameters), which is what this SDK's spec uses. The other OpenAPI styles —
// deepObject, label, matrix, spaceDelimited, pipeDelimited — return an error,
// as do object- and map-valued parameters.
func StyleParamWithOptions(style string, explode bool, paramName string, value any, opts StyleParamOptions) (string, error) {
	t := reflect.TypeOf(value)
	if t == nil {
		return "", fmt.Errorf("parameter '%s': value is nil", paramName)
	}
	v := reflect.ValueOf(value)

	// Optional parameters arrive by pointer.
	if t.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "", fmt.Errorf("parameter '%s': value is a nil pointer", paramName)
		}
		v = reflect.Indirect(v)
		t = v.Type()
	}

	// A type that marshals itself to text knows best — except time.Time,
	// whose MarshalText is RFC 3339 with a different precision than the
	// OpenAPI date-time form used below.
	if tm, ok := value.(encoding.TextMarshaler); ok && !isTimeLike(t) {
		b, err := tm.MarshalText()
		if err != nil {
			return "", fmt.Errorf("parameter '%s': marshaling as text: %w", paramName, err)
		}
		return stylePrimitive(style, paramName, opts, string(b))
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		// `format: byte` means a base64 string, not a list of numbers.
		if opts.Format == "byte" && t.Elem().Kind() == reflect.Uint8 {
			return stylePrimitive(style, paramName, opts,
				base64.StdEncoding.EncodeToString(v.Bytes()))
		}
		values := make([]any, v.Len())
		for i := range values {
			values[i] = v.Index(i).Interface()
		}
		return styleSlice(style, explode, paramName, opts, values)
	case reflect.Struct:
		// time.Time and the UUID array type are handled above; a struct
		// reaching here is a genuine object parameter.
		if s, ok := knownStructToString(v, t); ok {
			return stylePrimitive(style, paramName, opts, s)
		}
		return "", fmt.Errorf(
			"parameter '%s': cannot serialize object of type %s; "+
				"style-based serialization is only defined for primitives and arrays",
			paramName, t,
		)
	case reflect.Map:
		return "", fmt.Errorf(
			"parameter '%s': cannot serialize map of type %s; "+
				"style-based serialization is only defined for primitives and arrays",
			paramName, t,
		)
	default:
		return stylePrimitive(style, paramName, opts, value)
	}
}

// styleSlice serializes a multi-valued parameter.
//
//	form,   explode=true   ->  name=a&name=b   (one parameter repeated)
//	form,   explode=false  ->  name=a,b
//	simple, explode=either ->  a,b
func styleSlice(style string, explode bool, paramName string, opts StyleParamOptions, values []any) (string, error) {
	var prefix, separator string
	switch style {
	case "simple":
		separator = ","
	case "form":
		prefix = escapeParameterName(paramName, opts.ParamLocation) + "="
		if explode {
			separator = "&" + prefix
		} else {
			separator = ","
		}
	default:
		return "", unsupportedStyle(style, paramName)
	}

	parts := make([]string, len(values))
	for i, item := range values {
		s, err := primitiveToString(item)
		if err != nil {
			return "", fmt.Errorf("parameter '%s': %w", paramName, err)
		}
		parts[i] = escapeParameterString(s, opts.ParamLocation, opts.AllowReserved)
	}
	return prefix + strings.Join(parts, separator), nil
}

// stylePrimitive serializes a single-valued parameter.
//
//	form   ->  name=value
//	simple ->  value
func stylePrimitive(style string, paramName string, opts StyleParamOptions, value any) (string, error) {
	s, err := primitiveToString(value)
	if err != nil {
		return "", fmt.Errorf("parameter '%s': %w", paramName, err)
	}

	var prefix string
	switch style {
	case "simple":
	case "form":
		prefix = escapeParameterName(paramName, opts.ParamLocation) + "="
	default:
		return "", unsupportedStyle(style, paramName)
	}
	return prefix + escapeParameterString(s, opts.ParamLocation, opts.AllowReserved), nil
}

// primitiveToString renders a scalar. It switches on Kind rather than Type so
// that named types — the generated string enums, for instance — are handled
// like their underlying kind.
func primitiveToString(value any) (string, error) {
	v := reflect.Indirect(reflect.ValueOf(value))
	if !v.IsValid() {
		return "", fmt.Errorf("value is nil")
	}
	t := v.Type()

	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'f', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.String:
		return v.String(), nil
	}

	if s, ok := knownStructToString(v, t); ok {
		return s, nil
	}
	if tm, ok := value.(encoding.TextMarshaler); ok {
		b, err := tm.MarshalText()
		if err != nil {
			return "", fmt.Errorf("marshaling %s as text: %w", t, err)
		}
		return string(b), nil
	}
	if s, ok := value.(fmt.Stringer); ok {
		return s.String(), nil
	}
	// A JSON-marshaling scalar, such as a union branch used as a parameter.
	if m, ok := value.(json.Marshaler); ok {
		b, err := m.MarshalJSON()
		if err != nil {
			return "", fmt.Errorf("marshaling %s as JSON: %w", t, err)
		}
		var decoded any
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		if err := dec.Decode(&decoded); err != nil {
			return "", fmt.Errorf("decoding JSON for %s: %w", t, err)
		}
		if _, isString := decoded.(string); !isString {
			if _, isNumber := decoded.(json.Number); !isNumber {
				return "", fmt.Errorf("cannot serialize %s: not a scalar", t)
			}
		}
		return fmt.Sprintf("%v", decoded), nil
	}
	return "", fmt.Errorf("unsupported parameter type %s", t)
}

// knownStructToString renders the struct types that appear as scalars on the
// wire. Today that is time.Time and anything convertible to it.
func knownStructToString(v reflect.Value, t reflect.Type) (string, bool) {
	if isTimeLike(t) {
		tv := v.Convert(reflect.TypeFor[time.Time]()).Interface().(time.Time)
		return tv.Format(time.RFC3339Nano), true
	}
	return "", false
}

func isTimeLike(t reflect.Type) bool {
	return t.ConvertibleTo(reflect.TypeFor[time.Time]())
}

func unsupportedStyle(style, paramName string) error {
	return fmt.Errorf(
		"parameter '%s': unsupported style %q; this SDK generates only "+
			"'form' and 'simple' parameters", paramName, style,
	)
}

// escapeParameterName escapes a parameter name. AllowReserved applies to
// values only, so names are always fully escaped.
func escapeParameterName(name string, location ParamLocation) string {
	return escapeParameterString(name, location, false)
}

// escapeParameterString escapes a value for where it is going. Query and path
// have different rules; headers and cookies are written verbatim.
func escapeParameterString(value string, location ParamLocation, allowReserved bool) string {
	switch location {
	case ParamLocationQuery:
		if allowReserved {
			return escapeQueryAllowReserved(value)
		}
		return url.QueryEscape(value)
	case ParamLocationPath:
		return url.PathEscape(value)
	default:
		return value
	}
}

// escapeQueryAllowReserved percent-encodes a query value while leaving the RFC
// 3986 reserved characters alone, which is what OpenAPI's allowReserved asks
// for. Only characters that are neither unreserved nor reserved are encoded.
func escapeQueryAllowReserved(value string) string {
	const reserved = `:/?#[]@!$&'()*+,;=`

	var buf strings.Builder
	for _, b := range []byte(value) {
		if isUnreserved(b) || strings.IndexByte(reserved, b) >= 0 {
			buf.WriteByte(b)
		} else {
			fmt.Fprintf(&buf, "%%%02X", b)
		}
	}
	return buf.String()
}

// isUnreserved reports whether b is an RFC 3986 unreserved character:
// ALPHA / DIGIT / "-" / "." / "_" / "~".
func isUnreserved(b byte) bool {
	return (b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '.' || b == '_' || b == '~'
}
