// Package objective is the shared language between policy evaluation and planning.
// A policy produces an objective. Dispatch commands come from a plan.
package objective

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
)

// Kind identifies the typed objective body.
type Kind string

const (
	KindPower Kind = "power"
)

// SourceType names which path created an objective.
type SourceType string

const (
	SourcePolicy         SourceType = "policy"
	SourceManual         SourceType = "manual"
	SourceDemandResponse SourceType = "demand_response"
	SourceMarket         SourceType = "market"
	SourceExternal       SourceType = "external"
)

// TimeWindow is the interval an objective or plan applies to.
type TimeWindow struct {
	Start time.Time
	End   time.Time
}

// Validate requires a non-empty interval.
func (w TimeWindow) Validate() error {
	if w.Start.IsZero() || w.End.IsZero() {
		return fmt.Errorf("time window start and end are required")
	}
	if !w.End.After(w.Start) {
		return fmt.Errorf("time window end must be after start")
	}
	return nil
}

// Objective is the envelope every source must fill.
// PowerObjective is the first typed body. Further kinds add a type and a constructor.
// Envelope fields stay on the concrete value; Kind selects which body a caller reads.
type Objective interface {
	Kind() Kind
	Validate() error
}

// PowerObjective is a signed canonical power target for one scope.
// TargetPowerKW uses the platform sign for electrical.active_power_setpoint.v1:
// positive is discharge (export), the same sign the simulator applies to SOC.
type PowerObjective struct {
	ID             string
	TenantID       string
	Scope          policy.TargetScope
	MetricID       contracts.MetricID
	TargetPowerKW  float64
	Window         TimeWindow
	Priority       int
	Source         SourceType
	SourceID       string
	IdempotencyKey string
	PolicyID       string
	PolicyVersion  int64
}

// NewPowerObjectiveParams are the inputs for a power objective.
// PolicyID and PolicyVersion are required when Source is policy, and must be empty otherwise.
type NewPowerObjectiveParams struct {
	ID             string
	TenantID       string
	Scope          policy.TargetScope
	MetricID       contracts.MetricID
	TargetPowerKW  float64
	Window         TimeWindow
	Priority       int
	Source         SourceType
	SourceID       string
	IdempotencyKey string
	PolicyID       string
	PolicyVersion  int64
}

// NewPowerObjective trims identifiers and validates the canonical power target.
func NewPowerObjective(params NewPowerObjectiveParams) (*PowerObjective, error) {
	o := &PowerObjective{
		ID:             strings.TrimSpace(params.ID),
		TenantID:       strings.TrimSpace(params.TenantID),
		Scope:          policy.TargetScope{Type: params.Scope.Type, ID: strings.TrimSpace(params.Scope.ID)},
		MetricID:       params.MetricID,
		TargetPowerKW:  params.TargetPowerKW,
		Window:         params.Window,
		Priority:       params.Priority,
		Source:         params.Source,
		SourceID:       strings.TrimSpace(params.SourceID),
		IdempotencyKey: strings.TrimSpace(params.IdempotencyKey),
		PolicyID:       strings.TrimSpace(params.PolicyID),
		PolicyVersion:  params.PolicyVersion,
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return o, nil
}

// Kind returns the power discriminant.
func (o *PowerObjective) Kind() Kind {
	if o == nil {
		return ""
	}
	return KindPower
}

// Validate checks the envelope, the power metric, and the policy reference.
func (o *PowerObjective) Validate() error {
	if o == nil {
		return fmt.Errorf("objective: power objective is nil")
	}
	if o.ID == "" {
		return fmt.Errorf("objective: id is required")
	}
	if o.TenantID == "" {
		return fmt.Errorf("objective: tenant_id is required")
	}
	if err := o.Scope.Validate(); err != nil {
		return err
	}
	if err := o.Window.Validate(); err != nil {
		return fmt.Errorf("objective: %w", err)
	}
	if o.Priority < 0 {
		return fmt.Errorf("objective: priority must be zero or positive")
	}
	if err := o.Source.validate(); err != nil {
		return err
	}
	if o.SourceID == "" {
		return fmt.Errorf("objective: source_id is required")
	}
	if o.IdempotencyKey == "" {
		return fmt.Errorf("objective: idempotency_key is required")
	}
	if err := validatePowerMetric(o.MetricID); err != nil {
		return err
	}
	if math.IsNaN(o.TargetPowerKW) || math.IsInf(o.TargetPowerKW, 0) || o.TargetPowerKW == 0 {
		return fmt.Errorf("objective: target_power_kw must be finite and non-zero")
	}
	return o.validatePolicyRef()
}

func (s SourceType) validate() error {
	switch s {
	case SourcePolicy, SourceManual, SourceDemandResponse, SourceMarket, SourceExternal:
		return nil
	default:
		return fmt.Errorf("objective: unknown source %q", s)
	}
}

func (o *PowerObjective) validatePolicyRef() error {
	if o.Source == SourcePolicy {
		if o.PolicyID == "" {
			return fmt.Errorf("objective: policy_id is required for a policy source")
		}
		if o.PolicyVersion <= 0 {
			return fmt.Errorf("objective: policy_version must be positive for a policy source")
		}
		return nil
	}
	if o.PolicyID != "" || o.PolicyVersion != 0 {
		return fmt.Errorf("objective: policy reference is only set for a policy source")
	}
	return nil
}

func validatePowerMetric(id contracts.MetricID) error {
	desc, ok := contracts.LookupMetric(id)
	if !ok {
		return fmt.Errorf("objective: unknown metric id %q", id)
	}
	if desc.ValueKind != contracts.ValueKindFloat64 || !desc.Writable || desc.CanonicalUnit != "kW" {
		return fmt.Errorf("objective: metric %q is not a writable kW power setpoint", id)
	}
	return nil
}

var _ Objective = (*PowerObjective)(nil)
