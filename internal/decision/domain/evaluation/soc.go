// Package evaluation turns a policy and a decision context into an objective.
// It does not allocate commands or call Dispatch.
package evaluation

import (
	"context"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

const (
	ReasonFired             = "fired"
	ReasonInBand            = "in_band"
	ReasonCooldown          = "cooldown"
	ReasonEmpty             = "empty_scope"
	ReasonInvalidCapability = "invalid_capability"
	ReasonUnusableState     = "unusable_state"
	ReasonInvalidScope      = "invalid_scope"
)

// Outcome is the evaluator result. Objective is set only when Reason is fired.
// CoolingDown is checked here. The plan repository claims the cooldown when it saves a ready plan.
type Outcome struct {
	Objective *objective.PowerObjective
	Direction policy.Direction
	Reason    string
}

// SOCEvaluator compares state of charge with a policy threshold.
// A CU scope uses that CU. An asset or site uses the energy-weighted average.
type SOCEvaluator struct {
	cooldown        policy.CooldownStore
	defaultCooldown time.Duration
	newID           func() string
}

// NewSOCEvaluator requires a cooldown store, a positive default cooldown, and an id function.
func NewSOCEvaluator(cooldown policy.CooldownStore, defaultCooldown time.Duration, newID func() string) *SOCEvaluator {
	if cooldown == nil {
		panic("NewSOCEvaluator: cooldown store is required")
	}
	if defaultCooldown <= 0 {
		panic("NewSOCEvaluator: default cooldown must be positive")
	}
	if newID == nil {
		panic("NewSOCEvaluator: id function is required")
	}
	return &SOCEvaluator{cooldown: cooldown, defaultCooldown: defaultCooldown, newID: newID}
}

// Input is one enabled SOC policy plus the context collected for it.
type Input struct {
	Policy *policy.Policy
	DC     dctx.DecisionContext
	Now    time.Time
	Window time.Duration
}

// Evaluate returns a scope-level power objective when a threshold is crossed
// and that direction is not cooling down. The power sign follows
// electrical.active_power_setpoint.v1: positive discharges, negative charges.
func (e *SOCEvaluator) Evaluate(ctx context.Context, in Input) (Outcome, error) {
	if e == nil {
		return Outcome{}, fmt.Errorf("evaluation: evaluator is nil")
	}
	p := in.Policy
	if p == nil || p.Kind != policy.KindSOCThreshold || p.SOC == nil {
		return Outcome{}, fmt.Errorf("evaluation: soc policy is required")
	}
	if err := p.Validate(); err != nil {
		return Outcome{}, err
	}
	if in.DC.TenantID != p.TenantID || in.DC.Scope != p.Scope {
		return Outcome{}, fmt.Errorf("evaluation: context scope does not match the policy")
	}
	if in.Now.IsZero() || in.Window <= 0 {
		return Outcome{}, fmt.Errorf("evaluation: now and a positive window are required")
	}

	if len(in.DC.Resolved.Members) == 0 {
		return Outcome{Reason: ReasonEmpty}, nil
	}
	if p.Scope.Type == port.ScopeCU && len(in.DC.Resolved.Members) != 1 {
		return Outcome{Reason: ReasonInvalidScope}, nil
	}
	for _, member := range in.DC.Resolved.Members {
		if _, ok := dctx.EnergyStorageSpec(member); !ok {
			return Outcome{Reason: ReasonInvalidCapability}, nil
		}
		if _, ok := dctx.SetpointBinding(member); !ok {
			return Outcome{Reason: ReasonInvalidCapability}, nil
		}
	}

	soc, ok := aggregateSOC(p.Scope.Type, in.DC)
	if !ok {
		return Outcome{Reason: ReasonUnusableState}, nil
	}

	var (
		direction policy.Direction
		magnitude float64
	)
	switch {
	case soc <= p.SOC.MinSOC:
		direction = policy.DirectionCharge
		magnitude = p.SOC.ChargePowerKW
	case soc >= p.SOC.MaxSOC:
		direction = policy.DirectionDischarge
		magnitude = p.SOC.DischargePowerKW
	default:
		return Outcome{Reason: ReasonInBand}, nil
	}

	key := policy.CooldownKey{TenantID: p.TenantID, PolicyID: p.ID, Direction: direction}
	cooling, err := e.cooldown.CoolingDown(ctx, key, in.Now)
	if err != nil {
		return Outcome{}, err
	}
	if cooling {
		return Outcome{Direction: direction, Reason: ReasonCooldown}, nil
	}

	target := magnitude
	if direction == policy.DirectionCharge {
		target = -magnitude
	}
	obj, err := objective.NewPowerObjective(objective.NewPowerObjectiveParams{
		ID:            e.newID(),
		TenantID:      p.TenantID,
		Scope:         p.Scope,
		MetricID:      contracts.MetricElectricalActivePowerSetpoint,
		TargetPowerKW: target,
		Window: objective.TimeWindow{
			Start: in.Now,
			End:   in.Now.Add(in.Window),
		},
		Source:         objective.SourcePolicy,
		SourceID:       p.ID,
		IdempotencyKey: fmt.Sprintf("%s:%s:%d", p.ID, direction, in.Now.UnixNano()),
		PolicyID:       p.ID,
		PolicyVersion:  p.Version,
	})
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Objective: obj, Direction: direction, Reason: ReasonFired}, nil
}

// CooldownUntil is the exclusive end of the window for a policy that just fired.
func CooldownUntil(p *policy.Policy, defaultCooldown time.Duration, now time.Time) time.Time {
	d := p.Cooldown
	if d <= 0 {
		d = defaultCooldown
	}
	return now.Add(d)
}

func aggregateSOC(scopeType port.ScopeType, dc dctx.DecisionContext) (float64, bool) {
	samples := map[string]float64{}
	for _, unit := range dc.State.Units {
		value, ok := socSample(unit)
		if !ok {
			return 0, false
		}
		samples[unit.CUCode] = value
	}
	if scopeType == port.ScopeCU {
		value, ok := samples[dc.Resolved.Members[0].CUID]
		return value, ok
	}

	var weighted, energy float64
	for _, member := range dc.Resolved.Members {
		soc, ok := samples[member.CUID]
		if !ok {
			return 0, false
		}
		spec, ok := dctx.EnergyStorageSpec(member)
		if !ok || spec.UsableEnergyKWh <= 0 {
			return 0, false
		}
		weighted += soc * spec.UsableEnergyKWh
		energy += spec.UsableEnergyKWh
	}
	if energy <= 0 {
		return 0, false
	}
	return weighted / energy, true
}

func socSample(unit dctx.CUState) (float64, bool) {
	for _, sample := range unit.Metrics {
		if sample.MetricID == contracts.MetricEnergyStorageStateOfCharge && sample.Quality == port.QualityGood {
			return sample.Value, true
		}
	}
	return 0, false
}
