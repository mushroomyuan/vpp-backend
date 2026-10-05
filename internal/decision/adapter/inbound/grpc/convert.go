package grpc

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	decisionpb "github.com/mushroomyuan/vpp-backend/api/decision/proto/gen"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func policyToProto(p *policy.Policy) *decisionpb.Policy {
	if p == nil {
		return nil
	}
	out := &decisionpb.Policy{
		ID:        p.ID,
		TenantID:  p.TenantID,
		Name:      p.Name,
		Kind:      kindToProto(p.Kind),
		Scope:     scopeToProto(p.Scope),
		Enabled:   p.Enabled,
		Cooldown:  durationpb.New(p.Cooldown),
		Version:   p.Version,
		CreatedBy: p.CreatedBy,
		UpdatedBy: p.UpdatedBy,
	}
	if !p.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(p.UpdatedAt)
	}
	if p.SOC != nil {
		out.SOC = &decisionpb.SOCThresholdSpec{
			MinSOC:           p.SOC.MinSOC,
			MaxSOC:           p.SOC.MaxSOC,
			ChargePowerKW:    p.SOC.ChargePowerKW,
			DischargePowerKW: p.SOC.DischargePowerKW,
		}
	}
	return out
}

func kindToProto(kind policy.Kind) decisionpb.PolicyKind {
	switch kind {
	case policy.KindSOCThreshold:
		return decisionpb.PolicyKind_POLICY_KIND_SOC_THRESHOLD
	default:
		return decisionpb.PolicyKind_POLICY_KIND_UNSPECIFIED
	}
}

func kindFromProto(kind decisionpb.PolicyKind) (policy.Kind, error) {
	switch kind {
	case decisionpb.PolicyKind_POLICY_KIND_SOC_THRESHOLD:
		return policy.KindSOCThreshold, nil
	default:
		return "", policy.Invalid(fmt.Errorf("policy: unknown kind %q", kind.String()))
	}
}

func scopeToProto(scope policy.TargetScope) *decisionpb.TargetScope {
	var kind decisionpb.ScopeType
	switch scope.Type {
	case port.ScopeSite:
		kind = decisionpb.ScopeType_SCOPE_TYPE_SITE
	case port.ScopeAsset:
		kind = decisionpb.ScopeType_SCOPE_TYPE_ASSET
	case port.ScopeCU:
		kind = decisionpb.ScopeType_SCOPE_TYPE_CU
	default:
		kind = decisionpb.ScopeType_SCOPE_TYPE_UNSPECIFIED
	}
	return &decisionpb.TargetScope{Type: kind, ID: scope.ID}
}

func scopeFromProto(scope *decisionpb.TargetScope) (policy.TargetScope, error) {
	if scope == nil {
		return policy.TargetScope{}, policy.Invalid(fmt.Errorf("policy: scope is required"))
	}
	var kind port.ScopeType
	switch scope.GetType() {
	case decisionpb.ScopeType_SCOPE_TYPE_SITE:
		kind = port.ScopeSite
	case decisionpb.ScopeType_SCOPE_TYPE_ASSET:
		kind = port.ScopeAsset
	case decisionpb.ScopeType_SCOPE_TYPE_CU:
		kind = port.ScopeCU
	default:
		return policy.TargetScope{}, policy.Invalid(fmt.Errorf("policy: scope type %q is not site, asset, or cu", scope.GetType().String()))
	}
	return policy.TargetScope{Type: kind, ID: scope.GetID()}, nil
}

func socFromProto(spec *decisionpb.SOCThresholdSpec) *policy.SOCThresholdSpec {
	if spec == nil {
		return nil
	}
	return &policy.SOCThresholdSpec{
		MinSOC:           spec.GetMinSOC(),
		MaxSOC:           spec.GetMaxSOC(),
		ChargePowerKW:    spec.GetChargePowerKW(),
		DischargePowerKW: spec.GetDischargePowerKW(),
	}
}

func cooldownFromProto(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}
