package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

type ScopeType string

const (
	ScopeSite  ScopeType = "site"
	ScopeAsset ScopeType = "asset"
	ScopeCU    ScopeType = "cu"
)

// MetricAccess is the binding direction a caller requires.
type MetricAccess string

const (
	MetricAccessRead  MetricAccess = "read"
	MetricAccessWrite MetricAccess = "write"
)

type Lifecycle string

const (
	LifecycleUnspecified    Lifecycle = "unspecified"
	LifecycleActive         Lifecycle = "active"
	LifecycleDisabled       Lifecycle = "disabled"
	LifecycleDecommissioned Lifecycle = "decommissioned"
)

// BindingAccess is the access mode stored on a metric binding.
type BindingAccess string

const (
	BindingAccessUnspecified BindingAccess = "unspecified"
	BindingAccessRead        BindingAccess = "read"
	BindingAccessWrite       BindingAccess = "write"
	BindingAccessReadWrite   BindingAccess = "read_write"
)

type ExclusionReason string

const (
	ExclusionUnspecified        ExclusionReason = "unspecified"
	ExclusionNotActive          ExclusionReason = "not_active"
	ExclusionMissingCapability  ExclusionReason = "missing_capability"
	ExclusionCapabilityDisabled ExclusionReason = "capability_disabled"
)

type PrecheckFailureReason string

const (
	PrecheckUnspecified           PrecheckFailureReason = "unspecified"
	PrecheckInvalidCapabilitySpec PrecheckFailureReason = "invalid_capability_spec"
	PrecheckMissingMetricBinding  PrecheckFailureReason = "missing_metric_binding"
)

type MetricRequirement struct {
	MetricID contracts.MetricID
	Access   MetricAccess
}

// ScopeQuery is the ResolveScope request. External addresses are not returned.
type ScopeQuery struct {
	TenantID              string
	ScopeType             ScopeType
	ScopeID               string
	RequiredCapabilityIDs []contracts.CapabilityID
	RequiredMetrics       []MetricRequirement
}

// SafetyConstraint is the canonical-unit envelope copied from the binding.
// A nil pointer means the limit was not set.
type SafetyConstraint struct {
	MinValue           *float64
	MaxValue           *float64
	MaxChangePerSecond *float64
	Version            int64
}

type ResolvedCapability struct {
	CapabilityID  contracts.CapabilityID
	SchemaVersion int32
	Spec          map[string]any
	Enabled       bool
	Version       int64
}

type ResolvedBinding struct {
	MetricID   contracts.MetricID
	AccessMode BindingAccess
	Enabled    bool
	Revision   int64
	Safety     *SafetyConstraint
}

type ResolvedCU struct {
	CUID         string
	AssetID      string
	Lifecycle    Lifecycle
	NodeVersion  int64
	Capabilities []ResolvedCapability
	Bindings     []ResolvedBinding
}

type ScopeExclusion struct {
	CUID    string
	AssetID string
	Reason  ExclusionReason
	Detail  string
}

type ScopePrecheckFailure struct {
	CUID    string
	AssetID string
	Reason  PrecheckFailureReason
	Detail  string
}

// ResolvedScope is Decision's view of a Resource scope expansion.
type ResolvedScope struct {
	ScopeType        ScopeType
	ScopeID          string
	ResourceRevision string
	PrecheckOK       bool
	Members          []ResolvedCU
	Exclusions       []ScopeExclusion
	PrecheckFailures []ScopePrecheckFailure
}

// ResourcePort expands a site, asset, or CU into controllable members.
type ResourcePort interface {
	ResolveScope(ctx context.Context, query ScopeQuery) (ResolvedScope, error)
}
