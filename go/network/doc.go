// Package network provides the Viettel Cloud SDK client for VPCs and elastic IPs.
//
// It covers the VPC lifecycle (create, get, update, delete) and elastic IP
// management (create, get, delete). Create a client with NewClient and
// authenticate with WithPAT. Generated HTTP plumbing stays under internal/gen;
// consumers use only this facade package.
package network

//go:generate make -C .. generate SERVICES=network
