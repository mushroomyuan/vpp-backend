package context

import (
	stdctx "context"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// ScopeResolver expands a target scope into members, exclusions, and a resource revision.
// CachingScopeResolver keeps a last-known-good copy. Enable and update call ResourcePort
// directly, so a failed catalog read cannot be served from that cache.
type ScopeResolver interface {
	Resolve(ctx stdctx.Context, query port.ScopeQuery) (port.ResolvedScope, error)
}

// StateCollector loads canonical metric samples for the CUs in one resolved scope.
// The SOC collector skips the whole policy when a participating sample is missing, stale, or not good.
// CollectBatch is one GetSnapshots for every scope of a tenant.
type StateCollector interface {
	Collect(
		ctx stdctx.Context,
		tenantID string,
		resolved port.ResolvedScope,
		metricIDs []contracts.MetricID,
		staleAge time.Duration,
		asOf time.Time,
	) (ScopeState, error)
	CollectBatch(
		ctx stdctx.Context,
		tenantID string,
		scopes []port.ResolvedScope,
		metricIDs []contracts.MetricID,
		staleAge time.Duration,
		asOf time.Time,
	) ([]CollectedScope, error)
}

// CollectedScope is one scope inside a tenant batch.
// Err is ErrUnusableState when that scope must be skipped. A transport failure is returned by CollectBatch itself.
type CollectedScope struct {
	State ScopeState
	Err   error
}
