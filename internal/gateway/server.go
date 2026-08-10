package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	grpcpkg "github.com/mushroomyuan/vpp-backend/gateway/adapter/inbound/grpc"
	httppkg "github.com/mushroomyuan/vpp-backend/gateway/adapter/inbound/http"
	kafkasub "github.com/mushroomyuan/vpp-backend/gateway/adapter/inbound/kafka"
	emslog "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/ems_log"
	kafkapub "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/kafka"
	adapterpostgres "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/postgres"
	"github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/simulator"
	telemetrygrpc "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/telemetry_grpc"
	"github.com/mushroomyuan/vpp-backend/gateway/application"
	"github.com/mushroomyuan/vpp-backend/gateway/config"
	infrapg "github.com/mushroomyuan/vpp-backend/gateway/infrastructure/persistent/postgres"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type gatewayServer struct {
	grpcSrv               *googlegrpc.Server
	httpSrv               *http.Server
	cfg                   *config.Config
	metricsClient         *metrics.Client
	metricsCancel         context.CancelFunc
	telemetryClient       *telemetrygrpc.TelemetryGRPCClient
	lifecycleConsumer     *kafkasub.LifecycleConsumer
	commandEventPublisher *kafkapub.CommandEventPublisher
	authzSyncer           *authz.Syncer
	authzAdmin            authz.PermissionAdmin
	authzCatalog          authz.Catalog
	authzRegisterCatalog  bool
}

type preparedServer struct {
	*gatewayServer
}

func createServer(
	appCfg *config.Config,
	dbCfg platformpostgres.Config,
	telemetryCfg telemetrygrpc.Config,
	simulatorCfg simulator.Config,
) (*gatewayServer, error) {
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

	mappingInfra := infrapg.NewMappingRepository(pg)
	mappingRepo := adapterpostgres.NewMappingRepositoryPostgres(mappingInfra)

	telemetryClient, err := telemetrygrpc.NewTelemetryGRPCClient(telemetryCfg)
	if err != nil {
		metricsCancel()
		return nil, fmt.Errorf("init telemetry gRPC client: %w", err)
	}

	defaultClient := emslog.NewEMSLogClient()
	var simClient *simulator.Client
	if strings.TrimSpace(simulatorCfg.Addr) != "" {
		c, err := simulator.NewClient(simulatorCfg)
		if err != nil {
			metricsCancel()
			_ = telemetryClient.Close()
			return nil, fmt.Errorf("init simulator client: %w", err)
		}
		simClient = c
	}
	emsClient := simulator.NewRouter(simClient, defaultClient)

	commandEventPublisher := kafkapub.NewCommandEventPublisher(kafkapub.CommandEventPublisherConfig{
		Brokers: appCfg.Kafka.Brokers,
		Topic:   appCfg.Kafka.CommandTopic,
	})

	app := application.NewApplication(application.Dependencies{
		MappingRepo:     mappingRepo,
		TelemetryClient: telemetryClient,
		EMSClient:       emsClient,
		CommandEvents:   commandEventPublisher,
		Metrics:         metricsClient,
	})

	lifecycleConsumer := kafkasub.NewLifecycleConsumer(
		kafkasub.LifecycleConsumerConfig{
			Brokers: appCfg.Kafka.Brokers,
			Topic:   appCfg.Kafka.Topic,
			GroupID: appCfg.Kafka.GroupID,
		},
		app.Commands.DisableMappingByCUCode,
	)

	gatewaySvc := grpcpkg.NewServer(app)
	grpcSrv := platformserver.NewGRPCServer()
	reflection.Register(grpcSrv)
	gatewaypb.RegisterGatewayServiceServer(grpcSrv, gatewaySvc)

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
			_ = telemetryClient.Close()
			return nil, fmt.Errorf("wire authz: %w", err)
		}
		permissionChecker = wired.checker
		authzSyncer = wired.syncer
		authzAdmin = wired.admin
		authzCatalog = wired.catalog
		authzRegisterCatalog = cfg.Authz.RegisterCatalog
	}

	logger := logrus.NewEntry(logrus.StandardLogger())
	ginEngine := platformserver.NewGinEngine(cfg.ServiceName, logger)
	authMW := httppkg.AuthMiddleware(httppkg.AuthConfig{
		TrustProxyHeaders: cfg.TrustProxyHeaders,
	}, casdoor.ParseUserinfo, permissionChecker)
	httppkg.RegisterRoutes(ginEngine, app, authMW)

	httpSrv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ginEngine,
	}

	return &gatewayServer{
		grpcSrv:               grpcSrv,
		httpSrv:               httpSrv,
		cfg:                   cfg,
		metricsClient:         metricsClient,
		metricsCancel:         metricsCancel,
		telemetryClient:       telemetryClient,
		lifecycleConsumer:     lifecycleConsumer,
		commandEventPublisher: commandEventPublisher,
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
	out.catalog = httppkg.AuthzCatalog(cfg.Owner, cfg.ModelFilter)

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
			logrus.Infof("authz syncer configured (casdoor=%s owner=%s interval=%s register-catalog=%v)",
				cfg.CasdoorURL, cfg.Owner, cfg.SyncInterval, cfg.RegisterCatalog)
		} else {
			logrus.Infof("authz catalog register enabled without syncer (casdoor=%s)", cfg.CasdoorURL)
		}
	}
	if out.syncer == nil {
		logrus.Warn("authz checker enabled without syncer — using snapshot/safety-net only")
	}
	return out, nil
}

func (s *gatewayServer) PrepareRun() *preparedServer {
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

	eg.Go(func() error {
		if err := s.lifecycleConsumer.Run(egCtx); err != nil {
			return fmt.Errorf("lifecycle consumer: %w", err)
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

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		s.grpcSrv.GracefulStop()
		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			logrus.WithError(err).Error("HTTP graceful shutdown error")
		}
		if err := s.telemetryClient.Close(); err != nil {
			logrus.WithError(err).Warn("telemetry gRPC client close error")
		}
		if err := s.lifecycleConsumer.Close(); err != nil {
			logrus.WithError(err).Warn("lifecycle consumer close error")
		}
		if err := s.commandEventPublisher.Close(); err != nil {
			logrus.WithError(err).Warn("command event publisher close error")
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
