package grpc

import (
	"errors"
	"fmt"
	"strings"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrSiteNotFound),
		errors.Is(err, domain.ErrResourceNotFound),
		errors.Is(err, domain.ErrCUNotFound),
		errors.Is(err, domain.ErrPointNotFound),
		errors.Is(err, domain.ErrJobNotFound):
		return status.Error(codes.NotFound, err.Error())
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

func PointDataTypeProtoToDomain(t resourcepb.PointDataType) (model.DataType, error) {
	switch t {
	case resourcepb.PointDataType_POINT_DATA_TYPE_FLOAT:
		return model.DataTypeFloat, nil
	case resourcepb.PointDataType_POINT_DATA_TYPE_INT:
		return model.DataTypeInt, nil
	case resourcepb.PointDataType_POINT_DATA_TYPE_BOOL:
		return model.DataTypeBool, nil
	case resourcepb.PointDataType_POINT_DATA_TYPE_ENUM:
		return model.DataTypeEnum, nil
	case resourcepb.PointDataType_POINT_DATA_TYPE_UNSPECIFIED:
		return "", fmt.Errorf("DataType is required")
	default:
		return "", fmt.Errorf("unknown DataType: %v", t)
	}
}

func PointDataTypeDomainToProto(t model.DataType) resourcepb.PointDataType {
	switch t {
	case model.DataTypeFloat:
		return resourcepb.PointDataType_POINT_DATA_TYPE_FLOAT
	case model.DataTypeInt:
		return resourcepb.PointDataType_POINT_DATA_TYPE_INT
	case model.DataTypeBool:
		return resourcepb.PointDataType_POINT_DATA_TYPE_BOOL
	case model.DataTypeEnum:
		return resourcepb.PointDataType_POINT_DATA_TYPE_ENUM
	default:
		return resourcepb.PointDataType_POINT_DATA_TYPE_UNSPECIFIED
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

func AssetDomainToProto(a *model.Asset) (*resourcepb.Asset, error) {
	return AssetToProto(a, nil)
}

func AssetToProto(a *model.Asset, runtime *model.AssetRuntime) (*resourcepb.Asset, error) {
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
		Runtime:         AssetRuntimeDomainToProto(runtime),
	}, nil
}

func AssetRuntimeDomainToProto(r *model.AssetRuntime) *resourcepb.AssetRuntime {
	if r == nil {
		return nil
	}
	pb := &resourcepb.AssetRuntime{
		Online:       r.Online,
		Dispatchable: r.Dispatchable,
	}
	if r.CurrentPowerKW != nil {
		v := *r.CurrentPowerKW
		pb.CurrentPowerKW = &v
	}
	if r.AvailablePowerKW != nil {
		v := *r.AvailablePowerKW
		pb.AvailablePowerKW = &v
	}
	if r.SOC != nil {
		v := *r.SOC
		pb.SOC = &v
	}
	if r.NotDispatchableReason != nil {
		v := *r.NotDispatchableReason
		pb.NotDispatchableReason = &v
	}
	if r.MaxChargePowerKW != nil {
		v := *r.MaxChargePowerKW
		pb.MaxChargePowerKW = &v
	}
	if r.MaxDischargePowerKW != nil {
		v := *r.MaxDischargePowerKW
		pb.MaxDischargePowerKW = &v
	}
	if !r.UpdatedAt.IsZero() {
		pb.UpdatedAt = timestamppb.New(r.UpdatedAt)
	}
	return pb
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

func CUDomainToProto(cu *model.CU) (*resourcepb.CU, error) {
	return CUToProto(cu, nil)
}

func CUToProto(cu *model.CU, runtime *model.CURuntime) (*resourcepb.CU, error) {
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
		CapabilityTags: cu.CapabilityTags,
		Metadata:       meta,
		Protocol:       protocol,
		ProtocolConfig: protocolConfig,
		ConnStatus:     string(cu.ConnStatus),
		Provider:       provider,
		ExternalID:     externalID,
		Connection:     ConnectionDomainToProto(cu.Connection),
		Runtime:        CURuntimeDomainToProto(runtime),
	}, nil
}

func CURuntimeDomainToProto(r *model.CURuntime) *resourcepb.CURuntime {
	if r == nil {
		return nil
	}
	pb := &resourcepb.CURuntime{
		ConnStatus: r.ConnStatus,
	}
	if !r.LastSeenAt.IsZero() {
		pb.LastSeenAt = timestamppb.New(r.LastSeenAt)
	}
	if r.LatencyMS != nil {
		v := *r.LatencyMS
		pb.LatencyMS = &v
	}
	if r.LastError != nil {
		v := *r.LastError
		pb.LastError = &v
	}
	if !r.UpdatedAt.IsZero() {
		pb.UpdatedAt = timestamppb.New(r.UpdatedAt)
	}
	return pb
}

func PointDomainToProto(p *model.Point) (*resourcepb.Point, error) {
	return PointToProto(p, nil)
}

func PointToProto(p *model.Point, runtime *model.PointRuntime) (*resourcepb.Point, error) {
	if p == nil {
		return nil, nil
	}
	ext, err := MapToStructPB(p.ExtConfig)
	if err != nil {
		return nil, err
	}
	th, err := MapToStructPB(p.SafetyThresholds)
	if err != nil {
		return nil, err
	}
	return &resourcepb.Point{
		ID:               p.ID,
		AssetID:          p.AssetID,
		CUID:             p.CUID,
		PointKey:         p.PointKey,
		ExternalAddress:  p.ExternalAddress,
		DataType:         PointDataTypeDomainToProto(p.DataType),
		ExtConfig:        ext,
		Description:      p.Description,
		ControlFlag:      p.ControlFlag,
		IsVirtual:        p.IsVirtual,
		SafetyThresholds: th,
		CacheKeyAlias:    p.CacheKeyAlias,
		Runtime:          PointRuntimeDomainToProto(runtime),
	}, nil
}

func PointRuntimeDomainToProto(r *model.PointRuntime) *resourcepb.PointRuntime {
	if r == nil {
		return nil
	}
	pb := &resourcepb.PointRuntime{
		Sequence: r.Sequence,
	}
	if r.Value != nil {
		v := *r.Value
		pb.Value = &v
	}
	if r.NumericValue != nil {
		v := *r.NumericValue
		pb.NumericValue = &v
	}
	if r.QualityStatus != nil {
		v := *r.QualityStatus
		pb.QualityStatus = &v
	}
	if !r.SampledAt.IsZero() {
		pb.SampledAt = timestamppb.New(r.SampledAt)
	}
	if !r.UpdatedAt.IsZero() {
		pb.UpdatedAt = timestamppb.New(r.UpdatedAt)
	}
	return pb
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
		CapabilityTags: p.GetCapabilityTags(),
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
	extConfig, _ := StructPBToMap(p.GetExtConfig())
	safetyTh, _ := StructPBToMap(p.GetSafetyThresholds())
	dataType, _ := PointDataTypeProtoToDomain(p.GetDataType())
	return types.PointItem{
		PointKey:         p.GetPointKey(),
		ExternalAddress:  p.GetExternalAddress(),
		DataType:         dataType,
		ExtConfig:        extConfig,
		Description:      p.GetDescription(),
		ControlFlag:      p.GetControlFlag(),
		IsVirtual:        p.GetIsVirtual(),
		SafetyThresholds: safetyTh,
		CacheKeyAlias:    p.GetCacheKeyAlias(),
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

func logIn(method string, request any) {
	logrus.Infof("resource_grpc||%s||request_in||request=%v", method, request)
}
