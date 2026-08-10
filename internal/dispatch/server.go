package dispatch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	grpcpkg "github.com/mushroomyuan/vpp-backend/dispatch/adapter/inbound/grpc"
	kafkasub "github.com/mushroomyuan/vpp-backend/dispatch/adapter/inbound/kafka"
	gatewaygrpc "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/gateway_grpc"
	kafkapub "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/kafka"
	adapterpostgres "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/postgres"
	"github.com/mushroomyuan/vpp-backend/dispatch/application"
	"github.com/mushroomyuan/vpp-backend/dispatch/application/command"
	"github.com/mushroomyuan/vpp-backend/dispatch/config"
	infrapg "github.com/mushroomyuan/vpp-backend/dispatch/infrastructure/persistent/postgres"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	"github.com/mushroomyuan/vpp-backend/platform/middleware/grpcauth"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type dispatchServer struct {
	grpcSrv               *googlegrpc.Server
	cfg                   *config.Config
	metricsClient         *metrics.Client
	metricsCancel         context.CancelFunc
	gatewayClient         *gatewaygrpc.Client
	eventPublisher        *kafkapub.EventPublisher
	commandResultConsumer *kafkasub.CommandResultConsumer
	timeoutScanner        *command.TimeoutScanner
	authzSyncer           *authz.Syncer
	authzAdmin            authz.PermissionAdmin
	authzCatalog          authz.Catalog
	authzRegisterCatalog  bool
}

type preparedServer struct {
	*dispatchServer
}

func createServer(
	appCfg *config.Config,
	dbCfg platformpostgres.Config,
	gatewayCfg gatewaygrpc.Config,
) (*dispatchServer, error) {
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

	pg := infrapg.NewPostgres(dbCfg)
	if sqlDB, err := pg.SQLDb(); err != nil {
		logrus.WithError(err).Warn("skipping DB metrics: could not obtain sql.DB")
	} else if err := metricsClient.RegisterCollector(
		metrics.NewDBCollector(sqlDB, dbCfg.Driver, "primary"),
	); err != nil {
		logrus.WithError(err).Warn("skipping DB metrics: collector registration failed")
	}

	taskInfra := infrapg.NewTaskRepository(pg)
	actionInfra := infrapg.NewActionRepository(pg)
	commandInfra := infrapg.NewCommandRepository(pg)

	taskRepo := adapterpostgres.NewTaskRepositoryPostgres(taskInfra)
	actionRepo := adapterpostgres.NewActionRepositoryPostgres(actionInfra)
	commandRepo := adapterpostgres.NewCommandRepositoryPostgres(commandInfra)

	gatewayClient, err := gatewaygrpc.NewClient(gatewayCfg)
	if err != nil {
		metricsCancel()
		return nil, fmt.Errorf("init gateway gRPC client: %w", err)
	}

	eventPublisher := kafkapub.NewEventPublisher(kafkapub.Config{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.DispatchTopic,
	})

	app := application.NewApplication(application.Dependencies{
		TaskRepo:              taskRepo,
		ActionRepo:            actionRepo,
		CommandRepo:           commandRepo,
		Gateway:               gatewayClient,
		Publisher:             eventPublisher,
		Metrics:               metricsClient,
		TimeoutScanInterval:   cfg.TimeoutScanInterval,
		DefaultCommandTimeout: cfg.DefaultCommandTimeout,
		DefaultMaxRetries:     cfg.DefaultMaxRetries,
	})

	commandResultConsumer := kafkasub.NewCommandResultConsumer(
		kafkasub.CommandResultConsumerConfig{
			Brokers: cfg.Kafka.Brokers,
			Topic:   cfg.Kafka.CommandTopic,
			GroupID: cfg.Kafka.GroupID,
		},
		app.Commands.HandleCommandResult,
	)

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
			_ = gatewayClient.Close()
			return nil, fmt.Errorf("wire authz: %w", err)
		}
		permissionChecker = wired.checker
		authzSyncer = wired.syncer
		authzAdmin = wired.admin
		authzCatalog = wired.catalog
		authzRegisterCatalog = cfg.Authz.RegisterCatalog
	}

	dispatchSvc := grpcpkg.NewServer(app)

	var extraUnary []googlegrpc.UnaryServerInterceptor
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
	reflection.Register(grpcSrv)
	dispatchpb.RegisterDispatchServiceServer(grpcSrv, dispatchSvc)

	return &dispatchServer{
		grpcSrv:               grpcSrv,
		cfg:                   cfg,
		metricsClient:         metricsClient,
		metricsCancel:         metricsCancel,
		gatewayClient:         gatewayClient,
		eventPublisher:        eventPublisher,
		commandResultConsumer: commandResultConsumer,
		timeoutScanner:        app.TimeoutScanner,
		authzSyncer:           authzSyncer,
		authzAdmin:            authzAdmin,
		authzCatalog:          authzCatalog,
		authzRegisterCatalog:  authzRegisterCatalog,
	}, nil
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

	authzCfg := authz.Config{
		HealthyAfter:         cfg.HealthyAfter,
		StaleAfter:           cfg.StaleAfter,
		AllowReadWhenInvalid: cfg.AllowReadWhenInvalid,
		DenyWritesWhenStale:  cfg.DenyWritesWhenStale,
		SnapshotPath:         cfg.SnapshotPath,
		SyncInterval:         cfg.SyncInterval,
		Owner:                cfg.Owner,
		ModelFilter:          cfg.ModelFilter,
	}
	checker, err := authz.NewCheckerWithMetrics(authzCfg, authzMetrics)
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
			out.syncer = authz.NewSyncerWithMetrics(client, checker, authzCfg, authzMetrics)
			logrus.Infof("authz syncer configured (casdoor=%s owner=%s interval=%s healthy=%s stale=%s deny-writes-when-stale=%v)",
				cfg.CasdoorURL, cfg.Owner, cfg.SyncInterval, cfg.HealthyAfter, cfg.StaleAfter, cfg.DenyWritesWhenStale)
		}
	}
	if out.syncer == nil {
		logrus.Warn("authz checker enabled without syncer — using snapshot/safety-net only")
	}
	return out, nil
}

func (s *dispatchServer) PrepareRun() *preparedServer {
	return &preparedServer{s}
}

func (s *preparedServer) Run() error {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	eg, egCtx := errgroup.WithContext(rootCtx)

	lis, err := net.Listen("tcp", s.cfg.GRPCAddr)
	if err != nil {
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
		select {
		case err := <-s.metricsClient.Errors():
			return fmt.Errorf("metrics server: %w", err)
		case <-egCtx.Done():
			return nil
		}
	})

	eg.Go(func() error {
		if err := s.timeoutScanner.Run(egCtx); err != nil {
			return fmt.Errorf("timeout scanner: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		if err := s.commandResultConsumer.Run(egCtx); err != nil {
			return fmt.Errorf("command result consumer: %w", err)
		}
		return nil
	})

	if s.authzSyncer != nil || (s.authzRegisterCatalog && s.authzAdmin != nil) {
		eg.Go(func() error {
			if s.authzRegisterCatalog && s.authzAdmin != nil {
				res, err := authz.RegisterCatalog(egCtx, s.authzAdmin, s.authzCatalog)
				if err != nil {
					logrus.WithError(err).Warn("authz catalog register failed (continuing with sync)")
				} else {
					logrus.Infof("authz catalog registered: added=%d updated=%d skipped=%d",
						res.Added, res.Updated, res.Skipped)
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

		s.grpcSrv.GracefulStop()
		if err := s.gatewayClient.Close(); err != nil {
			logrus.WithError(err).Warn("gateway gRPC client close error")
		}
		if err := s.eventPublisher.Close(); err != nil {
			logrus.WithError(err).Warn("event publisher close error")
		}
		if err := s.commandResultConsumer.Close(); err != nil {
			logrus.WithError(err).Warn("command result consumer close error")
		}
		s.metricsCancel()
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
