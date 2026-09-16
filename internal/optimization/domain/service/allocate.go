package service

import (
	"context"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
)

// Allocate turns a Target into concrete CommandSpecs ready to be packed
// into a dispatch.SubmitTaskRequest (that packing happens in
// application/command.RunDecisionCycle). See discussion/2026-09-04.md §3.4
// for why this mirrors dispatch's own Task -> Action -> Command
// decomposition one level up, and §3.5 for the finalized Target/Allocate
// signature this implements.
//
// Known limitation carried over from the design discussion: Go's type
// switch below has no compile-time exhaustiveness check. A new Target
// implementation that isn't added here compiles fine and fails at
// runtime via the default case — this is a documented trade-off of using
// an interface for Target, not an oversight (see model.Target's doc
// comment).
func Allocate(ctx context.Context, resource port.ResourcePort, target model.Target) ([]model.CommandSpec, error) {
	switch t := target.(type) {
	case model.PointTarget:
		return pointToCommands(t), nil
	case model.AggregateTarget:
		return splitByCapacity(ctx, resource, t)
	default:
		return nil, fmt.Errorf("optimization: unsupported target type %T", target)
	}
}

// pointToCommands maps a PointTarget 1:1 onto a single CommandSpec — the
// rule engine already resolved exactly which CU/Point/value to set, so
// there is nothing left to allocate.
func pointToCommands(t model.PointTarget) []model.CommandSpec {
	return []model.CommandSpec{
		{
			CUCode:   t.CUCode,
			PointKey: t.PointKey,
			Value:    t.Value,
		},
	}
}

// splitByCapacity implements discussion §3.4's "按额定容量占比线性分摊": each
// in-scope resource gets a share of DeltaValue proportional to its rated
// capacity (via ResourcePort.GetCapacityKW — Resource's static config, see
// that port's doc comment for why not the AssetRuntime cache).
//
// v1 has no real caller for this path — AggregateTarget only arrives once
// an external actor (Market, Phase D) hands Optimization a coarse-grained
// objective, and that service does not exist yet (see model.Target's doc
// comment). It is implemented and unit-tested now so the Target/Allocate
// contract does not need a breaking change on the day Market lands.
//
// Simplification, acceptable only because there is no real caller yet:
// t.Metric is used directly as the CommandSpec.PointKey for every CU in
// scope, i.e. this assumes one metric name maps to the same PointKey on
// every in-scope CU. Real resources have no such global naming guarantee
// (see model.SOCThresholdRule's doc comment on Point.PointKey being
// free-form) — when Market actually wires this path up, a per-CU
// metric-to-PointKey mapping (structured the way SOCThresholdRule binds
// explicitly) should replace this pass-through.
func splitByCapacity(ctx context.Context, resource port.ResourcePort, t model.AggregateTarget) ([]model.CommandSpec, error) {
	if len(t.Scope) == 0 {
		return nil, fmt.Errorf("optimization: aggregate target has empty scope")
	}

	capacities, err := resource.GetCapacityKW(ctx, t.Tenant, t.Scope)
	if err != nil {
		return nil, fmt.Errorf("optimization: get capacity: %w", err)
	}

	var total float64
	for _, kw := range capacities {
		total += kw
	}
	if total <= 0 {
		return nil, fmt.Errorf("optimization: aggregate target scope %v has zero total capacity", t.Scope)
	}

	specs := make([]model.CommandSpec, 0, len(t.Scope))
	for _, id := range t.Scope {
		kw, ok := capacities[id]
		if !ok || kw <= 0 {
			// Unknown or zero-rated capacity: excluded from the split
			// entirely, not given a zero-value command — a CU that
			// cannot contribute should not receive a no-op command that
			// looks like a decision was made about it.
			continue
		}
		share := t.DeltaValue * (kw / total)
		specs = append(specs, model.CommandSpec{
			CUCode:   id,
			PointKey: t.Metric,
			Value:    model.FloatCommandValue(share),
		})
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("optimization: aggregate target scope %v has no resources with known capacity", t.Scope)
	}
	return specs, nil
}
