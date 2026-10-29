package core

import (
	"github.com/oapi-codegen/nullable"
)

// Nullable is the type used for fields that distinguish "omitted" from
// "explicitly null" on the wire — a field generated code produces wherever
// the spec declares it `anyOf: [T, null]` (see nullable-type in
// go/pipeline.yaml).
//
// It is an alias for github.com/oapi-codegen/nullable.Nullable[T], not a
// distinct type, so you never need to import that package yourself:
//
//	var id core.Nullable[core.UUID]
//	id.Set(core.NewUUID())  // send a value
//	id.SetNull()            // send an explicit null, clearing the field
//	// leave id unset        // omit the field entirely, meaning "no change"
//
//	v, err := id.Get()      // err is non-nil if id is unset or null
//	id.IsNull()              // true only for the explicit-null state
type Nullable[T any] = nullable.Nullable[T]

// NullableFromPtr returns a Nullable holding *p, or an unset Nullable when p
// is nil. An unset field is omitted from the request, the same as a nil
// pointer was before these fields became Nullable; call SetNull to clear one.
func NullableFromPtr[T any](p *T) Nullable[T] {
	if p == nil {
		return nil
	}
	return nullable.NewNullableWithValue(*p)
}

// NullableToPtr returns a pointer to the value n holds, or nil when n is unset
// or null. Use it where the difference between those two states does not
// matter, such as reading a response field.
func NullableToPtr[T any](n Nullable[T]) *T {
	v, err := n.Get()
	if err != nil {
		return nil
	}
	return &v
}
