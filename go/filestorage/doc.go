// Package filestorage provides the Viettel Cloud SDK client for cloud file storage.
//
// It covers the filesystem lifecycle (create, get, update, delete), filesystem
// access rules, filesystem types, filesystem snapshots, and snapshot policies.
// Create a client with NewClient and authenticate with WithPAT. Generated HTTP
// plumbing stays under internal/gen; consumers use only this facade package.
package filestorage

//go:generate make -C .. generate SERVICES=filestorage
