package policy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestNewPolicy_SOCThreshold(t *testing.T) {
	params := validPolicyParams()
	p, err := NewPolicy(params)
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	if p.Version != 1 || !p.Enabled || p.Kind != KindSOCThreshold {
		t.Fatalf("policy = %+v", p)
	}
	params.SOC.MinSOC = 1
	if p.SOC.MinSOC != 20 {
		t.Fatal("constructor must copy the SOC spec")
	}
}

func TestNewPolicy_Rejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NewPolicyParams)
		want   string
	}{
		{name: "missing id", mutate: func(p *NewPolicyParams) { p.ID = " " }, want: "id is required"},
		{name: "missing tenant", mutate: func(p *NewPolicyParams) { p.TenantID = "" }, want: "tenant_id is required"},
		{name: "missing name", mutate: func(p *NewPolicyParams) { p.Name = "" }, want: "name is required"},
		{name: "portfolio scope", mutate: func(p *NewPolicyParams) { p.Scope.Type = "portfolio" }, want: "scope type"},
		{name: "point scope", mutate: func(p *NewPolicyParams) { p.Scope.Type = "point" }, want: "scope type"},
		{name: "missing scope id", mutate: func(p *NewPolicyParams) { p.Scope.ID = "" }, want: "scope id is required"},
		{name: "negative cooldown", mutate: func(p *NewPolicyParams) { p.Cooldown = -time.Second }, want: "cooldown"},
		{name: "unknown kind", mutate: func(p *NewPolicyParams) { p.Kind = "market" }, want: "unknown kind"},
		{name: "missing soc spec", mutate: func(p *NewPolicyParams) { p.SOC = nil }, want: "soc threshold spec"},
		{name: "soc out of range", mutate: func(p *NewPolicyParams) { p.SOC.MaxSOC = 101 }, want: "max_soc"},
		{name: "min not below max", mutate: func(p *NewPolicyParams) { p.SOC.MinSOC = 90; p.SOC.MaxSOC = 90 }, want: "min_soc"},
		{name: "zero charge power", mutate: func(p *NewPolicyParams) { p.SOC.ChargePowerKW = 0 }, want: "charge_power_kw"},
		{name: "negative discharge power", mutate: func(p *NewPolicyParams) { p.SOC.DischargePowerKW = -10 }, want: "discharge_power_kw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := validPolicyParams()
			tt.mutate(&params)
			_, err := NewPolicy(params)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestNewPolicy_ZeroCooldownAndDisabled(t *testing.T) {
	params := validPolicyParams()
	params.Enabled = false
	params.Cooldown = 0
	params.Scope = TargetScope{Type: port.ScopeCU, ID: "cu-1"}
	if _, err := NewPolicy(params); err != nil {
		t.Fatal(err)
	}
}

func TestSOCScopeQuery(t *testing.T) {
	p, err := NewPolicy(validPolicyParams())
	if err != nil {
		t.Fatal(err)
	}
	query, err := SOCScopeQuery(p)
	if err != nil {
		t.Fatal(err)
	}
	if query.TenantID != p.TenantID || query.ScopeType != port.ScopeAsset || query.ScopeID != "asset-1" {
		t.Fatalf("query scope = %+v", query)
	}
	if len(query.RequiredCapabilityIDs) != 1 || query.RequiredCapabilityIDs[0] != "energy.storage.v1" {
		t.Fatalf("capabilities = %v", query.RequiredCapabilityIDs)
	}
	if len(query.RequiredMetrics) != 2 {
		t.Fatalf("metrics = %+v", query.RequiredMetrics)
	}
	if query.RequiredMetrics[0].MetricID != "energy_storage.state_of_charge.v1" || query.RequiredMetrics[0].Access != port.MetricAccessRead {
		t.Fatalf("soc metric = %+v", query.RequiredMetrics[0])
	}
	if query.RequiredMetrics[1].MetricID != "electrical.active_power_setpoint.v1" || query.RequiredMetrics[1].Access != port.MetricAccessWrite {
		t.Fatalf("setpoint metric = %+v", query.RequiredMetrics[1])
	}
}

func TestDirectionAndCooldownKey(t *testing.T) {
	if err := DirectionCharge.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := Direction("idle").Validate(); err == nil {
		t.Fatal("unknown direction should fail")
	}
	key := CooldownKey{TenantID: "tenant-1", PolicyID: "policy-1", Direction: DirectionDischarge}
	if err := key.Validate(); err != nil {
		t.Fatal(err)
	}
	key.PolicyID = ""
	if err := key.Validate(); err == nil {
		t.Fatal("missing policy id should fail")
	}
}

type fakeCooldown struct{}

func (fakeCooldown) CoolingDown(context.Context, CooldownKey, time.Time) (bool, error) {
	return false, nil
}
func (fakeCooldown) MarkTriggered(context.Context, CooldownKey, time.Time) error { return nil }

var _ CooldownStore = fakeCooldown{}

func validPolicyParams() NewPolicyParams {
	return NewPolicyParams{
		ID:       "policy-1",
		TenantID: "tenant-1",
		Name:     "asset soc",
		Kind:     KindSOCThreshold,
		Scope:    TargetScope{Type: port.ScopeAsset, ID: "asset-1"},
		Enabled:  true,
		Cooldown: 2 * time.Minute,
		SOC: &SOCThresholdSpec{
			MinSOC:           20,
			MaxSOC:           90,
			ChargePowerKW:    100,
			DischargePowerKW: 80,
		},
	}
}
