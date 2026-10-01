package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

// ScopeType is a site, asset, or CU target expanded by ResolveScope.
type ScopeType string

const (
	ScopeTypeSite  ScopeType = "site"
	ScopeTypeAsset ScopeType = "asset"
	ScopeTypeCU    ScopeType = "cu"
)

// MetricAccessNeed is the binding direction a caller requires.
type MetricAccessNeed string

const (
	MetricAccessNeedRead  MetricAccessNeed = "read"
	MetricAccessNeedWrite MetricAccessNeed = "write"
)

// ExclusionReason explains why a CU in the scope is not a control member.
type ExclusionReason string

const (
	ExclusionNotActive          ExclusionReason = "not_active"
	ExclusionMissingCapability  ExclusionReason = "missing_capability"
	ExclusionCapabilityDisabled ExclusionReason = "capability_disabled"
)

// PrecheckFailureReason explains why an otherwise selected active CU blocks the scope.
type PrecheckFailureReason string

const (
	PrecheckInvalidCapabilitySpec PrecheckFailureReason = "invalid_capability_spec"
	PrecheckMissingMetricBinding  PrecheckFailureReason = "missing_metric_binding"
)

// MetricRequirement is one canonical metric a policy must be able to read or write.
type MetricRequirement struct {
	MetricID contracts.MetricID
	Access   MetricAccessNeed
}

// ScopeCapability is a capability instance attached to a CU in the scope.
type ScopeCapability struct {
	CapabilityID  string
	SchemaVersion int
	Spec          []byte
	Enabled       bool
	Version       int64
}

// ScopeBinding is one active metric binding. External addresses are not loaded.
type ScopeBinding struct {
	MetricID   contracts.MetricID
	AccessMode AccessMode
	Enabled    bool
	Revision   int64
	Safety     *PointSafetyConstraint
}

// ScopeCU is one CU inside the expanded scope, before eligibility filtering.
type ScopeCU struct {
	CUID         string
	AssetID      string
	Lifecycle    NodeLifecycleStatus
	NodeVersion  int64
	Capabilities []ScopeCapability
	Bindings     []ScopeBinding
}

// ScopeSnapshot is the catalog state used both for eligibility and resource_revision.
// Nodes, capabilities, and bindings cover the whole scope, including CUs that are
// later excluded. Revision does not depend on the caller's capability filter.
type ScopeSnapshot struct {
	TenantID string
	ScopeID  string
	RootType ScopeType
	Nodes    []contracts.RevisionNode
	CUs      []ScopeCU
}

// ScopeExclusion is a CU that stays in the scope but is not controlled.
type ScopeExclusion struct {
	CUID    string
	AssetID string
	Reason  ExclusionReason
	Detail  string
}

// ScopePrecheckFailure is an active CU that matched the required capabilities
// but cannot be controlled safely. Any failure fails the whole scope.
type ScopePrecheckFailure struct {
	CUID    string
	AssetID string
	Reason  PrecheckFailureReason
	Detail  string
}

// ResolvedScope is the Decision-facing result of one scope expansion.
type ResolvedScope struct {
	ScopeType        ScopeType
	ScopeID          string
	ResourceRevision string
	PrecheckOK       bool
	Members          []ScopeCU
	Exclusions       []ScopeExclusion
	PrecheckFailures []ScopePrecheckFailure
}

func ParseScopeType(raw string) (ScopeType, error) {
	switch ScopeType(strings.TrimSpace(raw)) {
	case ScopeTypeSite, ScopeTypeAsset, ScopeTypeCU:
		return ScopeType(strings.TrimSpace(raw)), nil
	default:
		return "", fmt.Errorf("invalid scope type %q", raw)
	}
}

// ResolveSnapshot selects CUs by required capabilities and metrics.
// Active CUs that have the required capabilities but lack a valid spec or a
// required binding fail precheck. They are not silently dropped from a
// successful member list. Members is empty whenever precheck fails.
func ResolveSnapshot(
	snapshot ScopeSnapshot,
	scopeType ScopeType,
	requiredCapabilities []contracts.CapabilityID,
	requiredMetrics []MetricRequirement,
) (*ResolvedScope, error) {
	if snapshot.RootType != scopeType {
		return nil, fmt.Errorf("invalid scope type %q for node type %q", scopeType, snapshot.RootType)
	}

	revision, err := contracts.ResourceRevision(
		contracts.RevisionScope{
			TenantID:  snapshot.TenantID,
			ScopeType: string(scopeType),
			ScopeID:   snapshot.ScopeID,
		},
		snapshot.Nodes,
		capabilityRevisions(snapshot.CUs),
		bindingRevisions(snapshot.CUs),
	)
	if err != nil {
		return nil, err
	}

	members := make([]ScopeCU, 0)
	exclusions := make([]ScopeExclusion, 0)
	failures := make([]ScopePrecheckFailure, 0)
	for _, cu := range snapshot.CUs {
		member, exclusion, cuFailures := classifyScopeCU(cu, requiredCapabilities, requiredMetrics)
		if exclusion != nil {
			exclusions = append(exclusions, *exclusion)
			continue
		}
		if len(cuFailures) > 0 {
			failures = append(failures, cuFailures...)
			continue
		}
		members = append(members, member)
	}

	sort.Slice(exclusions, func(i, j int) bool { return exclusions[i].CUID < exclusions[j].CUID })
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].CUID != failures[j].CUID {
			return failures[i].CUID < failures[j].CUID
		}
		if failures[i].Reason != failures[j].Reason {
			return failures[i].Reason < failures[j].Reason
		}
		return failures[i].Detail < failures[j].Detail
	})
	sort.Slice(members, func(i, j int) bool { return members[i].CUID < members[j].CUID })

	resolved := &ResolvedScope{
		ScopeType:        scopeType,
		ScopeID:          snapshot.ScopeID,
		ResourceRevision: revision,
		PrecheckOK:       len(failures) == 0,
		Exclusions:       exclusions,
		PrecheckFailures: failures,
	}
	if resolved.PrecheckOK {
		resolved.Members = members
	}
	return resolved, nil
}

func classifyScopeCU(
	cu ScopeCU,
	requiredCapabilities []contracts.CapabilityID,
	requiredMetrics []MetricRequirement,
) (ScopeCU, *ScopeExclusion, []ScopePrecheckFailure) {
	if cu.Lifecycle != NodeLifecycleActive {
		return ScopeCU{}, &ScopeExclusion{
			CUID:    cu.CUID,
			AssetID: cu.AssetID,
			Reason:  ExclusionNotActive,
			Detail:  "lifecycle " + string(cu.Lifecycle),
		}, nil
	}

	byCapability := make(map[string]ScopeCapability, len(cu.Capabilities))
	for _, capability := range cu.Capabilities {
		byCapability[capability.CapabilityID] = capability
	}

	var missing, disabled []string
	for _, capabilityID := range requiredCapabilities {
		capability, ok := byCapability[string(capabilityID)]
		if !ok {
			missing = append(missing, string(capabilityID))
			continue
		}
		if !capability.Enabled {
			disabled = append(disabled, string(capabilityID))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return ScopeCU{}, &ScopeExclusion{
			CUID:    cu.CUID,
			AssetID: cu.AssetID,
			Reason:  ExclusionMissingCapability,
			Detail:  "missing capability " + strings.Join(missing, ", "),
		}, nil
	}
	if len(disabled) > 0 {
		sort.Strings(disabled)
		return ScopeCU{}, &ScopeExclusion{
			CUID:    cu.CUID,
			AssetID: cu.AssetID,
			Reason:  ExclusionCapabilityDisabled,
			Detail:  "disabled capability " + strings.Join(disabled, ", "),
		}, nil
	}

	var failures []ScopePrecheckFailure
	for _, capabilityID := range requiredCapabilities {
		capability := byCapability[string(capabilityID)]
		if err := contracts.ValidateCapabilitySpec(
			capabilityID,
			capability.SchemaVersion,
			capability.Spec,
		); err != nil {
			failures = append(failures, ScopePrecheckFailure{
				CUID:    cu.CUID,
				AssetID: cu.AssetID,
				Reason:  PrecheckInvalidCapabilitySpec,
				Detail:  fmt.Sprintf("invalid capability spec %s: %v", capabilityID, err),
			})
		}
	}

	byMetric := make(map[contracts.MetricID]ScopeBinding, len(cu.Bindings))
	for _, binding := range cu.Bindings {
		byMetric[binding.MetricID] = binding
	}
	for _, requirement := range requiredMetrics {
		binding, ok := byMetric[requirement.MetricID]
		if !ok {
			failures = append(failures, ScopePrecheckFailure{
				CUID:    cu.CUID,
				AssetID: cu.AssetID,
				Reason:  PrecheckMissingMetricBinding,
				Detail:  fmt.Sprintf("missing %s binding %s", requirement.Access, requirement.MetricID),
			})
			continue
		}
		if !binding.Enabled {
			failures = append(failures, ScopePrecheckFailure{
				CUID:    cu.CUID,
				AssetID: cu.AssetID,
				Reason:  PrecheckMissingMetricBinding,
				Detail:  fmt.Sprintf("disabled %s binding %s", requirement.Access, requirement.MetricID),
			})
			continue
		}
		if !bindingAllows(binding.AccessMode, requirement.Access) {
			failures = append(failures, ScopePrecheckFailure{
				CUID:    cu.CUID,
				AssetID: cu.AssetID,
				Reason:  PrecheckMissingMetricBinding,
				Detail: fmt.Sprintf(
					"%s binding %s has access_mode %s",
					requirement.Access, requirement.MetricID, binding.AccessMode,
				),
			})
		}
	}
	if len(failures) > 0 {
		return ScopeCU{}, nil, failures
	}

	member := cu
	member.Capabilities = append([]ScopeCapability(nil), cu.Capabilities...)
	member.Bindings = append([]ScopeBinding(nil), cu.Bindings...)
	sort.Slice(member.Capabilities, func(i, j int) bool {
		return member.Capabilities[i].CapabilityID < member.Capabilities[j].CapabilityID
	})
	sort.Slice(member.Bindings, func(i, j int) bool {
		return member.Bindings[i].MetricID < member.Bindings[j].MetricID
	})
	return member, nil, nil
}

func bindingAllows(mode AccessMode, need MetricAccessNeed) bool {
	switch need {
	case MetricAccessNeedRead:
		return mode.AllowsRead()
	case MetricAccessNeedWrite:
		return mode.AllowsWrite()
	default:
		return false
	}
}

func capabilityRevisions(cus []ScopeCU) []contracts.RevisionCapability {
	out := make([]contracts.RevisionCapability, 0)
	for _, cu := range cus {
		for _, capability := range cu.Capabilities {
			out = append(out, contracts.RevisionCapability{
				CUID:         cu.CUID,
				CapabilityID: capability.CapabilityID,
				Version:      capability.Version,
			})
		}
	}
	return out
}

func bindingRevisions(cus []ScopeCU) []contracts.RevisionBinding {
	out := make([]contracts.RevisionBinding, 0)
	for _, cu := range cus {
		for _, binding := range cu.Bindings {
			out = append(out, contracts.RevisionBinding{
				CUID:     cu.CUID,
				MetricID: string(binding.MetricID),
				Version:  binding.Revision,
			})
		}
	}
	return out
}
