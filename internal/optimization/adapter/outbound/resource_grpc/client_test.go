package resourcegrpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// fakeResourceServer implements resourcepb.ResourceServiceServer with a
// canned set of assets, returning NOT_FOUND for anything else.
type fakeResourceServer struct {
	resourcepb.UnimplementedResourceServiceServer
	assets map[string]float64 // ID -> RatedCapacityKW
}

func (s *fakeResourceServer) GetAsset(_ context.Context, req *resourcepb.GetAssetRequest) (*resourcepb.Asset, error) {
	kw, ok := s.assets[req.GetID()]
	if !ok {
		return nil, status.Error(codes.NotFound, "asset not found")
	}
	return &resourcepb.Asset{ID: req.GetID(), TenantID: req.GetTenantID(), RatedCapacityKW: kw}, nil
}

func newBufconnResourceServer(t *testing.T, srv *fakeResourceServer) (func(context.Context, string) (net.Conn, error), func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := platformserver.NewGRPCServer()
	resourcepb.RegisterResourceServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	cleanup := func() { grpcSrv.Stop() }
	return dialer, cleanup
}

func TestClient_GetCapacityKW_ReturnsKnownAssets(t *testing.T) {
	t.Parallel()

	srv := &fakeResourceServer{assets: map[string]float64{"asset-a": 100, "asset-b": 300}}
	dialer, cleanup := newBufconnResourceServer(t, srv)
	defer cleanup()

	client, err := NewClient(Config{
		Addr:        "passthrough:///bufresource",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	got, err := client.GetCapacityKW(context.Background(), "tenant-1", []string{"asset-a", "asset-b"})
	if err != nil {
		t.Fatalf("GetCapacityKW: %v", err)
	}
	if got["asset-a"] != 100 || got["asset-b"] != 300 {
		t.Errorf("unexpected capacities: %+v", got)
	}
}

func TestClient_GetCapacityKW_OmitsUnknownScopeEntries(t *testing.T) {
	t.Parallel()

	srv := &fakeResourceServer{assets: map[string]float64{"asset-a": 100}}
	dialer, cleanup := newBufconnResourceServer(t, srv)
	defer cleanup()

	client, err := NewClient(Config{
		Addr:        "passthrough:///bufresource-unknown",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	got, err := client.GetCapacityKW(context.Background(), "tenant-1", []string{"asset-a", "asset-unknown"})
	if err != nil {
		t.Fatalf("GetCapacityKW should not error on a NotFound scope entry: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only the known asset in the result, got %+v", got)
	}
	if _, ok := got["asset-unknown"]; ok {
		t.Error("expected unknown asset to be omitted, not zero-valued")
	}
}

func TestNewClient_RequiresAddr(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected an error when Addr is empty")
	}
}
