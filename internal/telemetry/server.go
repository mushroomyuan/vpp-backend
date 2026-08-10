package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	"github.com/mushroomyuan/vpp-backend/platform/middleware/grpcauth"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
	grpcpkg "github.com/mushroomyuan/vpp-backend/telemetry/adapter/inbound/grpc"
	kafka "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/kafka"
	redisadapter "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/redis"
	"github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/timescaledb"
	"github.com/mushroomyuan/vpp-backend/telemetry/application"
	"github.com/mushroomyuan/vpp-backend/telemetry/config"
)

type telemetryServer struct {
	grpcSrv              *googlegrpc.Server
	cfg                  *config.Config
	metricsClient        *metrics.Client
	metricsCancel        context.CancelFunc
	redisClient          *platformredis.Client
	tsPool               *pgxpool.Pool
	kafkaPublisher       *kafka.EventPublisher
	authzSyncer          *authz.Syncer
	authzAdmin           authz.PermissionAdmin
	authzCatalog         authz.Catalog
	authzRegisterCatalog bool
}

type preparedServer struct {
	*telemetryServer
}

func createServer(
	appCfg *config.Config,
	tsCfg timescaledb.Config,
	redisCfg platformredis.Config,
	kafkaCfg kafka.Config,
) (*telemetryServer, error) {
	metricsCtx, metricsCancel := context.WithCancel(context.Background())
	metricsClient, err := metrics.New(metricsCtx, metrics.Config{
		Addr:            appCfg.MetricsAddr,
		EnableGoMetrics: true,
	})
	if err != nil {
		metricsCancel()
		return nil, fmt.Errorf("start metrics server: %w", err)
	}
	logrus.Infof("metrics server listening on %s", appCfg.MetricsAddr)

	tsPool, err := timescaledb.NewPool(context.Background(), tsCfg)
	if err != nil {
		metricsCancel()
		return nil, fmt.Errorf("init timescaledb pool: %w", err)
	}
	logrus.Infof("timescaledb pool connected to %s:%d/%s", tsCfg.Host, tsCfg.Port, tsCfg.DBName)

	if err := timescaledb.ApplySchema(context.Background(), tsPool); err != nil {
		tsPool.Close()
		metricsCancel()
		return nil, fmt.Errorf("apply timescaledb schema: %w", err)
	}
	logrus.Info("timescaledb schema verified")

	redisClient, err := platformredis.New(redisCfg)
	if err != nil {
		metricsCancel()
		tsPool.Close()
		return nil, fmt.Errorf("init redis client: %w", err)
	}
	logrus.Infof("redis client connected to %s (db=%d)", redisCfg.Addr, redisCfg.DB)

	telemetryStore, aggregationStore := timescaledb.NewStores(tsPool)
	snapshotStore := redisadapter.NewSnapshotStore(redisClient.Client(), 0)
	eventPublisher := kafka.NewEventPublisher(kafkaCfg)

	app := application.NewApplication(application.Dependencies{
		TelemetryRepo:   telemetryStore,
		SnapshotRepo:    snapshotStore,
		AggregationRepo: aggregationStore,
		EventPublisher:  eventPublisher,
		Metrics:         metricsClient,
	})

	var (
		permissionChecker    authz.PermissionChecker
		authzSyncer          *authz.Syncer
		authzAdmin           authz.PermissionAdmin
		authzCatalog         authz.Catalog
		authzRegisterCatalog bool
	)
	if appCfg.Authz.Enabled {
		wired, err := wireAuthz(appCfg.Authz, appCfg.ServiceName, metricsClient)
		if err != nil {
			metricsCancel()
			_ = redisClient.Close()
			tsPool.Close()
			return nil, fmt.Errorf("wire authz: %w", err)
		}
		permissionChecker = wired.checker
		authzSyncer = wired.syncer
		authzAdmin = wired.admin
		authzCatalog = wired.catalog
		authzRegisterCatalog = appCfg.Authz.RegisterCatalog
	}

	telemetrySvc := grpcpkg.NewServer(app)

	var extraUnary []googlegrpc.UnaryServerInterceptor
	if appCfg.TrustProxyHeaders {
		pep := grpcauth.UnaryServerInterceptor(
			grpcauth.Config{TrustProxyHeaders: true},
			casdoor.ParseUserinfo,
			permissionChecker,
			grpcpkg.CatalogOf,
			grpcauth.ProtoTenantID,
		)
		extraUnary = append(extraUnary, grpcpkg.WithMachineBypass(pep))
	}
	grpcSrv := platformserver.NewGRPCServer(extraUnary...)
	reflection.Register(grpcSrv)
	telemetrypb.RegisterTelemetryServiceServer(grpcSrv, telemetrySvc)

	return &telemetryServer{
		grpcSrv:              grpcSrv,
		cfg:                  appCfg,
		metricsClient:        metricsClient,
		metricsCancel:        metricsCancel,
		redisClient:          redisClient,
		tsPool:               tsPool,
		kafkaPublisher:       eventPublisher,
		authzSyncer:          authzSyncer,
		authzAdmin:           authzAdmin,
		authzCatalog:         authzCatalog,
		authzRegisterCatalog: authzRegisterCatalog,
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

func (s *telemetryServer) PrepareRun() *preparedServer {
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

		_, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		s.grpcSrv.GracefulStop()

		if err := s.redisClient.Close(); err != nil {
			logrus.WithError(err).Warn("redis close error")
		}
		if err := s.kafkaPublisher.Close(); err != nil {
			logrus.WithError(err).Warn("kafka publisher close error")
		}
		s.tsPool.Close()
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
