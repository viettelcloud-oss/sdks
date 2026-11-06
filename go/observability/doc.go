// Package observability provides the Viettel Cloud SDK client for the Observability API.
//
// The client has one method for each operation in the Observability API spec.
// See operations.go for the full list. Create a client with NewClient and
// authenticate with WithPAT. Generated HTTP plumbing stays under internal/gen;
// consumers use only this facade package.
package observability

//go:generate make -C .. generate SERVICES=observability
