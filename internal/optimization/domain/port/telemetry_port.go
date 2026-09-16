// Package port holds the outbound interfaces Optimization's domain/service
// layer depends on. Concrete gRPC implementations live in
// adapter/outbound/*_grpc (a later step); domain/service only ever sees
// these interfaces, so Evaluator/Allocate can be unit-tested with fakes.
package port

import "context"

// Snapshot is Optimization's own read model for "latest known values for
// one CU". It mirrors telemetry.Snapshot's shape (a metric-name -> value
// map plus a staleness flag) closely enough for rule evaluation, without
// domain/service importing the telemetry proto package — that dependency
// belongs to the telemetry_grpc adapter (a later step), which converts a
// telemetrypb.Snapshot into this struct.
type Snapshot struct {
	CUCode  string
	Metrics map[string]float64
	// Stale mirrors telemetrypb.Snapshot.Stale. Evaluator must not decide
	// on stale data — see discussion/2026-09-04.md §一 risk #5: deciding
	// twice on the same reading is how oscillation happens.
	Stale bool
}

// TelemetryPort is what the rule engine needs to read current state.
// Optimization deliberately bypasses Resource's three-level Runtime cache
// and talks to Telemetry directly (discussion §3.3): a decision loop's
// freshness requirement is stricter than what a polling cache can offer,
// and (as of architecture.md §3.3.1) that cache's write path is empty
// anyway.
type TelemetryPort interface {
	// GetSnapshot returns the latest known metrics for one CU. Implementations
	// should map to telemetry's GetSnapshot RPC 1:1 (no aggregation/derivation
	// here — that belongs in domain/service if it's ever needed).
	GetSnapshot(ctx context.Context, tenantID, cuCode string) (Snapshot, error)
}
