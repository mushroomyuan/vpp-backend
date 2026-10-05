package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/memory"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestSOCEvaluator_NoTriggerWithinBand(t *testing.T) {
	at := atTime()
	p := socPolicy(policy.TargetScope{Type: port.ScopeAsset, ID: "asset-1"}, time.Minute)
	dc := mustContext(t, p.Scope, []cuFixture{
		{id: "cu-a", soc: 10, energy: 100},
		{id: "cu-b", soc: 90, energy: 100},
	}, at)
	outcome, err := newEval(t).Evaluate(context.Background(), Input{Policy: p, DC: dc, Now: at, Window: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Objective != nil || outcome.Reason != ReasonInBand {
		t.Fatalf("outcome = %+v, weighted SOC 50 must stay inside 20..90", outcome)
	}
}

func TestSOCEvaluator_CUUsesItsOwnSOCAndAssetUsesEnergyWeights(t *testing.T) {
	at := atTime()
	eval := newEval(t)

	cuPolicy := socPolicy(policy.TargetScope{Type: port.ScopeCU, ID: "cu-1"}, time.Minute)
	cuPolicy.SOC.ChargePowerKW = 50
	cuDC := mustContext(t, cuPolicy.Scope, []cuFixture{{id: "cu-1", soc: 15, energy: 100}}, at)
	fired, err := eval.Evaluate(context.Background(), Input{Policy: cuPolicy, DC: cuDC, Now: at, Window: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if fired.Objective == nil || fired.Objective.TargetPowerKW != -50 || fired.Direction != policy.DirectionCharge {
		t.Fatalf("cu charge = %+v", fired.Objective)
	}

	asset := socPolicy(policy.TargetScope{Type: port.ScopeAsset, ID: "asset-1"}, time.Minute)
	asset.SOC.DischargePowerKW = 80
	assetDC := mustContext(t, asset.Scope, []cuFixture{
		{id: "cu-a", soc: 80, energy: 100},
		{id: "cu-b", soc: 100, energy: 300},
	}, at)
	// (80*100 + 100*300) / 400 = 95, above max 90.
	fired, err = eval.Evaluate(context.Background(), Input{Policy: asset, DC: assetDC, Now: at, Window: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if fired.Objective == nil || fired.Objective.TargetPowerKW != 80 || fired.Objective.Scope != asset.Scope {
		t.Fatalf("asset discharge = %+v", fired.Objective)
	}
}

func TestSOCEvaluator_CooldownDoesNotSuppressOppositeDirection(t *testing.T) {
	at := atTime()
	store := memory.NewCooldownStore()
	eval := NewSOCEvaluator(store, time.Minute, seqIDs())
	p := socPolicy(policy.TargetScope{Type: port.ScopeCU, ID: "cu-1"}, time.Minute)
	p.SOC.ChargePowerKW = 50
	p.SOC.DischargePowerKW = 40
	low := mustContext(t, p.Scope, []cuFixture{{id: "cu-1", soc: 5, energy: 200}}, at)
	first, err := eval.Evaluate(context.Background(), Input{Policy: p, DC: low, Now: at, Window: time.Minute})
	if err != nil || first.Reason != ReasonFired || first.Objective.TargetPowerKW != -50 {
		t.Fatalf("charge = %+v err %v", first, err)
	}
	if err := store.MarkTriggered(context.Background(), policy.CooldownKey{
		TenantID: p.TenantID, PolicyID: p.ID, Direction: policy.DirectionCharge,
	}, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	high := mustContext(t, p.Scope, []cuFixture{{id: "cu-1", soc: 95, energy: 200}}, at)
	second, err := eval.Evaluate(context.Background(), Input{Policy: p, DC: high, Now: at.Add(10 * time.Second), Window: time.Minute})
	if err != nil || second.Reason != ReasonFired || second.Objective.TargetPowerKW != 40 {
		t.Fatalf("discharge = %+v err %v", second, err)
	}

	third, err := eval.Evaluate(context.Background(), Input{Policy: p, DC: low, Now: at.Add(15 * time.Second), Window: time.Minute})
	if err != nil || third.Reason != ReasonCooldown || third.Objective != nil {
		t.Fatalf("charge still cooling = %+v err %v", third, err)
	}
}

func newEval(t *testing.T) *SOCEvaluator {
	t.Helper()
	return NewSOCEvaluator(memory.NewCooldownStore(), time.Minute, seqIDs())
}

func socPolicy(scope policy.TargetScope, cooldown time.Duration) *policy.Policy {
	p, err := policy.NewPolicy(policy.NewPolicyParams{
		ID:       "policy-1",
		TenantID: "tenant-1",
		Name:     "soc",
		Kind:     policy.KindSOCThreshold,
		Scope:    scope,
		Enabled:  true,
		Cooldown: cooldown,
		SOC: &policy.SOCThresholdSpec{
			MinSOC: 20, MaxSOC: 90, ChargePowerKW: 100, DischargePowerKW: 80,
		},
	})
	if err != nil {
		panic(err)
	}
	return p
}

type cuFixture struct {
	id     string
	soc    float64
	energy float64
}

func mustContext(t *testing.T, scope policy.TargetScope, cus []cuFixture, at time.Time) dctx.DecisionContext {
	t.Helper()
	members := make([]port.ResolvedCU, len(cus))
	units := make([]dctx.CUState, len(cus))
	for i, cu := range cus {
		members[i] = port.ResolvedCU{
			CUID:      cu.id,
			AssetID:   "asset-1",
			Lifecycle: port.LifecycleActive,
			Capabilities: []port.ResolvedCapability{{
				CapabilityID:  contracts.CapabilityEnergyStorage,
				SchemaVersion: 1,
				Enabled:       true,
				Spec: map[string]any{
					"usable_energy_kwh":      cu.energy,
					"max_charge_power_kw":    100.0,
					"max_discharge_power_kw": 100.0,
				},
			}},
			Bindings: []port.ResolvedBinding{{
				MetricID:   contracts.MetricElectricalActivePowerSetpoint,
				AccessMode: port.BindingAccessWrite,
				Enabled:    true,
				Revision:   3,
			}},
		}
		units[i] = dctx.CUState{
			CUCode: cu.id,
			Metrics: []port.MetricSample{{
				MetricID:   contracts.MetricEnergyStorageStateOfCharge,
				Value:      cu.soc,
				ObservedAt: at,
				Quality:    port.QualityGood,
			}},
		}
	}
	dc, err := dctx.NewDecisionContext("tenant-1", scope, port.ResolvedScope{
		ScopeType:        scope.Type,
		ScopeID:          scope.ID,
		ResourceRevision: "rev-1",
		PrecheckOK:       true,
		Members:          members,
	}, dctx.ScopeState{Quality: port.QualityGood, Units: units}, at)
	if err != nil {
		t.Fatal(err)
	}
	return dc
}

func seqIDs() func() string {
	n := 0
	return func() string {
		n++
		return "id-" + itoa(n)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func atTime() time.Time {
	return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
}
