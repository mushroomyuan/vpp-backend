// Package plan is the versioned result of planning.
// The planner fills a plan in memory. It can be executed only after a repository saves it.
package plan

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// Status is the lifecycle of a plan inside Decision.
type Status string

const (
	StatusReady     Status = "ready"
	StatusRejected  Status = "rejected"
	StatusStale     Status = "stale"
	StatusSubmitted Status = "submitted"
)

// Feasibility records whether the commands cover the objective.
type Feasibility string

const (
	FeasibilityFeasible          Feasibility = "feasible"
	FeasibilityPartiallyFeasible Feasibility = "partially_feasible"
	FeasibilityInfeasible        Feasibility = "infeasible"
)

// PlannedCommand is one CU setpoint, with the binding revision and safety envelope used to produce it.
type PlannedCommand struct {
	ID              string
	CUCode          string
	MetricID        contracts.MetricID
	Value           model.CommandValue
	BindingRevision int64
	Safety          *port.SafetyConstraint
	// DispatchTaskID is empty until the step is accepted by Dispatch.
	DispatchTaskID string
}

// PlanStep is one ordered execution point. Commands inside a step run in parallel.
type PlanStep struct {
	ID        string
	Ordinal   int
	ExecuteAt time.Time
	Status    Status
	Version   int64
	Commands  []PlannedCommand
}

// Plan is one objective turned into steps and commands.
// UnmetPowerKW is the absolute shortfall in canonical kW. It is zero when the plan is fully feasible.
type Plan struct {
	ID               string
	ObjectiveID      string
	TenantID         string
	PolicyID         string
	PolicyVersion    int64
	ResourceRevision string
	PlannerID        string
	PlannerVersion   string
	Status           Status
	Feasibility      Feasibility
	UnmetPowerKW     float64
	Window           objective.TimeWindow
	GeneratedAt      time.Time
	Steps            []PlanStep
}

// NewPlanParams are the inputs for a plan. Identifiers are assigned by the caller.
type NewPlanParams struct {
	ID               string
	ObjectiveID      string
	TenantID         string
	PolicyID         string
	PolicyVersion    int64
	ResourceRevision string
	PlannerID        string
	PlannerVersion   string
	Status           Status
	Feasibility      Feasibility
	UnmetPowerKW     float64
	Window           objective.TimeWindow
	GeneratedAt      time.Time
	Steps            []PlanStep
}

// NewPlan copies commands and validates the audit fields planners must fill in.
func NewPlan(params NewPlanParams) (*Plan, error) {
	p := &Plan{
		ID:               strings.TrimSpace(params.ID),
		ObjectiveID:      strings.TrimSpace(params.ObjectiveID),
		TenantID:         strings.TrimSpace(params.TenantID),
		PolicyID:         strings.TrimSpace(params.PolicyID),
		PolicyVersion:    params.PolicyVersion,
		ResourceRevision: strings.TrimSpace(params.ResourceRevision),
		PlannerID:        strings.TrimSpace(params.PlannerID),
		PlannerVersion:   strings.TrimSpace(params.PlannerVersion),
		Status:           params.Status,
		Feasibility:      params.Feasibility,
		UnmetPowerKW:     params.UnmetPowerKW,
		Window:           params.Window,
		GeneratedAt:      params.GeneratedAt,
		Steps:            cloneSteps(params.Steps),
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Validate checks status, feasibility, and the command envelope.
func (p *Plan) Validate() error {
	if p == nil {
		return fmt.Errorf("plan: plan is nil")
	}
	if p.ID == "" {
		return fmt.Errorf("plan: id is required")
	}
	if p.ObjectiveID == "" {
		return fmt.Errorf("plan: objective_id is required")
	}
	if p.TenantID == "" {
		return fmt.Errorf("plan: tenant_id is required")
	}
	if err := validatePolicyRef(p.PolicyID, p.PolicyVersion); err != nil {
		return err
	}
	if p.ResourceRevision == "" {
		return fmt.Errorf("plan: resource_revision is required")
	}
	if p.PlannerID == "" || p.PlannerVersion == "" {
		return fmt.Errorf("plan: planner id and version are required")
	}
	if err := p.Window.Validate(); err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	if p.GeneratedAt.IsZero() {
		return fmt.Errorf("plan: generated_at is required")
	}
	if math.IsNaN(p.UnmetPowerKW) || math.IsInf(p.UnmetPowerKW, 0) || p.UnmetPowerKW < 0 {
		return fmt.Errorf("plan: unmet_power_kw must be finite and non-negative")
	}
	if err := validateStatus(p.Status, p.Feasibility, p.UnmetPowerKW, len(p.Steps)); err != nil {
		return err
	}
	return validateSteps(p.Status, p.Steps)
}

func validatePolicyRef(policyID string, version int64) error {
	if policyID == "" {
		if version != 0 {
			return fmt.Errorf("plan: policy_version requires policy_id")
		}
		return nil
	}
	if version <= 0 {
		return fmt.Errorf("plan: policy_version must be positive")
	}
	return nil
}

func validateStatus(status Status, feasibility Feasibility, unmet float64, steps int) error {
	switch status {
	case StatusReady, StatusStale, StatusSubmitted:
		if feasibility != FeasibilityFeasible && feasibility != FeasibilityPartiallyFeasible {
			return fmt.Errorf("plan: status %q requires a feasible allocation", status)
		}
		if steps == 0 {
			return fmt.Errorf("plan: status %q requires at least one step", status)
		}
	case StatusRejected:
		if feasibility != FeasibilityInfeasible {
			return fmt.Errorf("plan: rejected status requires infeasible allocation")
		}
		if steps != 0 {
			return fmt.Errorf("plan: rejected status has no steps")
		}
	default:
		return fmt.Errorf("plan: unknown status %q", status)
	}
	switch feasibility {
	case FeasibilityFeasible:
		if unmet != 0 {
			return fmt.Errorf("plan: feasible allocation has unmet power")
		}
	case FeasibilityPartiallyFeasible, FeasibilityInfeasible:
		if unmet <= 0 {
			return fmt.Errorf("plan: %s allocation requires unmet power", feasibility)
		}
	default:
		return fmt.Errorf("plan: unknown feasibility %q", feasibility)
	}
	return nil
}

func validateSteps(status Status, steps []PlanStep) error {
	if status == StatusRejected {
		return nil
	}
	seenStep := make(map[string]struct{}, len(steps))
	seenCmd := make(map[string]struct{})
	seenTarget := make(map[string]struct{})
	for i, step := range steps {
		if step.Ordinal != i+1 {
			return fmt.Errorf("plan: step ordinals must be 1..n in order")
		}
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("plan: step id is required")
		}
		if _, dup := seenStep[step.ID]; dup {
			return fmt.Errorf("plan: duplicate step id %q", step.ID)
		}
		seenStep[step.ID] = struct{}{}
		if step.ExecuteAt.IsZero() {
			return fmt.Errorf("plan: step %q execute_at is required", step.ID)
		}
		if step.Version <= 0 {
			return fmt.Errorf("plan: step %q version must be positive", step.ID)
		}
		if step.Status != status {
			return fmt.Errorf("plan: step %q status must match the plan", step.ID)
		}
		if len(step.Commands) == 0 {
			return fmt.Errorf("plan: step %q requires at least one command", step.ID)
		}
		for _, cmd := range step.Commands {
			if err := validateCommand(cmd, seenCmd, seenTarget); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCommand(cmd PlannedCommand, seenCmd, seenTarget map[string]struct{}) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return fmt.Errorf("plan: command id is required")
	}
	if _, dup := seenCmd[cmd.ID]; dup {
		return fmt.Errorf("plan: duplicate command id %q", cmd.ID)
	}
	seenCmd[cmd.ID] = struct{}{}
	if strings.TrimSpace(cmd.CUCode) == "" {
		return fmt.Errorf("plan: command %q cu code is required", cmd.ID)
	}
	desc, ok := contracts.LookupMetric(cmd.MetricID)
	if !ok || desc.ValueKind != contracts.ValueKindFloat64 || !desc.Writable || desc.CanonicalUnit != "kW" {
		return fmt.Errorf("plan: command %q metric %q is not a writable kW setpoint", cmd.ID, cmd.MetricID)
	}
	if err := cmd.Value.Validate(); err != nil {
		return fmt.Errorf("plan: command %q: %w", cmd.ID, err)
	}
	if cmd.Value.FloatValue == nil || math.IsNaN(*cmd.Value.FloatValue) || math.IsInf(*cmd.Value.FloatValue, 0) {
		return fmt.Errorf("plan: command %q value must be a finite float", cmd.ID)
	}
	target := cmd.CUCode + "\x00" + string(cmd.MetricID)
	if _, dup := seenTarget[target]; dup {
		return fmt.Errorf("plan: duplicate command for cu %q metric %q", cmd.CUCode, cmd.MetricID)
	}
	seenTarget[target] = struct{}{}
	if cmd.BindingRevision <= 0 {
		return fmt.Errorf("plan: command %q binding_revision must be positive", cmd.ID)
	}
	return validateSafety(cmd.ID, cmd.Safety)
}

func validateSafety(commandID string, safety *port.SafetyConstraint) error {
	if safety == nil {
		return nil
	}
	for name, value := range map[string]*float64{
		"min_value":             safety.MinValue,
		"max_value":             safety.MaxValue,
		"max_change_per_second": safety.MaxChangePerSecond,
	} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return fmt.Errorf("plan: command %q safety %s must be finite", commandID, name)
		}
	}
	if safety.MinValue != nil && safety.MaxValue != nil && *safety.MinValue > *safety.MaxValue {
		return fmt.Errorf("plan: command %q safety min_value must not exceed max_value", commandID)
	}
	if safety.MaxChangePerSecond != nil && *safety.MaxChangePerSecond <= 0 {
		return fmt.Errorf("plan: command %q safety max_change_per_second must be positive", commandID)
	}
	if safety.Version <= 0 {
		return fmt.Errorf("plan: command %q safety version must be positive", commandID)
	}
	return nil
}

func cloneSteps(steps []PlanStep) []PlanStep {
	out := make([]PlanStep, len(steps))
	for i, step := range steps {
		out[i] = step
		out[i].ID = strings.TrimSpace(step.ID)
		out[i].Commands = make([]PlannedCommand, len(step.Commands))
		for j, cmd := range step.Commands {
			out[i].Commands[j] = cmd
			out[i].Commands[j].ID = strings.TrimSpace(cmd.ID)
			out[i].Commands[j].CUCode = strings.TrimSpace(cmd.CUCode)
			out[i].Commands[j].Value = cloneCommandValue(cmd.Value)
			out[i].Commands[j].Safety = cloneSafety(cmd.Safety)
		}
	}
	return out
}

func cloneCommandValue(v model.CommandValue) model.CommandValue {
	out := model.CommandValue{}
	if v.BoolValue != nil {
		b := *v.BoolValue
		out.BoolValue = &b
	}
	if v.IntValue != nil {
		n := *v.IntValue
		out.IntValue = &n
	}
	if v.FloatValue != nil {
		f := *v.FloatValue
		out.FloatValue = &f
	}
	if v.StringValue != nil {
		s := *v.StringValue
		out.StringValue = &s
	}
	return out
}

func cloneSafety(in *port.SafetyConstraint) *port.SafetyConstraint {
	if in == nil {
		return nil
	}
	out := *in
	out.MinValue = cloneFloat(in.MinValue)
	out.MaxValue = cloneFloat(in.MaxValue)
	out.MaxChangePerSecond = cloneFloat(in.MaxChangePerSecond)
	return &out
}

func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
