package context

import (
	stdctx "context"
	"strings"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestNewDecisionContext(t *testing.T) {
	dc, err := NewDecisionContext("tenant-1", targetScope(), resolvedScope(), goodState(), collectedAt())
	if err != nil {
		t.Fatalf("NewDecisionContext: %v", err)
	}
	if dc.ResourceRevision() != "rev-1" || dc.State.Quality != port.QualityGood {
		t.Fatalf("context = %+v", dc)
	}
}

func TestNewDecisionContext_EmptyGoodScope(t *testing.T) {
	resolved := resolvedScope()
	resolved.Members = nil
	state := ScopeState{Quality: port.QualityGood}
	if _, err := NewDecisionContext("tenant-1", targetScope(), resolved, state, collectedAt()); err != nil {
		t.Fatal(err)
	}
}

func TestNewDecisionContext_Rejects(t *testing.T) {
	tests := []struct {
		name     string
		tenant   string
		scope    policy.TargetScope
		resolved port.ResolvedScope
		state    ScopeState
		at       time.Time
		want     string
	}{
		{
			name: "missing tenant", tenant: " ", scope: targetScope(), resolved: resolvedScope(),
			state: goodState(), at: collectedAt(), want: "tenant_id",
		},
		{
			name: "scope mismatch", tenant: "tenant-1", scope: targetScope(),
			resolved: func() port.ResolvedScope { r := resolvedScope(); r.ScopeID = "asset-2"; return r }(),
			state:    goodState(), at: collectedAt(), want: "must match",
		},
		{
			name: "precheck failed", tenant: "tenant-1", scope: targetScope(),
			resolved: func() port.ResolvedScope { r := resolvedScope(); r.PrecheckOK = false; return r }(),
			state:    goodState(), at: collectedAt(), want: "precheck",
		},
		{
			name: "missing revision", tenant: "tenant-1", scope: targetScope(),
			resolved: func() port.ResolvedScope { r := resolvedScope(); r.ResourceRevision = ""; return r }(),
			state:    goodState(), at: collectedAt(), want: "resource_revision",
		},
		{
			name: "missing collected at", tenant: "tenant-1", scope: targetScope(), resolved: resolvedScope(),
			state: goodState(), want: "collected_at",
		},
		{
			name: "unknown quality", tenant: "tenant-1", scope: targetScope(), resolved: resolvedScope(),
			state: ScopeState{Quality: "stale"}, at: collectedAt(), want: "unknown quality",
		},
		{
			name: "state outside members", tenant: "tenant-1", scope: targetScope(), resolved: resolvedScope(),
			state: func() ScopeState {
				s := goodState()
				s.Units[0].CUCode = "cu-other"
				return s
			}(),
			at: collectedAt(), want: "not a resolved member",
		},
		{
			name: "good aggregate with bad sample", tenant: "tenant-1", scope: targetScope(), resolved: resolvedScope(),
			state: func() ScopeState {
				s := goodState()
				s.Units[0].Metrics[0].Quality = port.QualityBad
				return s
			}(),
			at: collectedAt(), want: "quality must be good",
		},
		{
			name: "good sample missing time", tenant: "tenant-1", scope: targetScope(), resolved: resolvedScope(),
			state: func() ScopeState {
				s := goodState()
				s.Units[0].Metrics[0].ObservedAt = time.Time{}
				return s
			}(),
			at: collectedAt(), want: "observed_at",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDecisionContext(tt.tenant, tt.scope, tt.resolved, tt.state, tt.at)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want substring %q", err, tt.want)
			}
		})
	}
}

type fakeResolver struct{}

func (fakeResolver) Resolve(stdctx.Context, port.ScopeQuery) (port.ResolvedScope, error) {
	return port.ResolvedScope{}, nil
}

type fakeCollector struct{}

func (fakeCollector) Collect(stdctx.Context, string, port.ResolvedScope, []contracts.MetricID, time.Duration, time.Time) (ScopeState, error) {
	return ScopeState{}, nil
}

func (fakeCollector) CollectBatch(stdctx.Context, string, []port.ResolvedScope, []contracts.MetricID, time.Duration, time.Time) ([]CollectedScope, error) {
	return nil, nil
}

var (
	_ ScopeResolver  = fakeResolver{}
	_ StateCollector = fakeCollector{}
)

func targetScope() policy.TargetScope {
	return policy.TargetScope{Type: port.ScopeAsset, ID: "asset-1"}
}

func resolvedScope() port.ResolvedScope {
	return port.ResolvedScope{
		ScopeType:        port.ScopeAsset,
		ScopeID:          "asset-1",
		ResourceRevision: "rev-1",
		PrecheckOK:       true,
		Members: []port.ResolvedCU{{
			CUID:      "cu-1",
			AssetID:   "asset-1",
			Lifecycle: port.LifecycleActive,
		}},
	}
}

func goodState() ScopeState {
	return ScopeState{
		Quality: port.QualityGood,
		Units: []CUState{{
			CUCode: "cu-1",
			Metrics: []port.MetricSample{{
				MetricID:   contracts.MetricEnergyStorageStateOfCharge,
				Value:      15,
				ObservedAt: collectedAt(),
				Quality:    port.QualityGood,
			}},
		}},
	}
}

func collectedAt() time.Time {
	return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
}
