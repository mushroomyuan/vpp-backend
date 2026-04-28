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

func (s *Server) CreateCU(ctx context.Context, req *resourcepb.CreateCURequest) (*resourcepb.CreateCUResponse, error) {
	logIn("create_cu", req)

	meta := map[string]any(nil)
	if req.GetMetadata() != nil {
		meta = req.GetMetadata().AsMap()
	}

	res, err := s.createCU.Handle(ctx, command.CreateCU{
		ResourceID:     req.GetResourceID(),
		ParentCUID:     req.GetParentCUID(),
		Name:           req.GetName(),
		Type:           req.GetType(),
		CapabilityTags: req.GetCapabilityTags(),
		Metadata:       meta,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.CreateCUResponse{CUID: res.CUID}, nil
}

func (s *Server) GetCU(ctx context.Context, req *resourcepb.GetCURequest) (*resourcepb.CU, error) {
	logIn("get_cu", req)

	cu, err := s.getCU.Handle(ctx, query.GetCU{ID: req.GetID()})
	if err != nil {
		return nil, toGRPCError(err)
	}

	out, err := CUDomainToProto(cu)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return out, nil
}

func (s *Server) ListCUs(ctx context.Context, req *resourcepb.ListCUsRequest) (*resourcepb.ListCUsResponse, error) {
	logIn("list_cus", req)

	result, err := s.listCUs.Handle(ctx, query.ListCUs{
		TenantID:   req.GetTenantID(),
		SiteID:     req.GetSiteID(),
		ResourceID: req.GetResourceID(),
		ParentCUID: req.GetParentCUID(),
		Capability: req.GetCapability(),
		IDs:        req.GetIDs(),
		NameLike:   req.GetNameLike(),
		Offset:     int(req.GetOffset()),
		Limit:      int(req.GetLimit()),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	out := make([]*resourcepb.CU, 0, len(result.CUs))
	for _, item := range result.CUs {
		pb, err := CUDomainToProto(item)
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

	_, err := s.updateCU.Handle(ctx, command.UpdateCU{
		ID:             req.GetID(),
		ParentCUID:     req.GetParentCUID(),
		Name:           req.GetName(),
		Type:           req.GetType(),
		CapabilityTags: req.GetCapabilityTags(),
		Metadata:       meta,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) DeleteCU(ctx context.Context, req *resourcepb.DeleteCURequest) (*emptypb.Empty, error) {
	logIn("delete_cu", req)

	_, err := s.deleteCU.Handle(ctx, command.DeleteCU{ID: req.GetID()})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) BatchCreateCUs(ctx context.Context, req *resourcepb.BatchCreateCUsRequest) (*resourcepb.BatchCreateCUsResponse, error) {
	logIn("batch_create_cus", req)

	total := len(req.GetItems())
	if total == 0 {
		return &resourcepb.BatchCreateCUsResponse{
			CUIDs:        nil,
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
		validBatch []*model.CU
	)

	flush := func() error {
		if len(validBatch) == 0 {
			return nil
		}
		if err := s.cuRepo.BatchCreate(ctx, validBatch); err != nil {
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
		cu, err := model.NewCU(
			id,
			req.GetResourceID(),
			item.GetParentCUID(),
			item.GetName(),
			item.GetType(),
			item.GetCapabilityTags(),
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
		validBatch = append(validBatch, cu)

		if len(validBatch) >= batchSize {
			if err := flush(); err != nil {
				return nil, toGRPCError(err)
			}
		}
	}

	if err := flush(); err != nil {
		return nil, toGRPCError(err)
	}

	return &resourcepb.BatchCreateCUsResponse{
		CUIDs:        createdIDs,
		FailedItems:  failed,
		TotalCount:   int32(total),
		SuccessCount: int32(len(createdIDs)),
	}, nil
}
