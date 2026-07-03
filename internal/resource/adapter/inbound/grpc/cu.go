package grpc

import (
	"context"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreateCU(ctx context.Context, req *resourcepb.CreateCURequest) (*resourcepb.CreateCUResponse, error) {
	logIn("create_cu", req)

	meta := map[string]any(nil)
	if req.GetMetadata() != nil {
		meta = req.GetMetadata().AsMap()
	}
	var parentID *string
	if v := req.GetParentID(); v != "" {
		parentID = &v
	}
	var protocol *string
	if v := req.GetProtocol(); v != "" {
		protocol = &v
	}
	var provider *string
	if v := req.GetProvider(); v != "" {
		provider = &v
	}
	var externalID *string
	if v := req.GetExternalID(); v != "" {
		externalID = &v
	}
	var description *string
	if v := req.GetDescription(); v != "" {
		description = &v
	}
	protocolConfig := map[string]any(nil)
	if req.GetProtocolConfig() != nil {
		protocolConfig = req.GetProtocolConfig().AsMap()
	}
	conn, err := ConnectionProtoToDomain(req.GetConnection())
	if err != nil {
		return nil, toGRPCError(err)
	}

	res, err := s.createCU.Handle(ctx, command.CreateCU{
		TenantID:       req.GetTenantID(),
		ParentID:       parentID,
		Name:           req.GetName(),
		Type:           req.GetType(),
		Description:    description,
		CapabilityTags: req.GetCapabilityTags(),
		Provider:       provider,
		ExternalID:     externalID,
		Protocol:       protocol,
		ProtocolConfig: protocolConfig,
		Connection:     conn,
		Metadata:       meta,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.CreateCUResponse{CUID: res.CUID}, nil
}

func (s *Server) GetCU(ctx context.Context, req *resourcepb.GetCURequest) (*resourcepb.CU, error) {
	logIn("get_cu", req)

	cu, err := s.getCU.Handle(ctx, query.GetCU{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	out, err := CUToProto(cu.CU, cu.Runtime)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return out, nil
}

func (s *Server) ListCUs(ctx context.Context, req *resourcepb.ListCUsRequest) (*resourcepb.ListCUsResponse, error) {
	logIn("list_cus", req)

	result, err := s.listCUs.Handle(ctx, query.ListCUs{
		TenantID:       req.GetTenantID(),
		SiteID:         req.GetSiteID(),
		AssetID:        req.GetAssetID(),
		CapabilityTags: req.GetCapability(),
		IDs:            req.GetIDs(),
		NameLike:       req.GetNameLike(),
		Offset:         int(req.GetOffset()),
		Limit:          int(req.GetLimit()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	out := make([]*resourcepb.CU, 0, len(result.Items))
	for _, item := range result.Items {
		pb, err := CUToProto(item.CU, item.Runtime)
		if err != nil {
			return nil, toGRPCError(err)
		}
		out = append(out, pb)
	}
	return &resourcepb.ListCUsResponse{CUs: out}, nil
}

func (s *Server) UpdateCU(ctx context.Context, req *resourcepb.UpdateCURequest) (*emptypb.Empty, error) {
	logIn("update_cu", req)

	meta := map[string]any(nil)
	if req.GetMetadata() != nil {
		meta = req.GetMetadata().AsMap()
	}
	protocolConfig := map[string]any(nil)
	if req.GetProtocolConfig() != nil {
		protocolConfig = req.GetProtocolConfig().AsMap()
	}
	var protocol *string
	if v := req.GetProtocol(); v != "" {
		protocol = &v
	}
	var provider *string
	if v := req.GetProvider(); v != "" {
		provider = &v
	}
	var externalID *string
	if v := req.GetExternalID(); v != "" {
		externalID = &v
	}
	conn, err := ConnectionProtoToDomain(req.GetConnection())
	if err != nil {
		return nil, toGRPCError(err)
	}

	_, err = s.updateCU.Handle(ctx, command.UpdateCU{
		TenantID:       req.GetTenantID(),
		ID:             req.GetID(),
		Name:           req.GetName(),
		Type:           req.GetType(),
		CapabilityTags: req.GetCapabilityTags(),
		Provider:       provider,
		ExternalID:     externalID,
		Protocol:       protocol,
		ProtocolConfig: protocolConfig,
		Connection:     conn,
		Metadata:       meta,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}
