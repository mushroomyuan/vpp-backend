package grpc

import (
	"context"
	"strings"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/command"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreateAsset(ctx context.Context, req *resourcepb.CreateAssetRequest) (*resourcepb.CreateAssetResponse, error) {
	logIn("create_asset", req)

	meta := map[string]any(nil)
	if req.GetMetadata() != nil {
		meta = req.GetMetadata().AsMap()
	}

	kw := req.GetRatedCapacityKW()
	kwPtr := &kw

	res, err := s.createAsset.Handle(ctx, command.CreateAsset{
		TenantID:        req.GetTenantID(),
		SiteID:          req.GetSiteID(),
		Name:            req.GetName(),
		DispatchStatus:  model.DispatchStatus(strings.TrimSpace(req.GetDispatchStatus())),
		RatedCapacityKW: kwPtr,
		DispatchMode:    optionalTrimmedStringPtr(req.GetDispatchMode()),
		EnergyType:      optionalTrimmedStringPtr(req.GetEnergyType()),
		OwnerType:       optionalTrimmedStringPtr(req.GetOwnerType()),
		SubType:         optionalTrimmedStringPtr(req.GetSubType()),
		Description:     assetDescriptionPtr(req.GetDescription()),
		MarketEnabled:   assetMarketEnabledPtr(req.GetMarketEnabled()),
		Metadata:        meta,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &resourcepb.CreateAssetResponse{AssetID: res.AssetID}, nil
}

func (s *Server) GetAsset(ctx context.Context, req *resourcepb.GetAssetRequest) (*resourcepb.Asset, error) {
	logIn("get_asset", req)

	a, err := s.getAsset.Handle(ctx, query.GetAsset{
		TenantID: req.GetTenantID(),
		ID:       req.GetID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	out, err := AssetToProto(a.Asset, a.Runtime)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return out, nil
}

func (s *Server) ListAssets(ctx context.Context, req *resourcepb.ListAssetsRequest) (*resourcepb.ListAssetsResponse, error) {
	logIn("list_assets", req)

	result, err := s.listAssets.Handle(ctx, query.ListAssets{
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

	assets := make([]*resourcepb.Asset, 0, len(result.Items))
	for _, item := range result.Items {
		pb, err := AssetToProto(item.Asset, item.Runtime)
		if err != nil {
			return nil, toGRPCError(err)
		}
		assets = append(assets, pb)
	}
	return &resourcepb.ListAssetsResponse{Assets: assets}, nil
}

func (s *Server) UpdateAsset(ctx context.Context, req *resourcepb.UpdateAssetRequest) (*emptypb.Empty, error) {
	logIn("update_asset", req)

	meta := map[string]any(nil)
	if req.GetMetadata() != nil {
		meta = req.GetMetadata().AsMap()
	}

	kw := req.GetRatedCapacityKW()
	kwPtr := &kw

	d := strings.TrimSpace(req.GetDescription())
	descUpdate := &d

	_, err := s.updateAsset.Handle(ctx, command.UpdateAsset{
		TenantID:        req.GetTenantID(),
		ID:              req.GetID(),
		Name:            req.GetName(),
		DispatchStatus:  model.DispatchStatus(strings.TrimSpace(req.GetDispatchStatus())),
		RatedCapacityKW: kwPtr,
		DispatchMode:    optionalTrimmedStringPtr(req.GetDispatchMode()),
		EnergyType:      optionalTrimmedStringPtr(req.GetEnergyType()),
		OwnerType:       optionalTrimmedStringPtr(req.GetOwnerType()),
		SubType:         optionalTrimmedStringPtr(req.GetSubType()),
		Description:     descUpdate,
		MarketEnabled:   assetMarketEnabledPtr(req.GetMarketEnabled()),
		Metadata:        meta,
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
		Opts: command.DeleteOptions{
			IncludeDescendants: req.GetIncludeDescendants(),
		},
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) MoveResource(ctx context.Context, req *resourcepb.MoveResourceRequest) (*emptypb.Empty, error) {
	logIn("move_resource", req)

	_, err := s.moveResource.Handle(ctx, command.MoveResource{
		TenantID:    req.GetTenantID(),
		ResourceID:  req.GetResourceID(),
		NewParentID: req.GetNewParentID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func optionalTrimmedStringPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// assetDescriptionPtr maps proto description: empty/whitespace clears to unset on the node.
func assetDescriptionPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func assetMarketEnabledPtr(v bool) *bool {
	return &v
}
