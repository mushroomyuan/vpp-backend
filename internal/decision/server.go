package decision

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	decisionpb "github.com/mushroomyuan/vpp-backend/api/decision/proto/gen"
	grpcpkg "github.com/mushroomyuan/vpp-backend/decision/adapter/inbound/grpc"
	httpgw "github.com/mushroomyuan/vpp-backend/decision/adapter/inbound/http"
	"github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/approval_stub"
	dispatchgrpc "github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/dispatch_grpc"
	policyrepo "github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/postgres"
	resourcegrpc "github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/resource_grpc"
	telemetrygrpc "github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/telemetry_grpc"
	"github.com/mushroomyuan/vpp-backend/decision/application"
	"github.com/mushroomyuan/vpp-backend/decision/application/command"
	"github.com/mushroomyuan/vpp-backend/decision/config"
	"github.com/mushroomyuan/vpp-backend/decision/domain/allocation"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/evaluation"
	"github.com/mushroomyuan/vpp-backend/decision/domain/planning"
	decisionmetrics "github.com/mushroomyuan/vpp-backend/decision/metrics"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	"github.com/mushroomyuan/vpp-backend/platform/middleware/grpcauth"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type decisionServer struct {
	httpSrv              *http.Server
	grpcSrv              *grpc.Server
	cfg                  *config.Config
	metricsClient        *metrics.Client
	metricsCancel        context.CancelFunc
	gatewayCancel        context.CancelFunc
	telemetry            *telemetrygrpc.Client
	resource             *resourcegrpc.Client
	dispatch             *dispatchgrpc.Client
	authzSyncer          *authz.Syncer
	authzAdmin           authz.PermissionAdmin
	authzCatalog         authz.Catalog
	authzRegisterCatalog bool
	decisionLoop         *command.DecisionLoop
	executionLoop        *command.PlanExecutionLoop
}

type preparedServer struct {
	*decisionServer
}

var (
	_ command.CycleObserver = (*decisionmetrics.Metrics)(nil)
	_ dctx.ResolveObserver  = (*decisionmetrics.Metrics)(nil)
)

func createServer(appCfg *config.Config) (*decisionServer, error) {
	cfg := appCfg

	metricsCtx, metricsCancel := context.WithCancel(context.Background())
	metricsClient, err := metrics.New(metricsCtx, metrics.Config{
		Addr:            cfg.MetricsAddr,
		EnableGoMetrics: true,
	})
	if err != nil {
		metricsCancel()
		return nil, fmt.Errorf("start metrics server: %w", err)
	}
	logrus.Infof("metrics server listening on %s", cfg.MetricsAddr)

	pg := platformpostgres.NewPostgres(postgresConfig(cfg.Database))
	if sqlDB, err := pg.SQLDb(); err != nil {
		logrus.WithError(err).Warn("skipping DB metrics: could not obtain sql.DB")
	} else if err := metricsClient.RegisterCollector(metrics.NewDBCollector(sqlDB, cfg.Database.Driver, "primary")); err != nil {
		logrus.WithError(err).Warn("skipping DB metrics: collector registration failed")
	}

	telClient, err := telemetrygrpc.NewClient(telemetrygrpc.Config{
		Addr:    cfg.Telemetry.Addr,
		Timeout: cfg.Telemetry.Timeout,
		Breaker: cfg.Telemetry.Breaker,
	})
	if err != nil {
		metricsCancel()
		return nil, fmt.Errorf("init telemetry gRPC client: %w", err)
	}
	resClient, err := resourcegrpc.NewClient(resourcegrpc.Config{
		Addr:    cfg.Resource.Addr,
		Timeout: cfg.Resource.Timeout,
		Breaker: cfg.Resource.Breaker,
	})
	if err != nil {
		metricsCancel()
		_ = telClient.Close()
		return nil, fmt.Errorf("init resource gRPC client: %w", err)
	}
	disClient, err := dispatchgrpc.NewClient(dispatchgrpc.Config{
		Addr:    cfg.Dispatch.Addr,
		Timeout: cfg.Dispatch.Timeout,
		Breaker: cfg.Dispatch.Breaker,
	})
	if err != nil {
		metricsCancel()
		_ = telClient.Close()
		_ = resClient.Close()
		return nil, fmt.Errorf("init dispatch gRPC client: %w", err)
	}

	policies := policyrepo.NewPolicyRepository(pg)
	plans := policyrepo.NewPlanRepository(pg)
	cooldown := policyrepo.NewCooldownStore(pg)
	planner := planning.NewImmediatePlanner(allocation.NewHeuristicAllocator(), idgen.Must)
	submit := command.NewSubmitObjectiveHandler(planner, plans, approvalstub.New(), metricsClient)
	app := application.New(application.Dependencies{
		Policies: policies,
		Resource: resClient,
		Metrics:  metricsClient,
	})
	decisionSvc := grpcpkg.NewServer(app)

	obs := decisionmetrics.New()
	if err := metricsClient.RegisterCollector(obs.Collector()); err != nil {
		metricsCancel()
		_ = telClient.Close()
		_ = resClient.Close()
		_ = disClient.Close()
		return nil, fmt.Errorf("register decision metrics: %w", err)
	}
	cycle := command.NewRunDecisionCycle(command.CycleDependencies{
		Policies: policies,
		Resolver: dctx.NewCachingScopeResolver(dctx.CachingResolverConfig{
			Resource: resClient,
			TTL:      cfg.ScopeCacheTTL,
			MaxAge:   cfg.ScopeCacheMaxAge,
			Observer: obs,
		}),
		Collector:       dctx.NewSnapshotCollector(telClient),
		Evaluator:       evaluation.NewSOCEvaluator(cooldown, cfg.DefaultCooldown, idgen.Must),
		Submit:          submit,
		Observer:        obs,
		StaleAge:        cfg.TelemetryStaleAge,
		Window:          cfg.DecisionInterval,
		DefaultCooldown: cfg.DefaultCooldown,
	})
	decisionLoop := command.NewDecisionLoop(cycle, cfg.DecisionInterval)
	executionLoop := command.NewPlanExecutionLoop(command.ExecutionDependencies{
		Plans:      plans,
		Resource:   resClient,
		Dispatch:   disClient,
		Lease:      cfg.ExecutionLease,
		RetryAfter: cfg.ExecutionRetryAfter,
		Interval:   cfg.ExecutionPollInterval,
		WorkerID:   idgen.Must(),
	})

	var (
		permissionChecker    authz.PermissionChecker
		authzSyncer          *authz.Syncer
		authzAdmin           authz.PermissionAdmin
		authzCatalog         authz.Catalog
		authzRegisterCatalog bool
	)
	if cfg.Authz.Enabled {
		wired, err := wireAuthz(cfg.Authz, cfg.ServiceName, metricsClient)
		if err != nil {
			metricsCancel()
			_ = telClient.Close()
			_ = resClient.Close()
			_ = disClient.Close()
			return nil, fmt.Errorf("wire authz: %w", err)
		}
		permissionChecker = wired.checker
		authzSyncer = wired.syncer
		authzAdmin = wired.admin
		authzCatalog = wired.catalog
		authzRegisterCatalog = cfg.Authz.RegisterCatalog
	}

	var extraUnary []grpc.UnaryServerInterceptor
	if cfg.TrustProxyHeaders {
		extraUnary = append(extraUnary, grpcauth.UnaryServerInterceptor(
			grpcauth.Config{TrustProxyHeaders: true},
			casdoor.ParseUserinfo,
			permissionChecker,
			grpcpkg.CatalogOf,
			grpcauth.ProtoTenantID,
		))
	}
	grpcSrv := platformserver.NewGRPCServer(extraUnary...)
	decisionpb.RegisterDecisionServiceServer(grpcSrv, decisionSvc)

	logger := logrus.NewEntry(logrus.StandardLogger())
	ginEngine := platformserver.NewGinEngine(cfg.ServiceName, logger)
	gwCtx, gwCancel := context.WithCancel(context.Background())
	if err := httpgw.Mount(gwCtx, ginEngine, httpgw.DialTarget(cfg.GRPCAddr)); err != nil {
		gwCancel()
		metricsCancel()
		_ = telClient.Close()
		_ = resClient.Close()
		_ = disClient.Close()
		return nil, fmt.Errorf("mount grpc-gateway: %w", err)
	}

	return &decisionServer{
		httpSrv:              &http.Server{Addr: cfg.HTTPAddr, Handler: ginEngine},
		grpcSrv:              grpcSrv,
		cfg:                  cfg,
		metricsClient:        metricsClient,
		metricsCancel:        metricsCancel,
		gatewayCancel:        gwCancel,
		telemetry:            telClient,
		resource:             resClient,
		dispatch:             disClient,
		authzSyncer:          authzSyncer,
		authzAdmin:           authzAdmin,
		authzCatalog:         authzCatalog,
		authzRegisterCatalog: authzRegisterCatalog,
		decisionLoop:         decisionLoop,
		executionLoop:        executionLoop,
	}, nil
}

func postgresConfig(c config.DatabaseConfig) platformpostgres.Config {
	params := make(map[string]string, len(c.Params))
	for k, v := range c.Params {
		params[k] = v
	}
	return platformpostgres.Config{
		Driver:                 c.Driver,
		Host:                   c.Host,
		Port:                   c.Port,
		User:                   c.User,
		Password:               c.Password,
		DBName:                 c.DBName,
		Params:                 params,
		DSN:                    c.DSN,
		MaxOpenConns:           c.MaxOpenConns,
		MaxIdleConns:           c.MaxIdleConns,
		ConnMaxLifetimeSeconds: c.ConnMaxLifetimeSeconds,
		ConnMaxIdleTimeSeconds: c.ConnMaxIdleTimeSeconds,
	}
}

type authzWiring struct {
	checker *authz.Checker
	syncer  *authz.Syncer
	admin   authz.PermissionAdmin
	catalog authz.Catalog
}

func wireAuthz(cfg config.AuthzConfig, serviceName string, metricsClient *metrics.Client) (authzWiring, error) {
	var out authzWiring
	authzMetrics := authz.NewMetrics(serviceName)
	if metricsClient != nil {
		if err := metricsClient.RegisterCollector(authzMetrics.Collector()); err != nil {
			return out, fmt.Errorf("register authz metrics: %w", err)
		}
	}
	checker, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter:         cfg.HealthyAfter,
		StaleAfter:           cfg.StaleAfter,
		AllowReadWhenInvalid: cfg.AllowReadWhenInvalid,
		DenyWritesWhenStale:  cfg.DenyWritesWhenStale,
		SnapshotPath:         cfg.SnapshotPath,
		SyncInterval:         cfg.SyncInterval,
		Owner:                cfg.Owner,
		ModelFilter:          cfg.ModelFilter,
	}, authzMetrics)
	if err != nil {
		return out, err
	}
	out.checker = checker
	out.catalog = grpcpkg.AuthzCatalog(cfg.Owner, cfg.ModelFilter)
	if cfg.Sync || cfg.RegisterCatalog {
		client, err := authz.NewCasdoorClient(authz.CasdoorClientConfig{
			BaseURL:      cfg.CasdoorURL,
			Organization: cfg.CasdoorOrg,
			Application:  cfg.CasdoorApp,
			Username:     cfg.CasdoorUsername,
			Password:     cfg.CasdoorPassword,
		})
		if err != nil {
			return out, err
		}
		out.admin = client
		if cfg.Sync {
			out.syncer = authz.NewSyncerWithMetrics(client, checker, authz.Config{
				HealthyAfter:         cfg.HealthyAfter,
				StaleAfter:           cfg.StaleAfter,
				AllowReadWhenInvalid: cfg.AllowReadWhenInvalid,
				DenyWritesWhenStale:  cfg.DenyWritesWhenStale,
				SnapshotPath:         cfg.SnapshotPath,
				SyncInterval:         cfg.SyncInterval,
				Owner:                cfg.Owner,
				ModelFilter:          cfg.ModelFilter,
			}, authzMetrics)
			logrus.Infof("authz syncer configured (casdoor=%s owner=%s interval=%s)", cfg.CasdoorURL, cfg.Owner, cfg.SyncInterval)
		}
	}
	if out.syncer == nil {
		logrus.Warn("authz checker enabled without syncer — using snapshot/safety-net only")
	}
	return out, nil
}

func (s *decisionServer) PrepareRun() *preparedServer {
	return &preparedServer{s}
}

func (s *preparedServer) Run() error {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	eg, egCtx := errgroup.WithContext(rootCtx)

	lis, err := net.Listen("tcp", s.cfg.GRPCAddr)
	if err != nil {
		s.shutdownClients()
		return fmt.Errorf("listen gRPC on %s: %w", s.cfg.GRPCAddr, err)
	}
	eg.Go(func() error {
		logrus.Infof("gRPC server listening on %s", s.cfg.GRPCAddr)
		if err := s.grpcSrv.Serve(lis); err != nil {
			return fmt.Errorf("gRPC server: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		logrus.Infof("decision loop interval %s; execution poll %s", s.cfg.DecisionInterval, s.cfg.ExecutionPollInterval)
		return s.decisionLoop.Run(egCtx)
	})

	eg.Go(func() error {
		return s.executionLoop.Run(egCtx)
	})

	eg.Go(func() error {
		logrus.Infof("HTTP server listening on %s", s.cfg.HTTPAddr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		select {
		case err := <-s.metricsClient.Errors():
			return fmt.Errorf("metrics server: %w", err)
		case <-egCtx.Done():
			return nil
		}
	})

	if s.authzSyncer != nil || (s.authzRegisterCatalog && s.authzAdmin != nil) {
		eg.Go(func() error {
			if s.authzRegisterCatalog && s.authzAdmin != nil {
				res, err := authz.RegisterCatalog(egCtx, s.authzAdmin, s.authzCatalog)
				if err != nil {
					logrus.WithError(err).Warn("authz catalog register failed (continuing with sync)")
				} else {
					logrus.Infof("authz catalog registered: added=%d updated=%d skipped=%d", res.Added, res.Updated, res.Skipped)
				}
			}
			if s.authzSyncer == nil {
				return nil
			}
			err := s.authzSyncer.Run(egCtx)
			if err != nil && !errors.Is(err, context.Canceled) {
				logrus.WithError(err).Warn("authz syncer stopped")
			}
			return nil
		})
	}

	eg.Go(func() error {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(quit)
		select {
		case sig := <-quit:
			logrus.Infof("received signal %v — initiating graceful shutdown", sig)
			rootCancel()
			return nil
		case <-egCtx.Done():
			return nil
		}
	})

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-egCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		s.grpcSrv.GracefulStop()
		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			logrus.WithError(err).Error("HTTP graceful shutdown error")
		}
		s.shutdownClients()
	}()

	if err := eg.Wait(); err != nil {
		logrus.WithError(err).Error("server stopped with error")
		<-shutdownDone
		return err
	}

	<-shutdownDone
	logrus.Info("server exited cleanly")
	return nil
}

func (s *decisionServer) shutdownClients() {
	if s.gatewayCancel != nil {
		s.gatewayCancel()
	}
	s.closeClients()
	if s.metricsCancel != nil {
		s.metricsCancel()
	}
}

func (s *decisionServer) closeClients() {
	if s.telemetry != nil {
		if err := s.telemetry.Close(); err != nil {
			logrus.WithError(err).Warn("telemetry gRPC client close error")
		}
	}
	if s.resource != nil {
		if err := s.resource.Close(); err != nil {
			logrus.WithError(err).Warn("resource gRPC client close error")
		}
	}
	if s.dispatch != nil {
		if err := s.dispatch.Close(); err != nil {
			logrus.WithError(err).Warn("dispatch gRPC client close error")
		}
	}
}
