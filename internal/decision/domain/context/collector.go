package context

import (
	stdctx "context"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// SnapshotCollector loads scope state through TelemetryPort.GetSnapshots.
// One bad participating CU skips that scope. A transport error fails the batch.
type SnapshotCollector struct {
	telemetry port.TelemetryPort
}

// NewSnapshotCollector requires a telemetry port.
func NewSnapshotCollector(telemetry port.TelemetryPort) *SnapshotCollector {
	if telemetry == nil {
		panic("NewSnapshotCollector: telemetry port is required")
	}
	return &SnapshotCollector{telemetry: telemetry}
}

var _ StateCollector = (*SnapshotCollector)(nil)

// Collect reads one scope. It is CollectBatch of a single scope.
func (c *SnapshotCollector) Collect(
	ctx stdctx.Context,
	tenantID string,
	resolved port.ResolvedScope,
	metricIDs []contracts.MetricID,
	staleAge time.Duration,
	asOf time.Time,
) (ScopeState, error) {
	batch, err := c.CollectBatch(ctx, tenantID, []port.ResolvedScope{resolved}, metricIDs, staleAge, asOf)
	if err != nil {
		return ScopeState{}, err
	}
	return batch[0].State, batch[0].Err
}

// CollectBatch issues one GetSnapshots for every CU that participates in the scopes.
// Scopes with no members succeed with an empty good state and do not add CU codes.
func (c *SnapshotCollector) CollectBatch(
	ctx stdctx.Context,
	tenantID string,
	scopes []port.ResolvedScope,
	metricIDs []contracts.MetricID,
	staleAge time.Duration,
	asOf time.Time,
) ([]CollectedScope, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("state collector: tenant_id is required")
	}
	if staleAge < 0 {
		return nil, fmt.Errorf("state collector: stale age must not be negative")
	}
	if asOf.IsZero() {
		return nil, fmt.Errorf("state collector: as_of is required")
	}
	if len(metricIDs) == 0 {
		return nil, fmt.Errorf("state collector: at least one metric id is required")
	}
	for _, id := range metricIDs {
		if _, ok := contracts.LookupMetric(id); !ok {
			return nil, fmt.Errorf("state collector: unknown metric id %q", id)
		}
	}

	out := make([]CollectedScope, len(scopes))
	var codes []string
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		for _, member := range scope.Members {
			if member.CUID == "" {
				continue
			}
			if _, ok := seen[member.CUID]; ok {
				continue
			}
			seen[member.CUID] = struct{}{}
			codes = append(codes, member.CUID)
		}
	}

	snapshots := map[string]port.CUSnapshot{}
	if len(codes) > 0 {
		loaded, err := c.telemetry.GetSnapshots(ctx, port.SnapshotQuery{
			TenantID:  tenantID,
			CUCodes:   codes,
			MetricIDs: append([]contracts.MetricID(nil), metricIDs...),
			StaleAge:  staleAge,
		})
		if err != nil {
			return nil, fmt.Errorf("state collector: get snapshots: %w", err)
		}
		for _, snap := range loaded {
			if _, dup := snapshots[snap.CUCode]; dup {
				return nil, fmt.Errorf("state collector: duplicate snapshot for cu %q", snap.CUCode)
			}
			snapshots[snap.CUCode] = snap
		}
	}

	for i, scope := range scopes {
		state, err := stateForScope(scope, snapshots, metricIDs, staleAge, asOf)
		out[i] = CollectedScope{State: state, Err: err}
	}
	return out, nil
}

func stateForScope(
	scope port.ResolvedScope,
	snapshots map[string]port.CUSnapshot,
	metricIDs []contracts.MetricID,
	staleAge time.Duration,
	asOf time.Time,
) (ScopeState, error) {
	if len(scope.Members) == 0 {
		return ScopeState{Quality: port.QualityGood}, nil
	}
	units := make([]CUState, 0, len(scope.Members))
	seen := map[string]struct{}{}
	for _, member := range scope.Members {
		cu := member.CUID
		if cu == "" {
			return ScopeState{}, unusable("resolved member cu id is required")
		}
		if _, dup := seen[cu]; dup {
			return ScopeState{}, unusable(fmt.Sprintf("duplicate member %s", cu))
		}
		seen[cu] = struct{}{}
		snap, ok := snapshots[cu]
		if !ok {
			return ScopeState{}, unusable(fmt.Sprintf("cu %s has no snapshot", cu))
		}
		if snap.Stale {
			return ScopeState{}, unusable(fmt.Sprintf("cu %s is stale", cu))
		}
		samples, err := requiredSamples(cu, snap, metricIDs, staleAge, asOf)
		if err != nil {
			return ScopeState{}, err
		}
		units = append(units, CUState{CUCode: cu, Metrics: samples})
	}
	return ScopeState{Quality: port.QualityGood, Units: units}, nil
}

func requiredSamples(
	cu string,
	snap port.CUSnapshot,
	metricIDs []contracts.MetricID,
	staleAge time.Duration,
	asOf time.Time,
) ([]port.MetricSample, error) {
	byID := make(map[contracts.MetricID]port.MetricSample, len(snap.Metrics))
	for _, sample := range snap.Metrics {
		if _, dup := byID[sample.MetricID]; dup {
			return nil, unusable(fmt.Sprintf("cu %s metric %s is duplicated", cu, sample.MetricID))
		}
		byID[sample.MetricID] = sample
	}
	out := make([]port.MetricSample, 0, len(metricIDs))
	for _, id := range metricIDs {
		sample, ok := byID[id]
		if !ok {
			return nil, unusable(fmt.Sprintf("cu %s metric %s is missing", cu, id))
		}
		if sample.Quality != port.QualityGood {
			return nil, unusable(fmt.Sprintf("cu %s metric %s quality is %s", cu, id, sample.Quality))
		}
		if sample.ObservedAt.IsZero() {
			return nil, unusable(fmt.Sprintf("cu %s metric %s observed_at is missing", cu, id))
		}
		if staleAge > 0 && asOf.Sub(sample.ObservedAt) > staleAge {
			return nil, unusable(fmt.Sprintf("cu %s metric %s is stale", cu, id))
		}
		out = append(out, sample)
	}
	return out, nil
}

func unusable(reason string) error {
	return fmt.Errorf("state collector: %s: %w", reason, ErrUnusableState)
}
