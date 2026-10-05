package integration

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/approval_stub"
	decisiondispatch "github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/dispatch_grpc"
	decisionpg "github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/postgres"
	decisionresource "github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/resource_grpc"
	decisiontelemetry "github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/telemetry_grpc"
	decisionapp "github.com/mushroomyuan/vpp-backend/decision/application"
	decisioncommand "github.com/mushroomyuan/vpp-backend/decision/application/command"
	"github.com/mushroomyuan/vpp-backend/decision/domain/allocation"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/evaluation"
	"github.com/mushroomyuan/vpp-backend/decision/domain/planning"
	dispatchinbound "github.com/mushroomyuan/vpp-backend/dispatch/adapter/inbound/grpc"
	dispatchapp "github.com/mushroomyuan/vpp-backend/dispatch/application"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
	resourceinbound "github.com/mushroomyuan/vpp-backend/resource/adapter/inbound/grpc"
	resourcepg "github.com/mushroomyuan/vpp-backend/resource/adapter/outbound/postgres"
	resourceapp "github.com/mushroomyuan/vpp-backend/resource/application"
	resourceinfra "github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
)

// decisionChain is the production Decision composition, pointed at the shared
// telemetry Redis and at bufconn servers for resource and dispatch.
type decisionChain struct {
	Resource  resourceapp.Application
	Policies  decisionapp.Application
	Cycle     *decisioncommand.RunDecisionCycle
	Execution *decisioncommand.PlanExecutionLoop
}

type decisionChainInput struct {
	TelemetryDial func(context.Context, string) (net.Conn, error)
	Dispatch      dispatchapp.Application
}

func startDecisionChain(ctx context.Context, in decisionChainInput) (decisionChain, []func(), error) {
	var closers []func()
	fail := func(err error) (decisionChain, []func(), error) {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
		return decisionChain{}, nil, err
	}

	resourceDSN, resourceClose, err := startPostgres(ctx, "postgres:16-alpine", "resource",
		"../../migrations/resource/000001_init.up.sql",
		"../../migrations/resource/000002_import_jobs_operation_target.up.sql",
		"../../migrations/resource/000003_scope_resolve_indexes.up.sql",
		"../../migrations/resource/000004_drop_cu_conn_status.up.sql",
	)
	if err != nil {
		return fail(fmt.Errorf("start resource postgres: %w", err))
	}
	closers = append(closers, resourceClose)

	decisionDSN, decisionClose, err := startPostgres(ctx, "postgres:16-alpine", "decision",
		"../../migrations/decision/000001_init.up.sql",
		"../../migrations/decision/000002_plan_execution.up.sql",
	)
	if err != nil {
		return fail(fmt.Errorf("start decision postgres: %w", err))
	}
	closers = append(closers, decisionClose)

	resourceApp := newResourceApplication(resourceDSN)
	resourceLis := bufconn.Listen(bufSize)
	resourceGRPC := platformserver.NewGRPCServer()
	resourcepb.RegisterResourceServiceServer(resourceGRPC, resourceinbound.NewServer(resourceApp))
	go func() { _ = resourceGRPC.Serve(resourceLis) }()
	closers = append(closers, resourceGRPC.Stop)

	dispatchLis := bufconn.Listen(bufSize)
	dispatchGRPC := platformserver.NewGRPCServer()
	dispatchpb.RegisterDispatchServiceServer(dispatchGRPC, dispatchinbound.NewServer(in.Dispatch))
	go func() { _ = dispatchGRPC.Serve(dispatchLis) }()
	closers = append(closers, dispatchGRPC.Stop)

	resourceClient, err := decisionresource.NewClient(decisionresource.Config{
		Addr: "passthrough:///bufresource",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return resourceLis.DialContext(ctx)
		})},
	})
	if err != nil {
		return fail(fmt.Errorf("dial resource: %w", err))
	}
	closers = append(closers, func() { _ = resourceClient.Close() })

	telemetryClient, err := decisiontelemetry.NewClient(decisiontelemetry.Config{
		Addr:        "passthrough:///buftelemetry",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(in.TelemetryDial)},
	})
	if err != nil {
		return fail(fmt.Errorf("dial telemetry: %w", err))
	}
	closers = append(closers, func() { _ = telemetryClient.Close() })

	dispatchClient, err := decisiondispatch.NewClient(decisiondispatch.Config{
		Addr: "passthrough:///bufdispatch",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return dispatchLis.DialContext(ctx)
		})},
	})
	if err != nil {
		return fail(fmt.Errorf("dial dispatch: %w", err))
	}
	closers = append(closers, func() { _ = dispatchClient.Close() })

	decisionDB := platformpostgres.NewPostgres(platformpostgres.Config{DSN: decisionDSN})
	policies := decisionpg.NewPolicyRepository(decisionDB)
	plans := decisionpg.NewPlanRepository(decisionDB)
	cooldown := decisionpg.NewCooldownStore(decisionDB)
	const (
		staleAge    = 2 * time.Minute
		window      = 30 * time.Second
		cooldownFor = time.Minute
	)
	cycle := decisioncommand.NewRunDecisionCycle(decisioncommand.CycleDependencies{
		Policies: policies,
		Resolver: dctx.NewCachingScopeResolver(dctx.CachingResolverConfig{
			Resource: resourceClient,
			TTL:      time.Second,
			MaxAge:   5 * time.Second,
		}),
		Collector:       dctx.NewSnapshotCollector(telemetryClient),
		Evaluator:       evaluation.NewSOCEvaluator(cooldown, cooldownFor, idgen.Must),
		Submit:          decisioncommand.NewSubmitObjectiveHandler(planning.NewImmediatePlanner(allocation.NewHeuristicAllocator(), idgen.Must), plans, approvalstub.New(), noopMetrics{}),
		StaleAge:        staleAge,
		Window:          window,
		DefaultCooldown: cooldownFor,
	})
	execution := decisioncommand.NewPlanExecutionLoop(decisioncommand.ExecutionDependencies{
		Plans:      plans,
		Resource:   resourceClient,
		Dispatch:   dispatchClient,
		Lease:      30 * time.Second,
		RetryAfter: 5 * time.Second,
		Interval:   time.Second,
		WorkerID:   "it-decision",
	})

	return decisionChain{
		Resource: resourceApp,
		Policies: decisionapp.New(decisionapp.Dependencies{
			Policies: policies,
			Resource: resourceClient,
			Metrics:  noopMetrics{},
		}),
		Cycle:     cycle,
		Execution: execution,
	}, closers, nil
}

func newResourceApplication(dsn string) resourceapp.Application {
	pg := resourceinfra.NewPostgres(platformpostgres.Config{DSN: dsn})
	nodeInfra := resourceinfra.NewNodeRepository(pg)
	return resourceapp.NewApplication(resourceapp.Dependencies{
		SiteRepo:         resourcepg.NewSiteRepositoryPostgres(resourceinfra.NewSiteRepository(pg), nodeInfra),
		AssetRepo:        resourcepg.NewAssetRepositoryPostgres(resourceinfra.NewAssetRepository(pg), nodeInfra),
		CURepo:           resourcepg.NewCURepositoryPostgres(resourceinfra.NewCURepository(pg), nodeInfra),
		CUCapabilityRepo: resourcepg.NewCUCapabilityRepositoryPostgres(resourceinfra.NewCUCapabilityRepository(pg)),
		PointRepo:        resourcepg.NewPointRepositoryPostgres(resourceinfra.NewPointRepository(pg), nodeInfra),
		ScopeRepo:        resourcepg.NewScopeRepositoryPostgres(resourceinfra.NewScopeRepository(pg)),
		JobRepo:          resourcepg.NewJobRepositoryPostgres(resourceinfra.NewJobRepository(pg)),
		NodeRepo:         resourcepg.NewNodeRepositoryPostgres(nodeInfra),
		Metrics:          noopMetrics{},
	})
}
