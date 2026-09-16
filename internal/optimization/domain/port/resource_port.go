package port

import "context"

// ResourcePort is what Allocate's AggregateTarget branch needs: each
// in-scope resource's rated capacity, to split an aggregate DeltaValue
// proportionally (discussion/2026-09-04.md §3.4's "按额定容量占比线性分摊").
//
// Optimization reads Resource's static config (Asset.RatedCapacityKW via
// Resource's ordinary read RPCs), not the AssetRuntime Redis cache —
// architecture.md §3.3.1 already found that cache's MaxChargePowerKW /
// MaxDischargePowerKW fields are never written by anything, so discussion's
// original draft ("按 MaxDischargePowerKW 占比分摊") would have divided by a
// value that is always nil in the current codebase. This is a correction
// made when turning the discussion into this implementation, recorded in
// the Optimization design plan §2.
type ResourcePort interface {
	// GetCapacityKW returns each scope entry's rated capacity in kW, keyed
	// by the same identifier passed in scope (CUCode or AssetID — v1 does
	// not need to distinguish which, since the concrete resource_grpc
	// adapter, a later step, is responsible for resolving either kind of
	// ID to a capacity figure). Entries with unknown capacity are omitted
	// from the result rather than zero-valued, so splitByCapacity can tell
	// "unknown" apart from "known to be zero-rated".
	GetCapacityKW(ctx context.Context, tenantID string, scope []string) (map[string]float64, error)
}
