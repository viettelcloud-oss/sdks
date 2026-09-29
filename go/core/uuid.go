package core

import (
	"github.com/google/uuid"
)

// UUID is the type used for every resource identifier in the API.
//
// It is an alias for github.com/google/uuid.UUID, not a distinct type, so a
// uuid.UUID from anywhere else — a database driver, another SDK, your own
// code — can be passed directly with no conversion:
//
//	id := uuid.MustParse("3fa85f64-5717-4562-b3fc-2c963f66afa6")
//	server, err := client.GetServer(ctx, id, params)
//
// The helpers below cover the common cases, so importing google/uuid yourself
// is optional.
type UUID = uuid.UUID

// NilUUID is the all-zero UUID.
var NilUUID = uuid.Nil

// ParseUUID parses the canonical 8-4-4-4-12 text form of a UUID. It also
// accepts the braced, urn:uuid:, and unhyphenated variants.
func ParseUUID(s string) (UUID, error) {
	return uuid.Parse(s)
}

// MustParseUUID is ParseUUID for identifiers known to be valid at compile
// time, such as constants and test fixtures. It panics on invalid input.
func MustParseUUID(s string) UUID {
	return uuid.MustParse(s)
}

// NewUUID returns a new randomly generated (version 4) UUID. It panics if the
// system source of randomness fails, which in practice means the process is
// already unable to make progress.
func NewUUID() UUID {
	return uuid.New()
}
