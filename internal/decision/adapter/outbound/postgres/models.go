package postgres

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// PolicyModel is the decision_policies row. The SOC body lives in SOCThresholdSpecModel.
type PolicyModel struct {
	ID         string     `gorm:"column:id;primaryKey;type:uuid"`
	TenantID   string     `gorm:"column:tenant_id;not null"`
	Name       string     `gorm:"column:name;not null"`
	Kind       string     `gorm:"column:kind;not null"`
	ScopeType  string     `gorm:"column:scope_type;not null"`
	ScopeID    string     `gorm:"column:scope_id;not null"`
	Enabled    bool       `gorm:"column:enabled;not null"`
	CooldownNs int64      `gorm:"column:cooldown_ns;not null"`
	Version    int64      `gorm:"column:version;not null"`
	CreatedBy  string     `gorm:"column:created_by;not null"`
	UpdatedBy  string     `gorm:"column:updated_by;not null"`
	CreatedAt  time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;not null"`
	DeletedAt  *time.Time `gorm:"column:deleted_at"`
	DeletedBy  *string    `gorm:"column:deleted_by"`
}

func (PolicyModel) TableName() string { return "decision_policies" }

// SOCThresholdSpecModel is the one-to-one typed spec. It is not a JSON document.
type SOCThresholdSpecModel struct {
	PolicyID         string  `gorm:"column:policy_id;primaryKey;type:uuid"`
	MinSOC           float64 `gorm:"column:min_soc;not null"`
	MaxSOC           float64 `gorm:"column:max_soc;not null"`
	ChargePowerKW    float64 `gorm:"column:charge_power_kw;not null"`
	DischargePowerKW float64 `gorm:"column:discharge_power_kw;not null"`
}

func (SOCThresholdSpecModel) TableName() string { return "decision_soc_threshold_specs" }

func policyToModels(p *policy.Policy) (PolicyModel, SOCThresholdSpecModel) {
	row := PolicyModel{
		ID:         p.ID,
		TenantID:   p.TenantID,
		Name:       p.Name,
		Kind:       string(p.Kind),
		ScopeType:  string(p.Scope.Type),
		ScopeID:    p.Scope.ID,
		Enabled:    p.Enabled,
		CooldownNs: int64(p.Cooldown),
		Version:    p.Version,
		CreatedBy:  p.CreatedBy,
		UpdatedBy:  p.UpdatedBy,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
	}
	spec := SOCThresholdSpecModel{PolicyID: p.ID}
	if p.SOC != nil {
		spec.MinSOC = p.SOC.MinSOC
		spec.MaxSOC = p.SOC.MaxSOC
		spec.ChargePowerKW = p.SOC.ChargePowerKW
		spec.DischargePowerKW = p.SOC.DischargePowerKW
	}
	return row, spec
}

func modelsToPolicy(row PolicyModel, spec SOCThresholdSpecModel) (*policy.Policy, error) {
	p := &policy.Policy{
		ID:       row.ID,
		TenantID: row.TenantID,
		Name:     row.Name,
		Kind:     policy.Kind(row.Kind),
		Scope: policy.TargetScope{
			Type: port.ScopeType(row.ScopeType),
			ID:   row.ScopeID,
		},
		Enabled:   row.Enabled,
		Cooldown:  time.Duration(row.CooldownNs),
		Version:   row.Version,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
		CreatedBy: row.CreatedBy,
		UpdatedBy: row.UpdatedBy,
		SOC: &policy.SOCThresholdSpec{
			MinSOC:           spec.MinSOC,
			MaxSOC:           spec.MaxSOC,
			ChargePowerKW:    spec.ChargePowerKW,
			DischargePowerKW: spec.DischargePowerKW,
		},
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}
