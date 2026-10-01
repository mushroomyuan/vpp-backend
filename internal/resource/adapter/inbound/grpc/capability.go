package grpc

import (
	"context"
	"encoding/json"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

func (s *Server) CreateCUCapability(
	ctx context.Context,
	req *resourcepb.CreateCUCapabilityRequest,
) (*resourcepb.CreateCUCapabilityResponse, error) {
	spec, _ := StructPBToMap(req.GetSpec())
	result, err := s.createCUCapability.Handle(ctx, command.CreateCUCapability{
		TenantID:      req.GetTenantID(),
		CUID:          req.GetCUID(),
		CapabilityID:  req.GetCapabilityID(),
		SchemaVersion: int(req.GetSchemaVersion()),
		Spec:          spec,
		Enabled:       req.GetEnabled(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.CreateCUCapabilityResponse{
		CapabilityInstanceID: result.CapabilityInstanceID,
	}, nil
}

func (s *Server) GetCUCapability(
	ctx context.Context,
	req *resourcepb.GetCUCapabilityRequest,
) (*resourcepb.CUCapability, error) {
	result, err := s.getCUCapability.Handle(ctx, query.GetCUCapability{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return CUCapabilityToProto(result)
}

func (s *Server) ListCUCapabilities(
	ctx context.Context,
	req *resourcepb.ListCUCapabilitiesRequest,
) (*resourcepb.ListCUCapabilitiesResponse, error) {
	result, err := s.listCUCapabilities.Handle(ctx, query.ListCUCapabilities{
		TenantID:      req.GetTenantID(),
		CUID:          req.GetCUID(),
		CapabilityIDs: req.GetCapabilityIDs(),
		EnabledOnly:   req.GetEnabledOnly(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	items := make([]*resourcepb.CUCapability, 0, len(result))
	for _, capability := range result {
		item, err := CUCapabilityToProto(capability)
		if err != nil {
			return nil, toGRPCError(err)
		}
		items = append(items, item)
	}
	return &resourcepb.ListCUCapabilitiesResponse{Capabilities: items}, nil
}

func (s *Server) UpdateCUCapability(
	ctx context.Context,
	req *resourcepb.UpdateCUCapabilityRequest,
) (*emptypb.Empty, error) {
	spec, _ := StructPBToMap(req.GetSpec())
	_, err := s.updateCUCapability.Handle(ctx, command.UpdateCUCapability{
		TenantID:        req.GetTenantID(),
		ID:              req.GetID(),
		SchemaVersion:   int(req.GetSchemaVersion()),
		Spec:            spec,
		Enabled:         req.GetEnabled(),
		ExpectedVersion: req.GetExpectedVersion(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) DeleteCUCapability(
	ctx context.Context,
	req *resourcepb.DeleteCUCapabilityRequest,
) (*emptypb.Empty, error) {
	_, err := s.deleteCUCapability.Handle(ctx, command.DeleteCUCapability{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func CUCapabilityToProto(c *model.CUCapability) (*resourcepb.CUCapability, error) {
	var specMap map[string]any
	if err := json.Unmarshal(c.Spec, &specMap); err != nil {
		return nil, err
	}
	spec, err := structpb.NewStruct(specMap)
	if err != nil {
		return nil, err
	}
	return &resourcepb.CUCapability{
		ID:            c.ID,
		TenantID:      c.TenantID,
		CUID:          c.CUID,
		CapabilityID:  string(c.CapabilityID),
		SchemaVersion: int32(c.SchemaVersion),
		Spec:          spec,
		Enabled:       c.Enabled,
		Version:       c.Version,
	}, nil
}
