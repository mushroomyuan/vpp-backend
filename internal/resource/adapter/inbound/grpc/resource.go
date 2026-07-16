package grpc

import (
	"context"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) BatchMoveResources(ctx context.Context, req *resourcepb.BatchMoveResourcesRequest) (*resourcepb.BatchMoveResourcesResponse, error) {
	logIn(ctx, "batch_move_resources")

	res, err := s.batchMoveResource.Handle(ctx, command.BatchMoveResources{
		TenantID:    req.GetTenantID(),
		ResourceIDs: req.GetResourceIDs(),
		NewParentID: req.GetNewParentID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.BatchMoveResourcesResponse{MovedCount: int32(res.MovedCount)}, nil
}

func (s *Server) RenameResource(ctx context.Context, req *resourcepb.RenameResourceRequest) (*emptypb.Empty, error) {
	logIn(ctx, "rename_resource")

	_, err := s.renameResource.Handle(ctx, command.RenameResource{
		TenantID:   req.GetTenantID(),
		ResourceID: req.GetResourceID(),
		NewName:    req.GetNewName(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ChangeResourceLifecycle(ctx context.Context, req *resourcepb.ChangeResourceLifecycleRequest) (*emptypb.Empty, error) {
	logIn(ctx, "change_resource_lifecycle")

	_, err := s.changeLifecycle.Handle(ctx, command.ChangeResourceLifecycle{
		TenantID:   req.GetTenantID(),
		ResourceID: req.GetResourceID(),
		Status:     ResourceLifecycleStatusProtoToDomain(req.GetStatus()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) GetResourceDetail(ctx context.Context, req *resourcepb.GetResourceDetailRequest) (*resourcepb.Resource, error) {
	logIn(ctx, "get_resource_detail")

	node, err := s.getResourceDetail.Handle(ctx, query.GetResourceDetail{
		TenantID:   req.GetTenantID(),
		ResourceID: req.GetResourceID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	out, err := ResourceDomainToProto(node)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return out, nil
}

func (s *Server) ListChildren(ctx context.Context, req *resourcepb.ListChildrenRequest) (*resourcepb.ListChildrenResponse, error) {
	logIn(ctx, "list_children")

	result, err := s.listChildren.Handle(ctx, query.ListChildren{
		TenantID: req.GetTenantID(),
		ParentID: req.GetParentID(),
		Offset:   int(req.GetOffset()),
		Limit:    int(req.GetLimit()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	items := make([]*resourcepb.Resource, 0, len(result.Items))
	for _, node := range result.Items {
		pb, convErr := ResourceDomainToProto(node)
		if convErr != nil {
			return nil, toGRPCError(convErr)
		}
		items = append(items, pb)
	}
	return &resourcepb.ListChildrenResponse{
		Items:      items,
		TotalCount: int32(result.TotalCount),
		Offset:     int32(result.Offset),
		Limit:      int32(result.Limit),
	}, nil
}

func (s *Server) GetBreadcrumb(ctx context.Context, req *resourcepb.GetBreadcrumbRequest) (*resourcepb.GetBreadcrumbResponse, error) {
	logIn(ctx, "get_breadcrumb")

	result, err := s.getBreadcrumb.Handle(ctx, query.GetBreadcrumb{
		TenantID:   req.GetTenantID(),
		ResourceID: req.GetResourceID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	items := make([]*resourcepb.Resource, 0, len(result.Items))
	for _, node := range result.Items {
		pb, convErr := ResourceDomainToProto(node)
		if convErr != nil {
			return nil, toGRPCError(convErr)
		}
		items = append(items, pb)
	}
	return &resourcepb.GetBreadcrumbResponse{Items: items}, nil
}

func (s *Server) ExportResourceTree(ctx context.Context, req *resourcepb.ExportResourceTreeRequest) (*resourcepb.ExportResourceTreeResponse, error) {
	logIn(ctx, "export_resource_tree")

	result, err := s.exportTree.Handle(ctx, query.ExportResourceTree{
		TenantID: req.GetTenantID(),
		RootID:   req.GetRootID(),
		MaxDepth: int(req.GetMaxDepth()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	items := make([]*resourcepb.Resource, 0, len(result.Items))
	for _, node := range result.Items {
		pb, convErr := ResourceDomainToProto(node)
		if convErr != nil {
			return nil, toGRPCError(convErr)
		}
		items = append(items, pb)
	}
	return &resourcepb.ExportResourceTreeResponse{Items: items}, nil
}
