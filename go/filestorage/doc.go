// Package filestorage provides the Viettel Cloud SDK client for the File Storage API.
//
// The client has one method for each operation in the File Storage API spec.
// See operations.go for the full list. Create a client with NewClient and
// authenticate with WithPAT. Generated HTTP plumbing stays under internal/gen;
// consumers use only this facade package.
package filestorage

//go:generate make -C .. generate SERVICES=filestorage
