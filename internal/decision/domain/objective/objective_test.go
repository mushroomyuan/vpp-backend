package objective

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestNewPowerObjective(t *testing.T) {
	obj, err := NewPowerObjective(validPowerParams())
	if err != nil {
		t.Fatalf("NewPowerObjective: %v", err)
	}
	if obj.Kind() != KindPower || obj.TargetPowerKW != -100 || obj.PolicyVersion != 3 {
		t.Fatalf("objective = %+v", obj)
	}
	var as Objective = obj
	if err := as.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNewPowerObjective_ManualSource(t *testing.T) {
	params := validPowerParams()
	params.Source = SourceManual
	params.SourceID = "operator-1"
	params.PolicyID = ""
	params.PolicyVersion = 0
	params.TargetPowerKW = 40
	if _, err := NewPowerObjective(params); err != nil {
		t.Fatal(err)
	}
}

func TestNewPowerObjective_Rejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NewPowerObjectiveParams)
		want   string
	}{
		{name: "missing idempotency", mutate: func(p *NewPowerObjectiveParams) { p.IdempotencyKey = "" }, want: "idempotency_key"},
		{name: "negative priority", mutate: func(p *NewPowerObjectiveParams) { p.Priority = -1 }, want: "priority"},
		{name: "zero power", mutate: func(p *NewPowerObjectiveParams) { p.TargetPowerKW = 0 }, want: "target_power_kw"},
		{name: "nan power", mutate: func(p *NewPowerObjectiveParams) { p.TargetPowerKW = math.NaN() }, want: "target_power_kw"},
		{name: "soc metric", mutate: func(p *NewPowerObjectiveParams) {
			p.MetricID = contracts.MetricEnergyStorageStateOfCharge
		}, want: "not a writable kW"},
		{name: "readable power", mutate: func(p *NewPowerObjectiveParams) {
			p.MetricID = contracts.MetricElectricalActivePower
		}, want: "not a writable kW"},
		{name: "unknown metric", mutate: func(p *NewPowerObjectiveParams) { p.MetricID = "electrical.made_up.v1" }, want: "unknown metric"},
		{name: "closed window", mutate: func(p *NewPowerObjectiveParams) { p.Window.End = p.Window.Start }, want: "time window"},
		{name: "unknown source", mutate: func(p *NewPowerObjectiveParams) { p.Source = "vendor" }, want: "unknown source"},
		{name: "policy source missing id", mutate: func(p *NewPowerObjectiveParams) { p.PolicyID = "" }, want: "policy_id"},
		{name: "manual source keeps policy", mutate: func(p *NewPowerObjectiveParams) {
			p.Source = SourceDemandResponse
			p.PolicyVersion = 1
		}, want: "policy reference"},
		{name: "cu scope missing id", mutate: func(p *NewPowerObjectiveParams) {
			p.Scope = policy.TargetScope{Type: port.ScopeCU, ID: " "}
		}, want: "scope id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := validPowerParams()
			tt.mutate(&params)
			_, err := NewPowerObjective(params)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestPowerObjective_Nil(t *testing.T) {
	var obj *PowerObjective
	if err := obj.Validate(); err == nil {
		t.Fatal("nil objective should fail")
	}
	if obj.Kind() != "" {
		t.Fatal("nil kind should be empty")
	}
}

func validPowerParams() NewPowerObjectiveParams {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	return NewPowerObjectiveParams{
		ID:             "obj-1",
		TenantID:       "tenant-1",
		Scope:          policy.TargetScope{Type: port.ScopeAsset, ID: "asset-1"},
		MetricID:       contracts.MetricElectricalActivePowerSetpoint,
		TargetPowerKW:  -100,
		Window:         TimeWindow{Start: start, End: start.Add(time.Hour)},
		Priority:       0,
		Source:         SourcePolicy,
		SourceID:       "policy-1",
		IdempotencyKey: "policy-1:charge:1",
		PolicyID:       "policy-1",
		PolicyVersion:  3,
	}
}
