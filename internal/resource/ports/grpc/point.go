package grpc

import (
	"context"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
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
		TenantID:         req.GetTenantID(),
		AssetID:          req.GetAssetID(),
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

	p, err := s.getPoint.Handle(ctx, query.GetPoint{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	out, err := PointToProto(p.Point, p.Runtime)
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

	points := make([]*resourcepb.Point, 0, len(result.Items))
	for _, item := range result.Items {
		pb, err := PointToProto(item.Point, item.Runtime)
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
		TenantID:         req.GetTenantID(),
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

	_, err := s.deletePoint.Handle(ctx, command.DeletePoint{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}
