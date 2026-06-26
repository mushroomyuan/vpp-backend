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

// submitBatchImportProtoToCmd maps the proto oneof batch to SubmitBatchImport.
func submitBatchImportProtoToCmd(req *resourcepb.SubmitBatchRequest) (command.SubmitBatchImport, error) {
	batchSize := int(req.GetBatchSize())
	var cmd command.SubmitBatchImport

	switch b := req.Batch.(type) {
	case *resourcepb.SubmitBatchRequest_Asset:
		ab := b.Asset
		items := make([]types.AssetItem, 0, len(ab.GetItems()))
		for _, it := range ab.GetItems() {
			items = append(items, AssetImportItemProtoToCommand(it))
		}
		cmd.Asset = &types.AssetImportSpec{
			TenantID: ab.GetTenantID(),
			AssetImportPayload: types.AssetImportPayload{
				SiteID:    ab.GetSiteID(),
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
				AssetID:   cb.GetAssetID(),
				BatchSize: batchSize,
				Items:     items,
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
				AssetID:   pb.GetAssetID(),
				CUID:      pb.GetCUID(),
				BatchSize: batchSize,
				Items:     items,
			},
		}

	default:
		return cmd, errors.New("SubmitBatchImport: batch field is required (asset, cu, or point)")
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
