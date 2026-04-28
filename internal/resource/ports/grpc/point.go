package grpc

import (
	"context"
	"fmt"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreatePoint(ctx context.Context, req *resourcepb.CreatePointRequest) (*resourcepb.CreatePointResponse, error) {
	logIn("create_point", req)

	dataType, err := PointDataTypeProtoToDomain(req.GetDataType())
	if err != nil {
		return nil, toGRPCError(err)
	}

	ext := map[string]any(nil)
	if req.GetExtConfig() != nil {
		ext = req.GetExtConfig().AsMap()
	}
	thresholds := map[string]any(nil)
	if req.GetSafetyThresholds() != nil {
		thresholds = req.GetSafetyThresholds().AsMap()
	}

	res, err := s.createPoint.Handle(ctx, command.CreatePoint{
		ResourceID:       req.GetResourceID(),
		CUID:             req.GetCUID(),
		PointKey:         req.GetPointKey(),
		ExternalAddress:  req.GetExternalAddress(),
		DataType:         dataType,
		ExtConfig:        ext,
		Description:      req.GetDescription(),
		ControlFlag:      req.GetControlFlag(),
		IsVirtual:        req.GetIsVirtual(),
		SafetyThresholds: thresholds,
		CacheKeyAlias:    req.GetCacheKeyAlias(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.CreatePointResponse{PointID: res.PointID}, nil
}

func (s *Server) GetPoint(ctx context.Context, req *resourcepb.GetPointRequest) (*resourcepb.Point, error) {
	logIn("get_point", req)

	p, err := s.getPoint.Handle(ctx, query.GetPoint{ID: req.GetID()})
	if err != nil {
		return nil, toGRPCError(err)
	}
	out, err := PointDomainToProto(p)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return out, nil
}

func (s *Server) ListPoints(ctx context.Context, req *resourcepb.ListPointsRequest) (*resourcepb.ListPointsResponse, error) {
	logIn("list_points", req)

	var isVirtual *bool
	if v := req.GetIsVirtual(); v != nil {
		val := v.GetValue()
		isVirtual = &val
	}

	result, err := s.listPoints.Handle(ctx, query.ListPoints{
		TenantID:  req.GetTenantID(),
		SiteID:    req.GetSiteID(),
		CUID:      req.GetCUID(),
		PointKeys: req.GetPointKeys(),
		IsVirtual: isVirtual,
		DataTypes: req.GetDataTypes(),
		IDs:       req.GetIDs(),
		Offset:    int(req.GetOffset()),
		Limit:     int(req.GetLimit()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	points := make([]*resourcepb.Point, 0, len(result.Points))
	for _, item := range result.Points {
		pb, err := PointDomainToProto(item)
		if err != nil {
			return nil, toGRPCError(err)
		}
		points = append(points, pb)
	}
	return &resourcepb.ListPointsResponse{Points: points}, nil
}

func (s *Server) UpdatePoint(ctx context.Context, req *resourcepb.UpdatePointRequest) (*emptypb.Empty, error) {
	logIn("update_point", req)

	dataType, err := PointDataTypeProtoToDomain(req.GetDataType())
	if err != nil {
		return nil, toGRPCError(err)
	}

	ext := map[string]any(nil)
	if req.GetExtConfig() != nil {
		ext = req.GetExtConfig().AsMap()
	}
	thresholds := map[string]any(nil)
	if req.GetSafetyThresholds() != nil {
		thresholds = req.GetSafetyThresholds().AsMap()
	}

	_, err = s.updatePoint.Handle(ctx, command.UpdatePoint{
		ID:               req.GetID(),
		PointKey:         req.GetPointKey(),
		ExternalAddress:  req.GetExternalAddress(),
		DataType:         dataType,
		ExtConfig:        ext,
		Description:      req.GetDescription(),
		ControlFlag:      req.GetControlFlag(),
		IsVirtual:        req.GetIsVirtual(),
		SafetyThresholds: thresholds,
		CacheKeyAlias:    req.GetCacheKeyAlias(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) DeletePoint(ctx context.Context, req *resourcepb.DeletePointRequest) (*emptypb.Empty, error) {
	logIn("delete_point", req)

	_, err := s.deletePoint.Handle(ctx, command.DeletePoint{ID: req.GetID()})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) BatchCreatePoints(ctx context.Context, req *resourcepb.BatchCreatePointsRequest) (*resourcepb.BatchCreatePointsResponse, error) {
	logIn("batch_create_points", req)

	total := len(req.GetItems())
	if total == 0 {
		return &resourcepb.BatchCreatePointsResponse{
			PointIDs:     nil,
			FailedItems:  nil,
			TotalCount:   0,
			SuccessCount: 0,
		}, nil
	}

	batchSize := int(req.GetBatchSize())
	if batchSize <= 0 {
		batchSize = 100
	}

	var (
		createdIDs []string
		failed     []*resourcepb.BatchItemError
		validBatch []*model.Point
	)

	flush := func() error {
		if len(validBatch) == 0 {
			return nil
		}
		if err := s.pointRepo.BatchCreate(ctx, validBatch); err != nil {
			return fmt.Errorf("batch insert: %w", err)
		}
		validBatch = validBatch[:0]
		return nil
	}

	for idx, item := range req.GetItems() {
		if item == nil {
			failed = append(failed, &resourcepb.BatchItemError{
				Index:  int32(idx),
				Name:   "",
				Reason: "item is nil",
			})
			continue
		}
		if item.GetPointKey() == "" {
			failed = append(failed, &resourcepb.BatchItemError{
				Index:  int32(idx),
				Name:   "",
				Reason: "PointKey is required",
			})
			continue
		}

		dataType, err := PointDataTypeProtoToDomain(item.GetDataType())
		if err != nil {
			failed = append(failed, &resourcepb.BatchItemError{
				Index:  int32(idx),
				Name:   item.GetPointKey(),
				Reason: err.Error(),
			})
			continue
		}

		ext := map[string]any(nil)
		if item.GetExtConfig() != nil {
			ext = item.GetExtConfig().AsMap()
		}
		thresholds := map[string]any(nil)
		if item.GetSafetyThresholds() != nil {
			thresholds = item.GetSafetyThresholds().AsMap()
		}

		id := idgen.Must()
		p, err := model.NewPoint(
			id,
			req.GetResourceID(),
			req.GetCUID(),
			item.GetPointKey(),
			item.GetExternalAddress(),
			dataType,
			ext,
			item.GetDescription(),
			item.GetControlFlag(),
			item.GetIsVirtual(),
			thresholds,
			item.GetCacheKeyAlias(),
		)
		if err != nil {
			failed = append(failed, &resourcepb.BatchItemError{
				Index:  int32(idx),
				Name:   item.GetPointKey(),
				Reason: err.Error(),
			})
			continue
		}

		createdIDs = append(createdIDs, id)
		validBatch = append(validBatch, p)
		if len(validBatch) >= batchSize {
			if err := flush(); err != nil {
				return nil, toGRPCError(err)
			}
		}
	}

	if err := flush(); err != nil {
		return nil, toGRPCError(err)
	}

	return &resourcepb.BatchCreatePointsResponse{
		PointIDs:     createdIDs,
		FailedItems:  failed,
		TotalCount:   int32(total),
		SuccessCount: int32(len(createdIDs)),
	}, nil
}
