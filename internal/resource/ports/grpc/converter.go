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

func SiteStatusProtoToDomain(s resourcepb.SiteStatus) model.SiteStatus {
	switch s {
	case resourcepb.SiteStatus_SITE_STATUS_UNDER_CONSTRUCTION:
		return model.SiteStatusUnderConstruction
	case resourcepb.SiteStatus_SITE_STATUS_OPERATING:
		return model.SiteStatusOperating
	case resourcepb.SiteStatus_SITE_STATUS_FAULT:
		return model.SiteStatusFault
	case resourcepb.SiteStatus_SITE_STATUS_OFFLINE:
		return model.SiteStatusOffline
	default:
		return model.SiteStatusUnknown
	}
}

func SiteStatusDomainToProto(s model.SiteStatus) resourcepb.SiteStatus {
	switch s {
	case model.SiteStatusUnderConstruction:
		return resourcepb.SiteStatus_SITE_STATUS_UNDER_CONSTRUCTION
	case model.SiteStatusOperating:
		return resourcepb.SiteStatus_SITE_STATUS_OPERATING
	case model.SiteStatusFault:
		return resourcepb.SiteStatus_SITE_STATUS_FAULT
	case model.SiteStatusOffline:
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
	return &resourcepb.Site{
		ID:       s.ID,
		TenantID: s.TenantID,
		Name:     s.Name,
		Location: &resourcepb.Location{
			Latitude:  s.Location.Latitude,
			Longitude: s.Location.Longitude,
			Address:   s.Location.Address,
		},
		Description: s.Description,
		Status:      SiteStatusDomainToProto(s.Status),
	}
}

func ResourceDomainToProto(r *model.Resource) (*resourcepb.Resource, error) {
	if r == nil {
		return nil, nil
	}
	meta, err := MapToStructPB(r.Metadata)
	if err != nil {
		return nil, err
	}
	return &resourcepb.Resource{
		ID:           r.ID,
		TenantID:     r.TenantID,
		SiteID:       r.SiteID,
		Name:         r.Name,
		Type:         r.Type,
		Capacity:     r.Capacity,
		Manufacturer: r.Manufacturer,
		Model:        r.Model,
		Metadata:     meta,
	}, nil
}

func CUDomainToProto(cu *model.CU) (*resourcepb.CU, error) {
	if cu == nil {
		return nil, nil
	}
	meta, err := MapToStructPB(cu.Metadata)
	if err != nil {
		return nil, err
	}
	return &resourcepb.CU{
		ID:             cu.ID,
		ResourceID:     cu.ResourceID,
		ParentCUID:     cu.ParentCUID,
		Name:           cu.Name,
		Type:           cu.Type,
		CapabilityTags: cu.CapabilityTags,
		Metadata:       meta,
	}, nil
}

func PointDomainToProto(p *model.Point) (*resourcepb.Point, error) {
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
		ResourceID:       p.ResourceID,
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
	}, nil
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
		case model.JobTargetResource:
			jobType = resourcepb.JobType_IMPORT_JOB_TYPE_RESOURCE
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

// ResourceImportItemProtoToCommand converts a proto ResourceImportItem to the
// domain types.ResourceItem used by SubmitBatchImport.
func ResourceImportItemProtoToCommand(p *resourcepb.ResourceItem) types.ResourceItem {
	if p == nil {
		return types.ResourceItem{}
	}
	meta, _ := StructPBToMap(p.GetMetadata())
	return types.ResourceItem{
		Name:         p.GetName(),
		Type:         p.GetType(),
		Capacity:     p.GetCapacity(),
		Manufacturer: p.GetManufacturer(),
		Model:        p.GetModel(),
		Metadata:     meta,
	}
}

// CUImportItemProtoToCommand converts a proto CUImportItem to types.CUItem.
func CUImportItemProtoToCommand(p *resourcepb.CUItem) types.CUItem {
	if p == nil {
		return types.CUItem{}
	}
	meta, _ := StructPBToMap(p.GetMetadata())
	return types.CUItem{
		ParentCUID:     p.GetParentCUID(),
		Name:           p.GetName(),
		Type:           p.GetType(),
		CapabilityTags: p.GetCapabilityTags(),
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
