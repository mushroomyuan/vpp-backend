package resourcegrpc

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type fakeResourceServer struct {
	resourcepb.UnimplementedResourceServiceServer
	resp    *resourcepb.ResolveScopeResponse
	lastReq *resourcepb.ResolveScopeRequest
	fail    bool
	calls   atomic.Int64
}

func (s *fakeResourceServer) ResolveScope(
	_ context.Context, req *resourcepb.ResolveScopeRequest,
) (*resourcepb.ResolveScopeResponse, error) {
	s.calls.Add(1)
	s.lastReq = req
	if s.fail {
		return nil, status.Error(codes.Unavailable, "simulated dependency failure")
	}
	return s.resp, nil
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
	return dialer, func() { grpcSrv.Stop() }
}

func TestClient_ResolveScope_MapsMembersAndExclusions(t *testing.T) {
	t.Parallel()

	spec, err := structpb.NewStruct(map[string]any{"usable_energy_kwh": 200.0})
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	srv := &fakeResourceServer{
		resp: &resourcepb.ResolveScopeResponse{
			ScopeType:        resourcepb.ScopeType_SCOPE_TYPE_ASSET,
			ScopeID:          "asset-1",
			ResourceRevision: "abc123",
			PrecheckOK:       true,
			Members: []*resourcepb.ResolvedCU{{
				CUID:            "cu-1",
				AssetID:         "asset-1",
				LifecycleStatus: resourcepb.ResourceLifecycleStatus_RESOURCE_LIFECYCLE_STATUS_ACTIVE,
				NodeVersion:     4,
				Capabilities: []*resourcepb.ResolvedCapability{{
					CapabilityID:  string(contracts.CapabilityEnergyStorage),
					SchemaVersion: 1,
					Spec:          spec,
					Enabled:       true,
					Version:       2,
				}},
				Bindings: []*resourcepb.ResolvedMetricBinding{{
					MetricID:   string(contracts.MetricElectricalActivePowerSetpoint),
					AccessMode: resourcepb.PointAccessMode_POINT_ACCESS_MODE_WRITE,
					Enabled:    true,
					Revision:   9,
					SafetyConstraint: &resourcepb.PointSafetyConstraint{
						MinValue: wrapperspb.Double(-50),
						MaxValue: wrapperspb.Double(50),
						Version:  3,
					},
				}},
			}},
			Exclusions: []*resourcepb.ScopeExclusion{{
				CUID:    "cu-pv",
				AssetID: "asset-1",
				Reason:  resourcepb.ScopeExclusionReason_SCOPE_EXCLUSION_REASON_MISSING_CAPABILITY,
				Detail:  "energy.storage.v1",
			}},
			PrecheckFailures: []*resourcepb.ScopePrecheckFailure{{
				CUID:    "cu-2",
				AssetID: "asset-1",
				Reason:  resourcepb.ScopePrecheckFailureReason_SCOPE_PRECHECK_FAILURE_REASON_MISSING_METRIC_BINDING,
				Detail:  string(contracts.MetricEnergyStorageStateOfCharge),
			}},
		},
	}
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

	got, err := client.ResolveScope(context.Background(), port.ScopeQuery{
		TenantID:              "tenant-1",
		ScopeType:             port.ScopeAsset,
		ScopeID:               "asset-1",
		RequiredCapabilityIDs: []contracts.CapabilityID{contracts.CapabilityEnergyStorage},
		RequiredMetrics: []port.MetricRequirement{
			{MetricID: contracts.MetricEnergyStorageStateOfCharge, Access: port.MetricAccessRead},
			{MetricID: contracts.MetricElectricalActivePowerSetpoint, Access: port.MetricAccessWrite},
		},
	})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if got.ScopeType != port.ScopeAsset || got.ScopeID != "asset-1" || got.ResourceRevision != "abc123" || !got.PrecheckOK {
		t.Fatalf("envelope = %+v", got)
	}
	if len(got.Members) != 1 || got.Members[0].CUID != "cu-1" || got.Members[0].Lifecycle != port.LifecycleActive {
		t.Fatalf("members = %+v", got.Members)
	}
	cap := got.Members[0].Capabilities[0]
	if cap.CapabilityID != contracts.CapabilityEnergyStorage || cap.Spec["usable_energy_kwh"] != 200.0 {
		t.Fatalf("capability = %+v", cap)
	}
	binding := got.Members[0].Bindings[0]
	if binding.MetricID != contracts.MetricElectricalActivePowerSetpoint || binding.AccessMode != port.BindingAccessWrite || binding.Revision != 9 {
		t.Fatalf("binding = %+v", binding)
	}
	if binding.Safety == nil || binding.Safety.MinValue == nil || *binding.Safety.MinValue != -50 || binding.Safety.MaxChangePerSecond != nil {
		t.Fatalf("safety = %+v", binding.Safety)
	}
	if len(got.Exclusions) != 1 || got.Exclusions[0].Reason != port.ExclusionMissingCapability {
		t.Fatalf("exclusions = %+v", got.Exclusions)
	}
	if len(got.PrecheckFailures) != 1 || got.PrecheckFailures[0].Reason != port.PrecheckMissingMetricBinding {
		t.Fatalf("precheck = %+v", got.PrecheckFailures)
	}

	req := srv.lastReq
	if req.GetScopeType() != resourcepb.ScopeType_SCOPE_TYPE_ASSET || req.GetScopeID() != "asset-1" {
		t.Fatalf("request scope = %s %s", req.GetScopeType(), req.GetScopeID())
	}
	if len(req.GetRequiredCapabilityIDs()) != 1 || req.GetRequiredCapabilityIDs()[0] != string(contracts.CapabilityEnergyStorage) {
		t.Fatalf("capabilities = %v", req.GetRequiredCapabilityIDs())
	}
	if len(req.GetRequiredMetrics()) != 2 || req.GetRequiredMetrics()[1].GetAccess() != resourcepb.MetricAccessRequirement_METRIC_ACCESS_REQUIREMENT_WRITE {
		t.Fatalf("metrics = %+v", req.GetRequiredMetrics())
	}
}

func TestClient_ResolveScope_RejectsUnknownScopeWithoutRPC(t *testing.T) {
	t.Parallel()

	srv := &fakeResourceServer{}
	dialer, cleanup := newBufconnResourceServer(t, srv)
	defer cleanup()
	client, err := NewClient(Config{
		Addr:        "passthrough:///bufresource-bad",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.ResolveScope(context.Background(), port.ScopeQuery{
		TenantID:  "tenant-1",
		ScopeType: "portfolio",
		ScopeID:   "p-1",
	})
	if err == nil {
		t.Fatal("expected an error for an unsupported scope type")
	}
	if got := srv.calls.Load(); got != 0 {
		t.Fatalf("expected no RPC, got %d", got)
	}
}

func TestClient_BreakerOpen_ReturnsErrDependencyUnavailable(t *testing.T) {
	t.Parallel()

	srv := &fakeResourceServer{fail: true}
	dialer, cleanup := newBufconnResourceServer(t, srv)
	defer cleanup()
	breaker := resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             true,
		Name:                "test-decision->resource",
		ConsecutiveFailures: 1,
		OpenTimeout:         time.Minute,
		IsSuccessful:        resilience.GRPCIsSuccessful,
	})
	client, err := NewClient(Config{
		Addr:        "passthrough:///bufresource-breaker",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
		Breaker:     breaker,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	query := port.ScopeQuery{TenantID: "tenant-1", ScopeType: port.ScopeCU, ScopeID: "cu-1"}
	if _, err := client.ResolveScope(context.Background(), query); err == nil {
		t.Fatal("expected the first call to surface the RPC failure")
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
	_, err = client.ResolveScope(context.Background(), query)
	if !errors.Is(err, resilience.ErrDependencyUnavailable) {
		t.Fatalf("err = %v, want ErrDependencyUnavailable", err)
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("breaker open still called resource, calls = %d", got)
	}
}

func TestNewClient_RequiresAddr(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected an error when Addr is empty")
	}
}
