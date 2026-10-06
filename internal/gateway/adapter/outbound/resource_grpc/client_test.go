package resourcegrpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/binding"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type fakeResource struct {
	resourcepb.UnimplementedResourceServiceServer
	pages map[int32][]*resourcepb.Point
	last  *resourcepb.ListPointsRequest
}

func (f *fakeResource) ListPoints(_ context.Context, req *resourcepb.ListPointsRequest) (*resourcepb.ListPointsResponse, error) {
	f.last = req
	if req.GetTenantID() == "" || req.GetCUID() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant and cu")
	}
	return &resourcepb.ListPointsResponse{Points: f.pages[req.GetOffset()]}, nil
}

func dialFake(t *testing.T, srv *fakeResource) (*Client, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := platformserver.NewGRPCServer()
	resourcepb.RegisterResourceServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()
	client, err := NewClient(Config{
		Addr: "passthrough:///bufnet",
		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, func() {
		_ = client.Close()
		grpcSrv.Stop()
	}
}

func TestClient_ListCUBindings_PagesAndMapsFields(t *testing.T) {
	t.Parallel()
	full := make([]*resourcepb.Point, pageSize)
	for i := range full {
		full[i] = &resourcepb.Point{
			ID: "pt-page", CUID: "cu-1", MetricID: "electrical.active_power.v1",
			ExternalAddress: "reg", AccessMode: resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ,
			Scale: 1, Enabled: true, Revision: 1,
		}
	}
	min := wrapperspb.Double(0)
	srv := &fakeResource{pages: map[int32][]*resourcepb.Point{
		0: full,
		pageSize: {{
			ID: "pt-last", CUID: "cu-1", MetricID: "electrical.active_power_setpoint.v1",
			ExternalAddress: "set_kw", AccessMode: resourcepb.PointAccessMode_POINT_ACCESS_MODE_WRITE,
			Scale: 0.001, Offset: -5, Enabled: false, Revision: 4,
			SafetyConstraint: &resourcepb.PointSafetyConstraint{MinValue: min, Version: 2},
		}},
	}}
	client, done := dialFake(t, srv)
	defer done()

	got, err := client.ListCUBindings(context.Background(), "tenant", "cu-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != pageSize+1 {
		t.Fatalf("len = %d", len(got))
	}
	last := got[len(got)-1]
	if last.PointID != "pt-last" || last.AccessMode != binding.AccessWrite || last.Scale != 0.001 || last.Offset != -5 {
		t.Fatalf("last = %+v", last)
	}
	if last.Enabled || last.Revision != 4 || last.Safety == nil || *last.Safety.MinValue != 0 || last.Safety.Version != 2 {
		t.Fatalf("safety = %+v enabled %v", last.Safety, last.Enabled)
	}
	if srv.last.GetLimit() != pageSize || srv.last.GetCUID() != "cu-1" {
		t.Fatalf("last request = %+v", srv.last)
	}
}

func TestClient_ListCUBindings_RejectsMissingRevision(t *testing.T) {
	t.Parallel()
	srv := &fakeResource{pages: map[int32][]*resourcepb.Point{
		0: {{
			ID: "pt", MetricID: "electrical.active_power.v1", ExternalAddress: "reg",
			AccessMode: resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ, Scale: 1,
		}},
	}}
	client, done := dialFake(t, srv)
	defer done()
	if _, err := client.ListCUBindings(context.Background(), "tenant", "cu-1"); err == nil {
		t.Fatal("want revision error")
	}
}
