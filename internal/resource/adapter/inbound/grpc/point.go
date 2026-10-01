package grpc

import (
	"context"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreatePoint(ctx context.Context, req *resourcepb.CreatePointRequest) (*resourcepb.CreatePointResponse, error) {
	logIn(ctx, "create_point")

	accessMode, err := PointAccessModeProtoToDomain(req.GetAccessMode())
	if err != nil {
		return nil, toGRPCError(err)
	}
	scale := 1.0
	if req.GetScale() != nil {
		scale = req.GetScale().GetValue()
	}
	offset := 0.0
	if req.GetOffset() != nil {
		offset = req.GetOffset().GetValue()
	}

	res, err := s.createPoint.Handle(ctx, command.CreatePoint{
		TenantID:         req.GetTenantID(),
		AssetID:          req.GetAssetID(),
		CUID:             req.GetCUID(),
		MetricID:         req.GetMetricID(),
		ExternalAddress:  req.GetExternalAddress(),
		AccessMode:       accessMode,
		Scale:            scale,
		Offset:           offset,
		Enabled:          req.GetEnabled(),
		SafetyConstraint: PointSafetyConstraintProtoToDomain(req.GetSafetyConstraint()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.CreatePointResponse{PointID: res.PointID}, nil
}

func (s *Server) GetPoint(ctx context.Context, req *resourcepb.GetPointRequest) (*resourcepb.Point, error) {
	logIn(ctx, "get_point")

	p, err := s.getPoint.Handle(ctx, query.GetPoint{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	out, err := PointToProto(p.Point)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return out, nil
}

func (s *Server) ListPoints(ctx context.Context, req *resourcepb.ListPointsRequest) (*resourcepb.ListPointsResponse, error) {
	logIn(ctx, "list_points")

	var enabled *bool
	if v := req.GetEnabled(); v != nil {
		val := v.GetValue()
		enabled = &val
	}
	accessModes := make([]string, 0, len(req.GetAccessModes()))
	for _, mode := range req.GetAccessModes() {
		domainMode, err := PointAccessModeProtoToDomain(mode)
		if err != nil {
			return nil, toGRPCError(err)
		}
		accessModes = append(accessModes, string(domainMode))
	}

	result, err := s.listPoints.Handle(ctx, query.ListPoints{
		TenantID:    req.GetTenantID(),
		SiteID:      req.GetSiteID(),
		CUID:        req.GetCUID(),
		MetricIDs:   req.GetMetricIDs(),
		AccessModes: accessModes,
		Enabled:     enabled,
		IDs:         req.GetIDs(),
		Offset:      int(req.GetOffset()),
		Limit:       int(req.GetLimit()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	points := make([]*resourcepb.Point, 0, len(result.Items))
	for _, item := range result.Items {
		pb, err := PointToProto(item.Point)
		if err != nil {
			return nil, toGRPCError(err)
		}
		points = append(points, pb)
	}
	return &resourcepb.ListPointsResponse{Points: points}, nil
}

func (s *Server) UpdatePoint(ctx context.Context, req *resourcepb.UpdatePointRequest) (*emptypb.Empty, error) {
	logIn(ctx, "update_point")

	accessMode, err := PointAccessModeProtoToDomain(req.GetAccessMode())
	if err != nil {
		return nil, toGRPCError(err)
	}
	scale := 1.0
	if req.GetScale() != nil {
		scale = req.GetScale().GetValue()
	}
	offset := 0.0
	if req.GetOffset() != nil {
		offset = req.GetOffset().GetValue()
	}

	_, err = s.updatePoint.Handle(ctx, command.UpdatePoint{
		TenantID:         req.GetTenantID(),
		ID:               req.GetID(),
		MetricID:         req.GetMetricID(),
		ExternalAddress:  req.GetExternalAddress(),
		AccessMode:       accessMode,
		Scale:            scale,
		Offset:           offset,
		Enabled:          req.GetEnabled(),
		SafetyConstraint: PointSafetyConstraintProtoToDomain(req.GetSafetyConstraint()),
		ExpectedRevision: req.GetExpectedRevision(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) DeletePoint(ctx context.Context, req *resourcepb.DeletePointRequest) (*emptypb.Empty, error) {
	logIn(ctx, "delete_point")

	_, err := s.deletePoint.Handle(ctx, command.DeletePoint{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}
