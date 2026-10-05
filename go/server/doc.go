// Package server provides the Viettel Cloud SDK client for virtual servers.
//
// It covers the server lifecycle: create, get, partial update, and delete.
// Create a client with NewClient and authenticate with WithPAT. Generated HTTP
// plumbing stays under internal/gen; consumers use only this facade package.
package server

//go:generate make -C .. generate SERVICES=server
