package grpc

import (
	"context"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreateSite(ctx context.Context, req *resourcepb.CreateSiteRequest) (*resourcepb.CreateSiteResponse, error) {
	logIn("create_site", req)

	res, err := s.createSite.Handle(ctx, command.CreateSite{
		TenantID:    req.GetTenantID(),
		Name:        req.GetName(),
		Location:    LocationProtoToDomain(req.GetLocation()),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.CreateSiteResponse{SiteID: res.SiteID}, nil
}

func (s *Server) GetSite(ctx context.Context, req *resourcepb.GetSiteRequest) (*resourcepb.Site, error) {
	logIn("get_site", req)

	site, err := s.getSite.Handle(ctx, query.GetSite{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return SiteDomainToProto(site), nil
}

func (s *Server) ListSites(ctx context.Context, req *resourcepb.ListSitesRequest) (*resourcepb.ListSitesResponse, error) {
	logIn("list_sites", req)

	result, err := s.listSites.Handle(ctx, query.ListSites{
		TenantID: req.GetTenantID(),
		IDs:      req.GetIDs(),
		Status:   req.GetStatus(),
		NameLike: req.GetNameLike(),
		Offset:   int(req.GetOffset()),
		Limit:    int(req.GetLimit()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	sites := make([]*resourcepb.Site, 0, len(result.Sites))
	for _, item := range result.Sites {
		sites = append(sites, SiteDomainToProto(item))
	}
	return &resourcepb.ListSitesResponse{Sites: sites}, nil
}

func (s *Server) UpdateSite(ctx context.Context, req *resourcepb.UpdateSiteRequest) (*emptypb.Empty, error) {
	logIn("update_site", req)

	_, err := s.updateSite.Handle(ctx, command.UpdateSite{
		TenantID:    req.GetTenantID(),
		ID:          req.GetID(),
		Name:        req.GetName(),
		Location:    LocationProtoToDomain(req.GetLocation()),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) DeleteSite(ctx context.Context, req *resourcepb.DeleteSiteRequest) (*emptypb.Empty, error) {
	logIn("delete_site", req)

	_, err := s.deleteSite.Handle(ctx, command.DeleteSite{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}
