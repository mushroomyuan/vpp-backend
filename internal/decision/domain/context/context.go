// Package context is the snapshot a planner reads.
// Scope expansion and telemetry reads stay behind ScopeResolver and StateCollector.
package context

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// CUState is the metric sample set for one CU in the resolved scope.
// CUCode is the resource CU id. Telemetry and Dispatch use the same identifier.
type CUState struct {
	CUCode  string
	Metrics []port.MetricSample
}

// ScopeState is the collected runtime view of a resolved scope.
// Quality is the collector's verdict for the whole scope.
type ScopeState struct {
	Units   []CUState
	Quality port.Quality
}

// DecisionContext is the resolved catalog plus the collected runtime state.
// ResourceRevision is the revision on Resolved.
type DecisionContext struct {
	TenantID    string
	Scope       policy.TargetScope
	Resolved    port.ResolvedScope
	State       ScopeState
	CollectedAt time.Time
}

// NewDecisionContext copies the scope snapshot and checks that it matches the target.
func NewDecisionContext(
	tenantID string,
	scope policy.TargetScope,
	resolved port.ResolvedScope,
	state ScopeState,
	collectedAt time.Time,
) (DecisionContext, error) {
	dc := DecisionContext{
		TenantID:    strings.TrimSpace(tenantID),
		Scope:       policy.TargetScope{Type: scope.Type, ID: strings.TrimSpace(scope.ID)},
		Resolved:    cloneResolved(resolved),
		State:       cloneState(state),
		CollectedAt: collectedAt,
	}
	if err := dc.Validate(); err != nil {
		return DecisionContext{}, err
	}
	return dc, nil
}

// ResourceRevision returns the catalog revision captured in this context.
func (c DecisionContext) ResourceRevision() string {
	return c.Resolved.ResourceRevision
}

// Validate checks identity, precheck, and the shape of the collected state.
func (c DecisionContext) Validate() error {
	if c.TenantID == "" {
		return fmt.Errorf("context: tenant_id is required")
	}
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if c.Resolved.ScopeType != c.Scope.Type || c.Resolved.ScopeID != c.Scope.ID {
		return fmt.Errorf("context: resolved scope must match the target scope")
	}
	if !c.Resolved.PrecheckOK {
		return fmt.Errorf("context: resolved scope precheck failed")
	}
	if strings.TrimSpace(c.Resolved.ResourceRevision) == "" {
		return fmt.Errorf("context: resource_revision is required")
	}
	if c.CollectedAt.IsZero() {
		return fmt.Errorf("context: collected_at is required")
	}
	return c.State.validate(c.Resolved.Members)
}

func (s ScopeState) validate(members []port.ResolvedCU) error {
	if !knownQuality(s.Quality) {
		return fmt.Errorf("context: unknown quality %q", s.Quality)
	}
	membersByID := make(map[string]struct{}, len(members))
	for _, member := range members {
		id := strings.TrimSpace(member.CUID)
		if id == "" {
			return fmt.Errorf("context: resolved member cu id is required")
		}
		if _, dup := membersByID[id]; dup {
			return fmt.Errorf("context: duplicate resolved member %q", id)
		}
		membersByID[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(s.Units))
	for _, unit := range s.Units {
		cu := strings.TrimSpace(unit.CUCode)
		if cu == "" {
			return fmt.Errorf("context: state cu code is required")
		}
		if _, ok := membersByID[cu]; !ok {
			return fmt.Errorf("context: state cu %q is not a resolved member", cu)
		}
		if _, dup := seen[cu]; dup {
			return fmt.Errorf("context: duplicate state cu %q", cu)
		}
		seen[cu] = struct{}{}
		if len(unit.Metrics) == 0 {
			return fmt.Errorf("context: state cu %q has no metrics", cu)
		}
		for _, sample := range unit.Metrics {
			if err := validateSample(cu, sample, s.Quality); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSample(cu string, sample port.MetricSample, aggregate port.Quality) error {
	if _, ok := contracts.LookupMetric(sample.MetricID); !ok {
		return fmt.Errorf("context: unknown metric id %q", sample.MetricID)
	}
	if !knownQuality(sample.Quality) {
		return fmt.Errorf("context: cu %q metric %q has unknown quality %q", cu, sample.MetricID, sample.Quality)
	}
	if math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) {
		return fmt.Errorf("context: cu %q metric %q value must be finite", cu, sample.MetricID)
	}
	if aggregate == port.QualityGood && sample.Quality != port.QualityGood {
		return fmt.Errorf("context: cu %q metric %q quality must be good", cu, sample.MetricID)
	}
	if sample.Quality == port.QualityGood && sample.ObservedAt.IsZero() {
		return fmt.Errorf("context: cu %q metric %q observed_at is required", cu, sample.MetricID)
	}
	return nil
}

func knownQuality(q port.Quality) bool {
	switch q {
	case port.QualityUnspecified, port.QualityGood, port.QualityBad, port.QualityUncertain:
		return true
	default:
		return false
	}
}

func cloneResolved(in port.ResolvedScope) port.ResolvedScope {
	out := in
	out.ScopeID = strings.TrimSpace(in.ScopeID)
	out.ResourceRevision = strings.TrimSpace(in.ResourceRevision)
	out.Members = append([]port.ResolvedCU(nil), in.Members...)
	for i := range out.Members {
		out.Members[i].CUID = strings.TrimSpace(out.Members[i].CUID)
	}
	out.Exclusions = append([]port.ScopeExclusion(nil), in.Exclusions...)
	out.PrecheckFailures = append([]port.ScopePrecheckFailure(nil), in.PrecheckFailures...)
	return out
}

func cloneState(in ScopeState) ScopeState {
	out := ScopeState{Quality: in.Quality, Units: make([]CUState, len(in.Units))}
	for i, unit := range in.Units {
		out.Units[i] = CUState{
			CUCode:  strings.TrimSpace(unit.CUCode),
			Metrics: append([]port.MetricSample(nil), unit.Metrics...),
		}
	}
	return out
}
