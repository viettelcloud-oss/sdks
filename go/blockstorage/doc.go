// Package blockstorage provides the Viettel Cloud SDK client for volumes and volume types.
//
// It covers the volume lifecycle (create, get, update, delete) and listing the
// available volume types. Create a client with NewClient and authenticate with
// WithPAT. Generated HTTP plumbing stays under internal/gen; consumers use only
// this facade package.
package blockstorage

//go:generate make -C .. generate SERVICES=blockstorage
