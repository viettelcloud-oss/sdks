// Package observability provides the Viettel Cloud SDK client for cloud
// observability. It covers CRUD for receivers, tokens, dashboards, log
// dashboards, metric/log rule groups and rules, rule-group receivers and log
// stores, plus read-only access to dashboard/log-rule/metric-rule templates
// (get, list) and incidents (list only).
//
// Create a client with NewClient and authenticate with WithPAT. Generated
// HTTP plumbing stays under internal/gen; consumers use only this facade
// package.
package observability

//go:generate make -C .. generate SERVICES=observability
