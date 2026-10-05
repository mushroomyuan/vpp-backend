// Package policy holds a decision policy and the scope it applies to.
// Policies describe when to act. They do not list CUs, metric bindings, or commands.
package policy

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// Kind identifies the typed spec stored with a policy.
type Kind string

const (
	// KindSOCThreshold compares state of charge with a low and a high threshold.
	KindSOCThreshold Kind = "soc_threshold"
)

// Direction is one side of a threshold policy.
// Cooldown is tracked per direction so a charge does not suppress a later discharge.
type Direction string

const (
	DirectionCharge    Direction = "charge"
	DirectionDischarge Direction = "discharge"
)

// TargetScope is the site, asset, or CU a policy or objective applies to.
// Point is not a scope: the metric contract decides which binding is written.
type TargetScope struct {
	Type port.ScopeType
	ID   string
}

// SOCThresholdSpec is the typed body of a SOC threshold policy.
// Powers are positive scope totals in canonical kW. The signed setpoint is
// chosen later, when a power objective is created.
type SOCThresholdSpec struct {
	MinSOC           float64
	MaxSOC           float64
	ChargePowerKW    float64
	DischargePowerKW float64
}

// Policy is one tenant-scoped decision policy.
// Cooldown zero means the caller supplies the process default. It does not disable cooldown.
type Policy struct {
	ID        string
	TenantID  string
	Name      string
	Kind      Kind
	Scope     TargetScope
	Enabled   bool
	Cooldown  time.Duration
	Version   int64
	SOC       *SOCThresholdSpec
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}

// NewPolicyParams are the inputs for a new policy. Version starts at 1.
type NewPolicyParams struct {
	ID       string
	TenantID string
	Name     string
	Kind     Kind
	Scope    TargetScope
	Enabled  bool
	Cooldown time.Duration
	SOC      *SOCThresholdSpec
}

// NewPolicy trims identifiers, copies the SOC spec, and validates the result.
func NewPolicy(params NewPolicyParams) (*Policy, error) {
	p := &Policy{
		ID:       strings.TrimSpace(params.ID),
		TenantID: strings.TrimSpace(params.TenantID),
		Name:     strings.TrimSpace(params.Name),
		Kind:     params.Kind,
		Scope: TargetScope{
			Type: params.Scope.Type,
			ID:   strings.TrimSpace(params.Scope.ID),
		},
		Enabled:  params.Enabled,
		Cooldown: params.Cooldown,
		Version:  1,
	}
	if params.SOC != nil {
		spec := *params.SOC
		p.SOC = &spec
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Clone returns a deep copy. Repositories return clones so callers cannot
// mutate the stored spec.
func (p *Policy) Clone() *Policy {
	if p == nil {
		return nil
	}
	out := *p
	if p.SOC != nil {
		spec := *p.SOC
		out.SOC = &spec
	}
	return &out
}

// Validate checks the policy envelope and the spec required by its kind.
func (p *Policy) Validate() error {
	if p == nil {
		return fmt.Errorf("policy: policy is nil")
	}
	if p.ID == "" {
		return fmt.Errorf("policy: id is required")
	}
	if p.TenantID == "" {
		return fmt.Errorf("policy: tenant_id is required")
	}
	if p.Name == "" {
		return fmt.Errorf("policy: name is required")
	}
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if p.Cooldown < 0 {
		return fmt.Errorf("policy: cooldown must be zero or positive")
	}
	if p.Version <= 0 {
		return fmt.Errorf("policy: version must be positive")
	}
	switch p.Kind {
	case KindSOCThreshold:
		return p.SOC.validate()
	default:
		return fmt.Errorf("policy: unknown kind %q", p.Kind)
	}
}

// Validate accepts only a site, asset, or CU scope with an id.
func (s TargetScope) Validate() error {
	switch s.Type {
	case port.ScopeSite, port.ScopeAsset, port.ScopeCU:
	default:
		return fmt.Errorf("policy: scope type %q is not site, asset, or cu", s.Type)
	}
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("policy: scope id is required")
	}
	return nil
}

// Validate accepts charge and discharge only.
func (d Direction) Validate() error {
	switch d {
	case DirectionCharge, DirectionDischarge:
		return nil
	default:
		return fmt.Errorf("policy: unknown direction %q", d)
	}
}

func (s *SOCThresholdSpec) validate() error {
	if s == nil {
		return fmt.Errorf("policy: soc threshold spec is required")
	}
	minBound, maxBound := socBounds()
	if err := inSOCRange("min_soc", s.MinSOC, minBound, maxBound); err != nil {
		return err
	}
	if err := inSOCRange("max_soc", s.MaxSOC, minBound, maxBound); err != nil {
		return err
	}
	if s.MinSOC >= s.MaxSOC {
		return fmt.Errorf("policy: min_soc must be less than max_soc")
	}
	if err := positivePower("charge_power_kw", s.ChargePowerKW); err != nil {
		return err
	}
	return positivePower("discharge_power_kw", s.DischargePowerKW)
}

func socBounds() (float64, float64) {
	desc, ok := contracts.LookupMetric(contracts.MetricEnergyStorageStateOfCharge)
	if !ok || desc.MinValue == nil || desc.MaxValue == nil {
		return 0, 100
	}
	return *desc.MinValue, *desc.MaxValue
}

func inSOCRange(name string, value, minBound, maxBound float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("policy: %s must be finite", name)
	}
	if value < minBound || value > maxBound {
		return fmt.Errorf("policy: %s must be between %g and %g", name, minBound, maxBound)
	}
	return nil
}

func positivePower(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return fmt.Errorf("policy: %s must be finite and positive", name)
	}
	return nil
}
