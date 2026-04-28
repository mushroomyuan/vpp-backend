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

func (s *Server) CreateResource(ctx context.Context, req *resourcepb.CreateResourceRequest) (*resourcepb.CreateResourceResponse, error) {
	logIn("create_resource", req)

	meta := map[string]any(nil)
	if req.GetMetadata() != nil {
		meta = req.GetMetadata().AsMap()
	}

	res, err := s.createResource.Handle(ctx, command.CreateResource{
		TenantID:     req.GetTenantID(),
		SiteID:       req.GetSiteID(),
		Name:         req.GetName(),
		Type:         req.GetType(),
		Capacity:     req.GetCapacity(),
		Manufacturer: req.GetManufacturer(),
		Model:        req.GetModel(),
		Metadata:     meta,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.CreateResourceResponse{ResourceID: res.ResourceID}, nil
}

func (s *Server) GetResource(ctx context.Context, req *resourcepb.GetResourceRequest) (*resourcepb.Resource, error) {
	logIn("get_resource", req)

	r, err := s.getResource.Handle(ctx, query.GetResource{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	out, err := ResourceDomainToProto(r)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return out, nil
}

func (s *Server) ListResources(ctx context.Context, req *resourcepb.ListResourcesRequest) (*resourcepb.ListResourcesResponse, error) {
	logIn("list_resources", req)

	result, err := s.listResources.Handle(ctx, query.ListResources{
		TenantID: req.GetTenantID(),
		SiteID:   req.GetSiteID(),
		IDs:      req.GetIDs(),
		Types:    req.GetTypes(),
		NameLike: req.GetNameLike(),
		Offset:   int(req.GetOffset()),
		Limit:    int(req.GetLimit()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	resources := make([]*resourcepb.Resource, 0, len(result.Resources))
	for _, item := range result.Resources {
		pb, err := ResourceDomainToProto(item)
		if err != nil {
			return nil, toGRPCError(err)
		}
		resources = append(resources, pb)
	}
	return &resourcepb.ListResourcesResponse{Resources: resources}, nil
}

func (s *Server) UpdateResource(ctx context.Context, req *resourcepb.UpdateResourceRequest) (*emptypb.Empty, error) {
	logIn("update_resource", req)

	meta := map[string]any(nil)
	if req.GetMetadata() != nil {
		meta = req.GetMetadata().AsMap()
	}

	_, err := s.updateResource.Handle(ctx, command.UpdateResource{
		TenantID:     req.GetTenantID(),
		ID:           req.GetID(),
		Name:         req.GetName(),
		Type:         req.GetType(),
		Capacity:     req.GetCapacity(),
		Manufacturer: req.GetManufacturer(),
		Model:        req.GetModel(),
		Metadata:     meta,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) DeleteResource(ctx context.Context, req *resourcepb.DeleteResourceRequest) (*emptypb.Empty, error) {
	logIn("delete_resource", req)

	_, err := s.deleteResource.Handle(ctx, command.DeleteResource{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) BatchCreateResources(ctx context.Context, req *resourcepb.BatchCreateResourcesRequest) (*resourcepb.BatchCreateResourcesResponse, error) {
	logIn("batch_create_resources", req)

	total := len(req.GetItems())
	if total == 0 {
		return &resourcepb.BatchCreateResourcesResponse{
			ResourceIDs:  nil,
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
		validBatch []*model.Resource
	)

	flush := func() error {
		if len(validBatch) == 0 {
			return nil
		}
		if err := s.resourceRepo.BatchCreate(ctx, validBatch); err != nil {
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
		if item.GetName() == "" || item.GetType() == "" {
			failed = append(failed, &resourcepb.BatchItemError{
				Index:  int32(idx),
				Name:   item.GetName(),
				Reason: "Name and Type are required",
			})
			continue
		}

		meta := map[string]any(nil)
		if item.GetMetadata() != nil {
			meta = item.GetMetadata().AsMap()
		}

		id := idgen.Must()
		r, err := model.NewResource(
			id,
			req.GetTenantID(),
			req.GetSiteID(),
			item.GetName(),
			item.GetType(),
			item.GetCapacity(),
			item.GetManufacturer(),
			item.GetModel(),
			meta,
		)
		if err != nil {
			failed = append(failed, &resourcepb.BatchItemError{
				Index:  int32(idx),
				Name:   item.GetName(),
				Reason: err.Error(),
			})
			continue
		}

		createdIDs = append(createdIDs, id)
		validBatch = append(validBatch, r)

		if len(validBatch) >= batchSize {
			if err := flush(); err != nil {
				return nil, toGRPCError(err)
			}
		}
	}

	if err := flush(); err != nil {
		return nil, toGRPCError(err)
	}

	return &resourcepb.BatchCreateResourcesResponse{
		ResourceIDs:  createdIDs,
		FailedItems:  failed,
		TotalCount:   int32(total),
		SuccessCount: int32(len(createdIDs)),
	}, nil
}
