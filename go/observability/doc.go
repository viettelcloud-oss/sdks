// Package observability provides the Viettel Cloud SDK client for cloud
// observability. It covers CRUD for receivers, tokens, dashboards, log
// dashboards, metric/log rule groups and rules, rule-group receivers and log
// stores, plus listing dashboard/log-rule/metric-rule templates — the
// backend resolves a template fully from its id, so listing is only there
// to discover one, not to read its content.
//
// The surface is deliberately the minimum Terraform's provisioning needs:
// every public method is an interface consumers can come to depend on, so
// it stays out unless provisioning needs it (see go/pipeline.yaml's
// drop_paths for what's excluded and why — template detail/label reads,
// incident history, log search/query, and bulk/cascade delete).
//
// Create a client with NewClient and authenticate with WithPAT. Generated
// HTTP plumbing stays under internal/gen; consumers use only this facade
// package.
package observability

//go:generate make -C .. generate SERVICES=observability
