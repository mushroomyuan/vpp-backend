package policy

import (
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// SOCScopeQuery is the ResolveScope request used before a SOC policy may be enabled.
// Active storage CUs must have a valid energy.storage.v1 spec, a SOC read binding,
// and an active-power setpoint write binding. Other CUs are exclusions, not failures.
func SOCScopeQuery(p *Policy) (port.ScopeQuery, error) {
	if err := p.Validate(); err != nil {
		return port.ScopeQuery{}, err
	}
	if p.Kind != KindSOCThreshold {
		return port.ScopeQuery{}, fmt.Errorf("policy: kind %q has no scope precheck", p.Kind)
	}
	return SOCScopeQueryFor(p.TenantID, p.Scope)
}

// SOCScopeQueryFor is the ResolveScope request for a SOC objective.
// Enable precheck and plan execution both use it, so a revision check sees the same members.
func SOCScopeQueryFor(tenantID string, scope TargetScope) (port.ScopeQuery, error) {
	if err := scope.Validate(); err != nil {
		return port.ScopeQuery{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return port.ScopeQuery{}, fmt.Errorf("policy: tenant_id is required")
	}
	return port.ScopeQuery{
		TenantID:  tenantID,
		ScopeType: scope.Type,
		ScopeID:   scope.ID,
		RequiredCapabilityIDs: []contracts.CapabilityID{
			contracts.CapabilityEnergyStorage,
		},
		RequiredMetrics: []port.MetricRequirement{
			{MetricID: contracts.MetricEnergyStorageStateOfCharge, Access: port.MetricAccessRead},
			{MetricID: contracts.MetricElectricalActivePowerSetpoint, Access: port.MetricAccessWrite},
		},
	}, nil
}
