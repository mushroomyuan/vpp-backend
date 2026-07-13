package resourceclient

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/wrapperspb"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/simulator/domain"
)

// Config for the Resource gRPC client.
type Config struct {
	Addr string
}

// Client wraps ResourceServiceClient.
type Client struct {
	conn   *grpc.ClientConn
	client resourcepb.ResourceServiceClient
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, fmt.Errorf("resource client: addr is required")
	}
	conn, err := grpc.NewClient(cfg.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("resource client: dial %s: %w", cfg.Addr, err)
	}
	return &Client{
		conn:   conn,
		client: resourcepb.NewResourceServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// LoadFilter selects which CUs become simulated devices.
type LoadFilter struct {
	TenantID        string
	SiteIDs         []string
	CUIDs           []string
	RequireProvider string
}

// LoadDeviceSpecs walks Site → Asset → CU → Point and returns DeviceSpecs.
func (c *Client) LoadDeviceSpecs(ctx context.Context, f LoadFilter) ([]domain.DeviceSpec, error) {
	cuIDSet := toSet(f.CUIDs)
	siteIDSet := toSet(f.SiteIDs)

	sites, err := c.listAllSites(ctx, f.TenantID)
	if err != nil {
		return nil, err
	}

	var specs []domain.DeviceSpec
	for _, site := range sites {
		if len(siteIDSet) > 0 && !siteIDSet[site.GetID()] {
			continue
		}
		assets, err := c.listAllAssets(ctx, f.TenantID, site.GetID())
		if err != nil {
			return nil, err
		}
		for _, asset := range assets {
			cus, err := c.listAllCUs(ctx, f.TenantID, asset.GetID())
			if err != nil {
				return nil, err
			}
			for _, cu := range cus {
				if len(cuIDSet) > 0 && !cuIDSet[cu.GetID()] {
					continue
				}
				if f.RequireProvider != "" &&
					!strings.EqualFold(strings.TrimSpace(cu.GetProvider()), f.RequireProvider) {
					continue
				}
				points, err := c.listAllPoints(ctx, f.TenantID, cu.GetID())
				if err != nil {
					return nil, err
				}
				extID := strings.TrimSpace(cu.GetExternalID())
				if extID == "" {
					extID = cu.GetID()
				}
				spec := domain.DeviceSpec{
					TenantID:        f.TenantID,
					CUCode:          cu.GetID(),
					ExternalID:      extID,
					Name:            cu.GetName(),
					Type:            cu.GetType(),
					Provider:        cu.GetProvider(),
					AssetID:         asset.GetID(),
					RatedCapacityKW: asset.GetRatedCapacityKW(),
					EnergyType:      asset.GetEnergyType(),
					Points:          mapPoints(points),
				}
				specs = append(specs, spec)
			}
		}
	}
	return specs, nil
}

func mapPoints(points []*resourcepb.Point) []domain.PointDef {
	out := make([]domain.PointDef, 0, len(points))
	for _, p := range points {
		out = append(out, domain.PointDef{
			ID:          p.GetID(),
			PointKey:    p.GetPointKey(),
			DataType:    p.GetDataType().String(),
			ControlFlag: p.GetControlFlag(),
			IsVirtual:   p.GetIsVirtual(),
		})
	}
	return out
}

func (c *Client) listAllSites(ctx context.Context, tenantID string) ([]*resourcepb.Site, error) {
	var all []*resourcepb.Site
	const limit int32 = 200
	for offset := int32(0); ; offset += limit {
		resp, err := c.client.ListSites(ctx, &resourcepb.ListSitesRequest{
			TenantID: tenantID,
			Offset:   offset,
			Limit:    limit,
		})
		if err != nil {
			return nil, fmt.Errorf("ListSites: %w", err)
		}
		all = append(all, resp.GetSites()...)
		if int32(len(resp.GetSites())) < limit {
			break
		}
	}
	return all, nil
}

func (c *Client) listAllAssets(ctx context.Context, tenantID, siteID string) ([]*resourcepb.Asset, error) {
	var all []*resourcepb.Asset
	const limit int32 = 200
	for offset := int32(0); ; offset += limit {
		resp, err := c.client.ListAssets(ctx, &resourcepb.ListAssetsRequest{
			TenantID: tenantID,
			SiteID:   siteID,
			Offset:   offset,
			Limit:    limit,
		})
		if err != nil {
			return nil, fmt.Errorf("ListAssets site=%s: %w", siteID, err)
		}
		all = append(all, resp.GetAssets()...)
		if int32(len(resp.GetAssets())) < limit {
			break
		}
	}
	return all, nil
}

func (c *Client) listAllCUs(ctx context.Context, tenantID, assetID string) ([]*resourcepb.CU, error) {
	var all []*resourcepb.CU
	const limit int32 = 200
	for offset := int32(0); ; offset += limit {
		resp, err := c.client.ListCUs(ctx, &resourcepb.ListCUsRequest{
			TenantID: tenantID,
			AssetID:  assetID,
			Offset:   offset,
			Limit:    limit,
		})
		if err != nil {
			return nil, fmt.Errorf("ListCUs asset=%s: %w", assetID, err)
		}
		all = append(all, resp.GetCUs()...)
		if int32(len(resp.GetCUs())) < limit {
			break
		}
	}
	return all, nil
}

func (c *Client) listAllPoints(ctx context.Context, tenantID, cuID string) ([]*resourcepb.Point, error) {
	var all []*resourcepb.Point
	const limit int32 = 200
	for offset := int32(0); ; offset += limit {
		resp, err := c.client.ListPoints(ctx, &resourcepb.ListPointsRequest{
			TenantID:  tenantID,
			CUID:      cuID,
			IsVirtual: wrapperspb.Bool(false),
			Offset:    offset,
			Limit:     limit,
		})
		if err != nil {
			return nil, fmt.Errorf("ListPoints cu=%s: %w", cuID, err)
		}
		all = append(all, resp.GetPoints()...)
		if int32(len(resp.GetPoints())) < limit {
			break
		}
	}
	return all, nil
}

func toSet(ids []string) map[string]bool {
	if len(ids) == 0 {
		return nil
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			m[id] = true
		}
	}
	return m
}
