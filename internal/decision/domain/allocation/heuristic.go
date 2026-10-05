package allocation

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

const powerEPS = 1e-6

// Result is a feasible, partial, or rejected split of one power objective.
// Commands use electrical.active_power_setpoint.v1. IDs are left empty for the planner.
type Result struct {
	Feasibility  plan.Feasibility
	UnmetPowerKW float64
	Commands     []plan.PlannedCommand
}

// Allocator splits a scope power target across the CUs in a decision context.
type Allocator interface {
	Allocate(ctx context.Context, obj *objective.PowerObjective, dc dctx.DecisionContext) (Result, error)
}

// HeuristicAllocator weights each CU by SOC headroom and caps it by the
// direction power limit and the setpoint safety envelope.
// Positive target power discharges. Negative target power charges.
// Usable power below the target is partially feasible. Zero usable power rejects the plan.
type HeuristicAllocator struct{}

// NewHeuristicAllocator returns the SOC allocator. It does not call Solver.
func NewHeuristicAllocator() *HeuristicAllocator {
	return &HeuristicAllocator{}
}

var _ Allocator = (*HeuristicAllocator)(nil)

// Allocate assigns target power across members. It does not read Forecast.
func (h *HeuristicAllocator) Allocate(_ context.Context, obj *objective.PowerObjective, dc dctx.DecisionContext) (Result, error) {
	if h == nil {
		return Result{}, fmt.Errorf("heuristic: allocator is nil")
	}
	if obj == nil {
		return Result{}, fmt.Errorf("heuristic: objective is required")
	}
	if err := obj.Validate(); err != nil {
		return Result{}, err
	}
	if err := dc.Validate(); err != nil {
		return Result{}, err
	}
	if obj.TenantID != dc.TenantID || obj.Scope != dc.Scope {
		return Result{}, fmt.Errorf("heuristic: objective scope does not match the context")
	}
	if obj.MetricID != contracts.MetricElectricalActivePowerSetpoint {
		return Result{}, fmt.Errorf("heuristic: metric %s is not the active power setpoint", obj.MetricID)
	}

	discharge := obj.TargetPowerKW > 0
	target := math.Abs(obj.TargetPowerKW)
	socByCU := map[string]float64{}
	for _, unit := range dc.State.Units {
		for _, sample := range unit.Metrics {
			if sample.MetricID == contracts.MetricEnergyStorageStateOfCharge {
				socByCU[unit.CUCode] = sample.Value
			}
		}
	}

	type candidate struct {
		cu       port.ResolvedCU
		spec     contracts.EnergyStorageSpec
		binding  port.ResolvedBinding
		cap      float64
		weight   float64
		assigned float64
	}
	items := make([]candidate, 0, len(dc.Resolved.Members))
	for _, member := range dc.Resolved.Members {
		spec, ok := dctx.EnergyStorageSpec(member)
		if !ok {
			return Result{}, fmt.Errorf("heuristic: cu %s has no energy.storage.v1 spec", member.CUID)
		}
		binding, ok := dctx.SetpointBinding(member)
		if !ok {
			return Result{}, fmt.Errorf("heuristic: cu %s has no active power setpoint binding", member.CUID)
		}
		soc, ok := socByCU[member.CUID]
		if !ok {
			return Result{}, fmt.Errorf("heuristic: cu %s has no state of charge", member.CUID)
		}
		safety := recordedSafety(binding.Safety)
		items = append(items, candidate{
			cu:      member,
			spec:    spec,
			binding: binding,
			cap:     directionCap(spec, safety, discharge),
			weight:  headroomKWh(soc, spec.UsableEnergyKWh, discharge),
		})
	}

	caps := make([]float64, len(items))
	weights := make([]float64, len(items))
	for i := range items {
		caps[i] = items[i].cap
		weights[i] = items[i].weight
	}
	assigned := distribute(target, caps, weights)
	for i := range items {
		items[i].assigned = assigned[i]
	}

	commands := make([]plan.PlannedCommand, 0, len(items))
	var delivered float64
	for _, item := range items {
		if item.assigned <= powerEPS {
			continue
		}
		signed := item.assigned
		if !discharge {
			signed = -item.assigned
		}
		safety := recordedSafety(item.binding.Safety)
		value, ok := clampSetpoint(signed, safety)
		if !ok || math.Abs(value) <= powerEPS {
			continue
		}
		delivered += math.Abs(value)
		commands = append(commands, plan.PlannedCommand{
			CUCode:          item.cu.CUID,
			MetricID:        contracts.MetricElectricalActivePowerSetpoint,
			Value:           model.FloatCommandValue(value),
			BindingRevision: item.binding.Revision,
			Safety:          safety,
		})
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].CUCode < commands[j].CUCode })

	unmet := target - delivered
	if unmet < powerEPS {
		unmet = 0
	}
	if delivered <= powerEPS {
		return Result{
			Feasibility:  plan.FeasibilityInfeasible,
			UnmetPowerKW: target,
		}, nil
	}
	if unmet > 0 {
		return Result{
			Feasibility:  plan.FeasibilityPartiallyFeasible,
			UnmetPowerKW: unmet,
			Commands:     commands,
		}, nil
	}
	return Result{
		Feasibility: plan.FeasibilityFeasible,
		Commands:    commands,
	}, nil
}

func directionCap(spec contracts.EnergyStorageSpec, safety *port.SafetyConstraint, discharge bool) float64 {
	cap := spec.MaxChargePowerKW
	if discharge {
		cap = spec.MaxDischargePowerKW
	}
	if safety == nil {
		return nonNegative(cap)
	}
	if discharge {
		if safety.MaxValue != nil && *safety.MaxValue < cap {
			cap = *safety.MaxValue
		}
		if safety.MinValue != nil && cap < *safety.MinValue {
			return 0
		}
		return nonNegative(cap)
	}
	if safety.MinValue != nil {
		if *safety.MinValue >= 0 {
			return 0
		}
		mag := -*safety.MinValue
		if mag < cap {
			cap = mag
		}
	}
	if safety.MaxValue != nil && *safety.MaxValue <= 0 && safety.MinValue != nil && *safety.MinValue > *safety.MaxValue {
		return 0
	}
	return nonNegative(cap)
}

func headroomKWh(soc, usable float64, discharge bool) float64 {
	if usable <= 0 || math.IsNaN(soc) || math.IsInf(soc, 0) {
		return 0
	}
	if soc < 0 {
		soc = 0
	}
	if soc > 100 {
		soc = 100
	}
	if discharge {
		return usable * soc / 100
	}
	return usable * (100 - soc) / 100
}

func recordedSafety(in *port.SafetyConstraint) *port.SafetyConstraint {
	if in == nil || in.Version <= 0 {
		return nil
	}
	out := *in
	out.MinValue = cloneFloat(in.MinValue)
	out.MaxValue = cloneFloat(in.MaxValue)
	out.MaxChangePerSecond = cloneFloat(in.MaxChangePerSecond)
	return &out
}

func clampSetpoint(signed float64, safety *port.SafetyConstraint) (float64, bool) {
	if safety == nil {
		return signed, signed != 0
	}
	v := signed
	if safety.MinValue != nil && v < *safety.MinValue {
		v = *safety.MinValue
	}
	if safety.MaxValue != nil && v > *safety.MaxValue {
		v = *safety.MaxValue
	}
	if signed < 0 && v >= 0 {
		return 0, false
	}
	if signed > 0 && v <= 0 {
		return 0, false
	}
	if math.Abs(v) > math.Abs(signed)+powerEPS {
		return 0, false
	}
	return v, true
}

func distribute(target float64, caps, weights []float64) []float64 {
	n := len(caps)
	assigned := make([]float64, n)
	active := make([]bool, n)
	nActive := 0
	for i := range caps {
		if caps[i] > powerEPS && weights[i] > powerEPS {
			active[i] = true
			nActive++
		}
	}
	remaining := target
	for nActive > 0 && remaining > powerEPS {
		var weightSum float64
		for i := range active {
			if active[i] {
				weightSum += weights[i]
			}
		}
		if weightSum <= powerEPS {
			break
		}
		type proposal struct {
			index int
			share float64
			room  float64
		}
		proposals := make([]proposal, 0, nActive)
		for i := range active {
			if !active[i] {
				continue
			}
			proposals = append(proposals, proposal{
				index: i,
				share: remaining * weights[i] / weightSum,
				room:  caps[i] - assigned[i],
			})
		}
		var saturated []proposal
		for _, item := range proposals {
			if item.share > item.room+powerEPS {
				saturated = append(saturated, item)
			}
		}
		if len(saturated) == 0 {
			for _, item := range proposals {
				assigned[item.index] += item.share
			}
			remaining = 0
			break
		}
		for _, item := range saturated {
			room := item.room
			if room < 0 {
				room = 0
			}
			assigned[item.index] += room
			remaining -= room
			active[item.index] = false
			nActive--
		}
		if remaining < 0 {
			remaining = 0
		}
	}
	return assigned
}

func nonNegative(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}

func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
