package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrSiteNotFound),
		errors.Is(err, domain.ErrResourceNotFound),
		errors.Is(err, domain.ErrCUNotFound),
		errors.Is(err, domain.ErrCUCapabilityNotFound),
		errors.Is(err, domain.ErrPointNotFound),
		errors.Is(err, domain.ErrJobNotFound),
		errors.Is(err, domain.ErrScopeNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrVersionConflict):
		return status.Error(codes.Aborted, err.Error())
	default:
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "required") || strings.Contains(lower, "invalid") {
			return status.Error(codes.InvalidArgument, msg)
		}
		return status.Error(codes.Internal, msg)
	}
}

func SiteStatusProtoToDomain(s resourcepb.SiteStatus) model.OperatingStatus {
	switch s {
	case resourcepb.SiteStatus_SITE_STATUS_UNDER_CONSTRUCTION:
		return model.OperatingStatusUnderConstruction
	case resourcepb.SiteStatus_SITE_STATUS_OPERATING:
		return model.OperatingStatusOperating
	case resourcepb.SiteStatus_SITE_STATUS_FAULT:
		return model.OperatingStatusFault
	case resourcepb.SiteStatus_SITE_STATUS_OFFLINE:
		return model.OperatingStatusOffline
	default:
		return model.OperatingStatusUnknown
	}
}

func SiteStatusDomainToProto(s model.OperatingStatus) resourcepb.SiteStatus {
	switch s {
	case model.OperatingStatusUnderConstruction:
		return resourcepb.SiteStatus_SITE_STATUS_UNDER_CONSTRUCTION
	case model.OperatingStatusOperating:
		return resourcepb.SiteStatus_SITE_STATUS_OPERATING
	case model.OperatingStatusFault:
		return resourcepb.SiteStatus_SITE_STATUS_FAULT
	case model.OperatingStatusOffline:
		return resourcepb.SiteStatus_SITE_STATUS_OFFLINE
	default:
		return resourcepb.SiteStatus_SITE_STATUS_UNKNOWN
	}
}

func PointAccessModeProtoToDomain(t resourcepb.PointAccessMode) (model.AccessMode, error) {
	switch t {
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ:
		return model.AccessModeRead, nil
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_WRITE:
		return model.AccessModeWrite, nil
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ_WRITE:
		return model.AccessModeReadWrite, nil
	default:
		return "", fmt.Errorf("AccessMode is required")
	}
}

func PointAccessModeDomainToProto(t model.AccessMode) resourcepb.PointAccessMode {
	switch t {
	case model.AccessModeRead:
		return resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ
	case model.AccessModeWrite:
		return resourcepb.PointAccessMode_POINT_ACCESS_MODE_WRITE
	case model.AccessModeReadWrite:
		return resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ_WRITE
	default:
		return resourcepb.PointAccessMode_POINT_ACCESS_MODE_UNSPECIFIED
	}
}

func LocationProtoToDomain(loc *resourcepb.Location) model.Location {
	if loc == nil {
		return model.Location{}
	}
	return model.Location{
		Latitude:  loc.GetLatitude(),
		Longitude: loc.GetLongitude(),
		Address:   loc.GetAddress(),
	}
}

func SiteDomainToProto(s *model.Site) *resourcepb.Site {
	if s == nil {
		return nil
	}
	var location *resourcepb.Location
	if s.Location != nil {
		location = &resourcepb.Location{
			Latitude:  s.Location.Latitude,
			Longitude: s.Location.Longitude,
			Address:   s.Location.Address,
		}
	}
	desc := ""
	if s.Description != nil {
		desc = *s.Description
	}
	return &resourcepb.Site{
		ID:          s.ID,
		TenantID:    s.TenantID,
		Name:        s.DisplayName,
		Location:    location,
		Description: desc,
		Status:      SiteStatusDomainToProto(s.OperatingStatus),
	}
}

func AssetToProto(a *model.Asset) (*resourcepb.Asset, error) {
	if a == nil {
		return nil, nil
	}
	meta, err := MapToStructPB(a.Metadata)
	if err != nil {
		return nil, err
	}
	siteID := ""
	if a.ParentID != nil {
		siteID = *a.ParentID
	}
	subType := ""
	if a.SubType != nil {
		subType = *a.SubType
	}
	capacity := 0.0
	if a.RatedCapacityKW != nil {
		capacity = *a.RatedCapacityKW
	}
	dispatchMode := ""
	if a.DispatchMode != nil {
		dispatchMode = *a.DispatchMode
	}
	energyType := ""
	if a.EnergyType != nil {
		energyType = *a.EnergyType
	}
	ownerType := ""
	if a.OwnerType != nil {
		ownerType = *a.OwnerType
	}
	desc := ""
	if a.Description != nil {
		desc = *a.Description
	}
	market := false
	if a.MarketEnabled != nil {
		market = *a.MarketEnabled
	}
	return &resourcepb.Asset{
		ID:              a.ID,
		TenantID:        a.TenantID,
		SiteID:          siteID,
		Name:            a.DisplayName,
		DispatchStatus:  string(a.DispatchStatus),
		RatedCapacityKW: capacity,
		DispatchMode:    dispatchMode,
		EnergyType:      energyType,
		OwnerType:       ownerType,
		SubType:         subType,
		Description:     desc,
		MarketEnabled:   market,
		Metadata:        meta,
	}, nil
}

func ResourceDomainToProto(n *model.Node) (*resourcepb.Resource, error) {
	if n == nil {
		return nil, nil
	}
	meta, err := MapToStructPB(n.Metadata)
	if err != nil {
		return nil, err
	}
	parentID := ""
	if n.ParentID != nil {
		parentID = *n.ParentID
	}
	subType := ""
	if n.SubType != nil {
		subType = *n.SubType
	}
	description := ""
	if n.Description != nil {
		description = *n.Description
	}
	return &resourcepb.Resource{
		ID:              n.ID,
		TenantID:        n.TenantID,
		ParentID:        parentID,
		DisplayName:     n.DisplayName,
		Type:            n.Type,
		SubType:         subType,
		LifecycleStatus: ResourceLifecycleStatusDomainToProto(n.LifecycleStatus),
		Description:     description,
		Path:            n.Path,
		Depth:           int32(n.Depth),
		Metadata:        meta,
	}, nil
}

func ResourceLifecycleStatusProtoToDomain(s resourcepb.ResourceLifecycleStatus) model.NodeLifecycleStatus {
	switch s {
	case resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_ACTIVE:
		return model.NodeLifecycleActive
	case resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_DISABLED:
		return model.NodeLifecycleDisabled
	case resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_DECOMMISSIONED:
		return model.NodeLifecycleArchived
	default:
		return model.NodeLifecycleDraft
	}
}

func ResourceLifecycleStatusDomainToProto(s model.NodeLifecycleStatus) resourcepb.ResourceLifecycleStatus {
	switch s {
	case model.NodeLifecycleActive:
		return resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_ACTIVE
	case model.NodeLifecycleDisabled:
		return resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_DISABLED
	case model.NodeLifecycleArchived, model.NodeLifecycleDeleted:
		return resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_DECOMMISSIONED
	default:
		return resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_UNSPECIFIED
	}
}

func ConnectionDomainToProto(c *model.ConnectionConfig) *resourcepb.ConnectionConfig {
	if c == nil {
		return nil
	}
	return &resourcepb.ConnectionConfig{
		Host:    c.Host,
		Port:    int32(c.Port),
		Timeout: int32(c.Timeout),
		RetryPolicy: &resourcepb.RetryPolicy{
			MaxAttempts:       int32(c.RetryPolicy.MaxAttempts),
			InitialBackoffMS:  int32(c.RetryPolicy.InitialBackoffMS),
			MaxBackoffMS:      int32(c.RetryPolicy.MaxBackoffMS),
			BackoffMultiplier: c.RetryPolicy.BackoffMultiplier,
		},
	}
}

func ConnectionProtoToDomain(pb *resourcepb.ConnectionConfig) (*model.ConnectionConfig, error) {
	if pb == nil {
		return nil, nil
	}
	cc := &model.ConnectionConfig{
		Host:    pb.GetHost(),
		Port:    int(pb.GetPort()),
		Timeout: int(pb.GetTimeout()),
	}
	if rp := pb.GetRetryPolicy(); rp != nil {
		cc.RetryPolicy = model.RetryPolicy{
			MaxAttempts:       int(rp.GetMaxAttempts()),
			InitialBackoffMS:  int(rp.GetInitialBackoffMS()),
			MaxBackoffMS:      int(rp.GetMaxBackoffMS()),
			BackoffMultiplier: rp.GetBackoffMultiplier(),
		}
	}
	return cc, nil
}

func CUToProto(cu *model.CU) (*resourcepb.CU, error) {
	if cu == nil {
		return nil, nil
	}
	meta, err := MapToStructPB(cu.Metadata)
	if err != nil {
		return nil, err
	}
	parentID := ""
	if cu.ParentID != nil {
		parentID = *cu.ParentID
	}
	cuType := ""
	if cu.SubType != nil {
		cuType = *cu.SubType
	}
	protocol := ""
	if cu.Protocol != nil {
		protocol = *cu.Protocol
	}
	provider := ""
	if cu.Provider != nil {
		provider = *cu.Provider
	}
	externalID := ""
	if cu.ExternalID != nil {
		externalID = *cu.ExternalID
	}
	protocolConfig, err := MapToStructPB(cu.ProtocolConfig)
	if err != nil {
		return nil, err
	}
	return &resourcepb.CU{
		ID:             cu.ID,
		TenantID:       cu.TenantID,
		ParentID:       parentID,
		Name:           cu.DisplayName,
		Type:           cuType,
		Metadata:       meta,
		Protocol:       protocol,
		ProtocolConfig: protocolConfig,
		Provider:       provider,
		ExternalID:     externalID,
		Connection:     ConnectionDomainToProto(cu.Connection),
	}, nil
}

func PointToProto(p *model.Point) (*resourcepb.Point, error) {
	if p == nil {
		return nil, nil
	}
	return &resourcepb.Point{
		ID: p.ID, AssetID: p.AssetID, CUID: p.CUID,
		MetricID: string(p.MetricID), ExternalAddress: p.ExternalAddress,
		AccessMode: PointAccessModeDomainToProto(p.AccessMode),
		Scale:      p.Scale, Offset: p.Offset, Enabled: p.Enabled, Revision: p.Revision,
		SafetyConstraint: PointSafetyConstraintDomainToProto(p.SafetyConstraint),
	}, nil
}

func PointSafetyConstraintProtoToDomain(
	c *resourcepb.PointSafetyConstraint,
) *model.PointSafetyConstraint {
	if c == nil {
		return nil
	}
	out := &model.PointSafetyConstraint{Version: c.GetVersion()}
	if c.GetMinValue() != nil {
		value := c.GetMinValue().GetValue()
		out.MinValue = &value
	}
	if c.GetMaxValue() != nil {
		value := c.GetMaxValue().GetValue()
		out.MaxValue = &value
	}
	if c.GetMaxChangePerSecond() != nil {
		value := c.GetMaxChangePerSecond().GetValue()
		out.MaxChangePerSecond = &value
	}
	return out
}

func PointSafetyConstraintDomainToProto(
	c *model.PointSafetyConstraint,
) *resourcepb.PointSafetyConstraint {
	if c == nil {
		return nil
	}
	out := &resourcepb.PointSafetyConstraint{Version: c.Version}
	if c.MinValue != nil {
		out.MinValue = wrapperspb.Double(*c.MinValue)
	}
	if c.MaxValue != nil {
		out.MaxValue = wrapperspb.Double(*c.MaxValue)
	}
	if c.MaxChangePerSecond != nil {
		out.MaxChangePerSecond = wrapperspb.Double(*c.MaxChangePerSecond)
	}
	return out
}

func JobDomainToProto(j *model.Job) *resourcepb.Job {
	if j == nil {
		return nil
	}

	var startedAt, finishedAt, nextRetryAt *timestamppb.Timestamp
	if j.StartedAt != nil {
		startedAt = timestamppb.New(*j.StartedAt)
	}
	if j.FinishedAt != nil {
		finishedAt = timestamppb.New(*j.FinishedAt)
	}
	if j.NextRetryAt != nil {
		nextRetryAt = timestamppb.New(*j.NextRetryAt)
	}

	var createdAt, updatedAt *timestamppb.Timestamp
	if !j.CreatedAt.IsZero() {
		createdAt = timestamppb.New(j.CreatedAt)
	}
	if !j.UpdatedAt.IsZero() {
		updatedAt = timestamppb.New(j.UpdatedAt)
	}

	var jobType resourcepb.JobType
	if j.OperationType == model.JobOperationImport {
		switch j.TargetType {
		case model.JobTargetAsset:
			jobType = resourcepb.JobType_IMPORT_JOB_TYPE_ASSET
		case model.JobTargetCU:
			jobType = resourcepb.JobType_IMPORT_JOB_TYPE_CU
		case model.JobTargetPoint:
			jobType = resourcepb.JobType_IMPORT_JOB_TYPE_POINT
		default:
			jobType = resourcepb.JobType_IMPORT_JOB_TYPE_UNSPECIFIED
		}
	} else {
		jobType = resourcepb.JobType_IMPORT_JOB_TYPE_UNSPECIFIED
	}

	var jobStatus resourcepb.JobStatus
	switch j.Status {
	case model.JobStatusPending:
		jobStatus = resourcepb.JobStatus_IMPORT_JOB_STATUS_PENDING
	case model.JobStatusRunning:
		jobStatus = resourcepb.JobStatus_IMPORT_JOB_STATUS_RUNNING
	case model.JobStatusSuccess:
		jobStatus = resourcepb.JobStatus_IMPORT_JOB_STATUS_SUCCESS
	case model.JobStatusFailed:
		jobStatus = resourcepb.JobStatus_IMPORT_JOB_STATUS_FAILED
	default:
		jobStatus = resourcepb.JobStatus_IMPORT_JOB_STATUS_UNSPECIFIED
	}

	return &resourcepb.Job{
		ID:          j.ID,
		TenantID:    j.TenantID,
		Type:        jobType,
		Status:      jobStatus,
		Payload:     j.Payload,
		Total:       int64(j.Total),
		Succeeded:   int64(j.Succeeded),
		FailedCount: int64(j.FailedCount),
		ErrorMsg:    j.ErrorMsg,
		ResultJSON:  j.ResultJSON,
		Attempts:    int64(j.Attempts),
		MaxAttempts: int64(j.MaxAttempts),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
		NextRetryAt: nextRetryAt,
	}
}

// AssetImportItemProtoToCommand converts a proto AssetItem to the
// domain types.AssetItem used by SubmitBatchImport.
func AssetImportItemProtoToCommand(p *resourcepb.AssetItem) types.AssetItem {
	if p == nil {
		return types.AssetItem{}
	}
	meta, _ := StructPBToMap(p.GetMetadata())
	subType := strings.TrimSpace(p.GetSubType())
	var subTypePtr *string
	if subType != "" {
		subTypePtr = &subType
	}
	kw := p.GetRatedCapacityKW()
	kwPtr := &kw

	var dispatchMode *string
	if v := strings.TrimSpace(p.GetDispatchMode()); v != "" {
		dispatchMode = &v
	}
	var energyType *string
	if v := strings.TrimSpace(p.GetEnergyType()); v != "" {
		energyType = &v
	}
	var ownerType *string
	if v := strings.TrimSpace(p.GetOwnerType()); v != "" {
		ownerType = &v
	}
	var description *string
	if v := strings.TrimSpace(p.GetDescription()); v != "" {
		description = &v
	}
	me := p.GetMarketEnabled()
	mePtr := &me

	ds := model.DispatchStatus(strings.TrimSpace(p.GetDispatchStatus()))
	if ds == "" {
		ds = model.DispatchStatusUnknown
	}

	return types.AssetItem{
		Name:            strings.TrimSpace(p.GetName()),
		DispatchStatus:  ds,
		SubType:         subTypePtr,
		RatedCapacityKW: kwPtr,
		DispatchMode:    dispatchMode,
		EnergyType:      energyType,
		OwnerType:       ownerType,
		Description:     description,
		MarketEnabled:   mePtr,
		Metadata:        meta,
	}
}

// CUImportItemProtoToCommand converts a proto CUImportItem to types.CUItem.
func CUImportItemProtoToCommand(p *resourcepb.CUItem) types.CUItem {
	if p == nil {
		return types.CUItem{}
	}
	meta, _ := StructPBToMap(p.GetMetadata())
	protocolConfig, _ := StructPBToMap(p.GetProtocolConfig())
	var parentID *string
	if v := strings.TrimSpace(p.GetParentID()); v != "" {
		parentID = &v
	}
	var protocol *string
	if v := strings.TrimSpace(p.GetProtocol()); v != "" {
		protocol = &v
	}
	var provider *string
	if v := strings.TrimSpace(p.GetProvider()); v != "" {
		provider = &v
	}
	var externalID *string
	if v := strings.TrimSpace(p.GetExternalID()); v != "" {
		externalID = &v
	}
	conn, _ := ConnectionProtoToDomain(p.GetConnection())
	return types.CUItem{
		ParentID:       parentID,
		Name:           p.GetName(),
		Type:           p.GetType(),
		Provider:       provider,
		ExternalID:     externalID,
		Protocol:       protocol,
		ProtocolConfig: protocolConfig,
		Connection:     conn,
		Metadata:       meta,
	}
}

// PointImportItemProtoToCommand converts a proto PointImportItem to types.PointItem.
func PointImportItemProtoToCommand(p *resourcepb.PointItem) types.PointItem {
	if p == nil {
		return types.PointItem{}
	}
	accessMode, _ := PointAccessModeProtoToDomain(p.GetAccessMode())
	scale := 1.0
	if p.GetScale() != nil {
		scale = p.GetScale().GetValue()
	}
	offset := 0.0
	if p.GetOffset() != nil {
		offset = p.GetOffset().GetValue()
	}
	return types.PointItem{
		MetricID:         p.GetMetricID(),
		ExternalAddress:  p.GetExternalAddress(),
		AccessMode:       accessMode,
		Scale:            scale,
		Offset:           offset,
		Enabled:          p.GetEnabled(),
		SafetyConstraint: PointSafetyConstraintProtoToDomain(p.GetSafetyConstraint()),
	}
}

// BatchItemErrorDomainToProto converts a types.BatchItemError to the proto
// BatchItemError returned in SubmitBatchImportResponse.
func BatchItemErrorDomainToProto(e types.BatchItemError) *resourcepb.BatchItemError {
	return &resourcepb.BatchItemError{
		Index:  int32(e.Index),
		Name:   e.Name,
		Reason: e.Reason,
	}
}

// StructPBToMap converts a *structpb.Struct to map[string]any.
func StructPBToMap(s *structpb.Struct) (map[string]any, error) {
	if s == nil {
		return nil, nil
	}
	return s.AsMap(), nil
}

func MapToStructPB(m map[string]any) (*structpb.Struct, error) {
	if m == nil {
		return nil, nil
	}
	return structpb.NewStruct(m)
}

func logIn(ctx context.Context, method string) {
	logging.Infof(ctx, logrus.Fields{
		"component": "resource_grpc",
		"method":    method,
	}, "request_in")
}

func scopeTypeProtoToDomain(scopeType resourcepb.ScopeType) string {
	switch scopeType {
	case resourcepb.ScopeType_SCOPE_TYPE_SITE:
		return string(model.ScopeTypeSite)
	case resourcepb.ScopeType_SCOPE_TYPE_ASSET:
		return string(model.ScopeTypeAsset)
	case resourcepb.ScopeType_SCOPE_TYPE_CU:
		return string(model.ScopeTypeCU)
	default:
		return ""
	}
}

func metricAccessProtoToDomain(access resourcepb.MetricAccessRequirement) string {
	switch access {
	case resourcepb.MetricAccessRequirement_METRIC_ACCESS_REQUIREMENT_READ:
		return string(model.MetricAccessNeedRead)
	case resourcepb.MetricAccessRequirement_METRIC_ACCESS_REQUIREMENT_WRITE:
		return string(model.MetricAccessNeedWrite)
	default:
		return ""
	}
}

func resolvedScopeToProto(resolved *model.ResolvedScope) (*resourcepb.ResolveScopeResponse, error) {
	if resolved == nil {
		return nil, nil
	}
	members := make([]*resourcepb.ResolvedCU, 0, len(resolved.Members))
	for _, member := range resolved.Members {
		item, err := resolvedCUToProto(member)
		if err != nil {
			return nil, toGRPCError(err)
		}
		members = append(members, item)
	}
	exclusions := make([]*resourcepb.ScopeExclusion, 0, len(resolved.Exclusions))
	for _, exclusion := range resolved.Exclusions {
		exclusions = append(exclusions, &resourcepb.ScopeExclusion{
			CUID:    exclusion.CUID,
			AssetID: exclusion.AssetID,
			Reason:  exclusionReasonToProto(exclusion.Reason),
			Detail:  exclusion.Detail,
		})
	}
	failures := make([]*resourcepb.ScopePrecheckFailure, 0, len(resolved.PrecheckFailures))
	for _, failure := range resolved.PrecheckFailures {
		failures = append(failures, &resourcepb.ScopePrecheckFailure{
			CUID:    failure.CUID,
			AssetID: failure.AssetID,
			Reason:  precheckFailureReasonToProto(failure.Reason),
			Detail:  failure.Detail,
		})
	}
	return &resourcepb.ResolveScopeResponse{
		ScopeType:        scopeTypeDomainToProto(resolved.ScopeType),
		ScopeID:          resolved.ScopeID,
		ResourceRevision: resolved.ResourceRevision,
		PrecheckOK:       resolved.PrecheckOK,
		Members:          members,
		Exclusions:       exclusions,
		PrecheckFailures: failures,
	}, nil
}

func resolvedCUToProto(cu model.ScopeCU) (*resourcepb.ResolvedCU, error) {
	capabilities := make([]*resourcepb.ResolvedCapability, 0, len(cu.Capabilities))
	for _, capability := range cu.Capabilities {
		spec, err := capabilitySpecToProto(capability.Spec)
		if err != nil {
			return nil, err
		}
		capabilities = append(capabilities, &resourcepb.ResolvedCapability{
			CapabilityID:  capability.CapabilityID,
			SchemaVersion: int32(capability.SchemaVersion),
			Spec:          spec,
			Enabled:       capability.Enabled,
			Version:       capability.Version,
		})
	}
	bindings := make([]*resourcepb.ResolvedMetricBinding, 0, len(cu.Bindings))
	for _, binding := range cu.Bindings {
		bindings = append(bindings, &resourcepb.ResolvedMetricBinding{
			MetricID:         string(binding.MetricID),
			AccessMode:       PointAccessModeDomainToProto(binding.AccessMode),
			Enabled:          binding.Enabled,
			Revision:         binding.Revision,
			SafetyConstraint: PointSafetyConstraintDomainToProto(binding.Safety),
		})
	}
	return &resourcepb.ResolvedCU{
		CUID:            cu.CUID,
		AssetID:         cu.AssetID,
		LifecycleStatus: ResourceLifecycleStatusDomainToProto(cu.Lifecycle),
		NodeVersion:     cu.NodeVersion,
		Capabilities:    capabilities,
		Bindings:        bindings,
	}, nil
}

func capabilitySpecToProto(raw []byte) (*structpb.Struct, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var specMap map[string]any
	if err := json.Unmarshal(raw, &specMap); err != nil {
		return nil, err
	}
	return structpb.NewStruct(specMap)
}

func scopeTypeDomainToProto(scopeType model.ScopeType) resourcepb.ScopeType {
	switch scopeType {
	case model.ScopeTypeSite:
		return resourcepb.ScopeType_SCOPE_TYPE_SITE
	case model.ScopeTypeAsset:
		return resourcepb.ScopeType_SCOPE_TYPE_ASSET
	case model.ScopeTypeCU:
		return resourcepb.ScopeType_SCOPE_TYPE_CU
	default:
		return resourcepb.ScopeType_SCOPE_TYPE_UNSPECIFIED
	}
}

func exclusionReasonToProto(reason model.ExclusionReason) resourcepb.ScopeExclusionReason {
	switch reason {
	case model.ExclusionNotActive:
		return resourcepb.ScopeExclusionReason_SCOPE_EXCLUSION_REASON_NOT_ACTIVE
	case model.ExclusionMissingCapability:
		return resourcepb.ScopeExclusionReason_SCOPE_EXCLUSION_REASON_MISSING_CAPABILITY
	case model.ExclusionCapabilityDisabled:
		return resourcepb.ScopeExclusionReason_SCOPE_EXCLUSION_REASON_CAPABILITY_DISABLED
	default:
		return resourcepb.ScopeExclusionReason_SCOPE_EXCLUSION_REASON_UNSPECIFIED
	}
}

func precheckFailureReasonToProto(reason model.PrecheckFailureReason) resourcepb.ScopePrecheckFailureReason {
	switch reason {
	case model.PrecheckInvalidCapabilitySpec:
		return resourcepb.ScopePrecheckFailureReason_SCOPE_PRECHECK_FAILURE_REASON_INVALID_CAPABILITY_SPEC
	case model.PrecheckMissingMetricBinding:
		return resourcepb.ScopePrecheckFailureReason_SCOPE_PRECHECK_FAILURE_REASON_MISSING_METRIC_BINDING
	default:
		return resourcepb.ScopePrecheckFailureReason_SCOPE_PRECHECK_FAILURE_REASON_UNSPECIFIED
	}
}
