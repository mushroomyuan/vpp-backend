package grpc

import (
	"context"
	"errors"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) GetJob(ctx context.Context, req *resourcepb.GetJobRequest) (*resourcepb.Job, error) {
	logIn("get_import_job", req)

	job, err := s.getJob.Handle(ctx, query.GetJob{JobID: req.GetID()})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return JobDomainToProto(job), nil
}

func (s *Server) SubmitBatchImport(ctx context.Context, req *resourcepb.SubmitBatchRequest) (*resourcepb.SubmitBatchResponse, error) {
	logIn("submit_batch_import", req)

	cmd, convErr := submitBatchImportProtoToCmd(req)
	if convErr != nil {
		return nil, toGRPCError(convErr)
	}

	result, err := s.submitBatchImport.Handle(ctx, cmd)
	if err != nil && !errors.Is(err, types.ErrBatchImportValidation) {
		return nil, toGRPCError(err)
	}

	resp := &resourcepb.SubmitBatchResponse{JobId: result.JobID}
	for _, fe := range result.FailedItems {
		resp.FailedItems = append(resp.FailedItems, BatchItemErrorDomainToProto(fe))
	}
	return resp, nil
}

func (s *Server) SubmitBatchDelete(ctx context.Context, req *resourcepb.SubmitBatchDeleteRequest) (*resourcepb.SubmitBatchResponse, error) {
	logIn("submit_batch_delete", req)

	cmd, convErr := submitBatchDeleteProtoToCmd(req)
	if convErr != nil {
		return nil, toGRPCError(convErr)
	}

	result, err := s.submitBatchDelete.Handle(ctx, cmd)
	if err != nil && !errors.Is(err, types.ErrBatchDeleteValidation) {
		return nil, toGRPCError(err)
	}

	resp := &resourcepb.SubmitBatchResponse{JobId: result.JobID}
	for _, fe := range result.FailedItems {
		resp.FailedItems = append(resp.FailedItems, BatchItemErrorDomainToProto(fe))
	}
	return resp, nil
}

// submitBatchDeleteProtoToCmd maps the proto oneof batch to SubmitBatchDelete.
func submitBatchDeleteProtoToCmd(req *resourcepb.SubmitBatchDeleteRequest) (command.SubmitBatchDelete, error) {
	batchSize := int(req.GetBatchSize())
	var cmd command.SubmitBatchDelete
	cmd.BatchSize = batchSize

	switch b := req.Batch.(type) {
	case *resourcepb.SubmitBatchDeleteRequest_Resource:
		rb := b.Resource
		cmd.Resource = &types.ResourceDeleteSpec{
			TenantID: rb.GetTenantID(),
			IDs:      append([]string(nil), rb.GetIds()...),
		}
	case *resourcepb.SubmitBatchDeleteRequest_Cu:
		cb := b.Cu
		cmd.CU = &types.CUDeleteSpec{
			TenantID: cb.GetTenantID(),
			IDs:      append([]string(nil), cb.GetIds()...),
		}
	case *resourcepb.SubmitBatchDeleteRequest_Point:
		pb := b.Point
		cmd.Point = &types.PointDeleteSpec{
			TenantID: pb.GetTenantID(),
			IDs:      append([]string(nil), pb.GetIds()...),
		}
	default:
		return cmd, errors.New("SubmitBatchDelete: batch field is required (resource, cu, or point)")
	}

	return cmd, nil
}

// submitBatchImportProtoToCmd maps the proto oneof batch to SubmitBatchImport.
func submitBatchImportProtoToCmd(req *resourcepb.SubmitBatchRequest) (command.SubmitBatchImport, error) {
	batchSize := int(req.GetBatchSize())
	var cmd command.SubmitBatchImport

	switch b := req.Batch.(type) {
	case *resourcepb.SubmitBatchRequest_Resource:
		rb := b.Resource
		items := make([]types.ResourceItem, 0, len(rb.GetItems()))
		for _, it := range rb.GetItems() {
			items = append(items, ResourceImportItemProtoToCommand(it))
		}
		cmd.Resource = &types.ResourceImportSpec{
			TenantID: rb.GetTenantID(),
			ResourceImportPayload: types.ResourceImportPayload{
				SiteID:    rb.GetSiteID(),
				BatchSize: batchSize,
				Items:     items,
			},
		}

	case *resourcepb.SubmitBatchRequest_Cu:
		cb := b.Cu
		items := make([]types.CUItem, 0, len(cb.GetItems()))
		for _, it := range cb.GetItems() {
			items = append(items, CUImportItemProtoToCommand(it))
		}
		cmd.CU = &types.CUImportSpec{
			TenantID: cb.GetTenantID(),
			CUImportPayload: types.CUImportPayload{
				ResourceID: cb.GetResourceID(),
				BatchSize:  batchSize,
				Items:      items,
			},
		}

	case *resourcepb.SubmitBatchRequest_Point:
		pb := b.Point
		items := make([]types.PointItem, 0, len(pb.GetItems()))
		for _, it := range pb.GetItems() {
			items = append(items, PointImportItemProtoToCommand(it))
		}
		cmd.Point = &types.PointImportSpec{
			TenantID: pb.GetTenantID(),
			PointImportPayload: types.PointImportPayload{
				ResourceID: pb.GetResourceID(),
				CUID:       pb.GetCUID(),
				BatchSize:  batchSize,
				Items:      items,
			},
		}

	default:
		return cmd, errors.New("SubmitBatchImport: batch field is required (resource, cu, or point)")
	}

	return cmd, nil
}

func (s *Server) RetryJob(ctx context.Context, req *resourcepb.RetryJobRequest) (*emptypb.Empty, error) {
	logIn("retry_import_job", req)

	if _, err := s.retryJob.Handle(ctx, command.RetryJob{JobID: req.GetID()}); err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}
