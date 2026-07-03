package telemetry

import (
	"context"
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
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
	grpcpkg "github.com/mushroomyuan/vpp-backend/telemetry/adapter/inbound/grpc"
	kafka "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/kafka"
	redisadapter "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/redis"
	"github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/timescaledb"
	"github.com/mushroomyuan/vpp-backend/telemetry/application"
	"github.com/mushroomyuan/vpp-backend/telemetry/config"
)

// ─── server structs ───────────────────────────────────────────────────────────

type telemetryServer struct {
	grpcSrv        *googlegrpc.Server
	cfg            *config.Config
	metricsClient  *metrics.Client
	metricsCancel  context.CancelFunc
	redisClient    *platformredis.Client
	tsPool         *pgxpool.Pool
	kafkaPublisher *kafka.EventPublisher
}

type preparedServer struct {
	*telemetryServer
}

// ─── wiring ───────────────────────────────────────────────────────────────────

// createServer wires every layer (TimescaleDB → infra repos → application →
// gRPC server) and returns a fully initialised but not-yet-started server.
// No goroutines are spawned here except for the metrics HTTP server, which has
// its own cancel function stored in telemetryServer.
func createServer(
	appCfg *config.Config,
	tsCfg timescaledb.Config,
	redisCfg platformredis.Config,
	kafkaCfg kafka.Config,
) (*telemetryServer, error) {
	// ── metrics ───────────────────────────────────────────────────────────────
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

	// ── infrastructure: TimescaleDB ───────────────────────────────────────────
	// pgxpool.Pool is created with a background context because the pool itself
	// is long-lived; individual queries pass their own request contexts.
	tsPool, err := timescaledb.NewPool(context.Background(), tsCfg)
	if err != nil {
		metricsCancel()
		return nil, fmt.Errorf("init timescaledb pool: %w", err)
	}
	logrus.Infof("timescaledb pool connected to %s:%d/%s", tsCfg.Host, tsCfg.Port, tsCfg.DBName)

	// Apply DDL (hypertable + continuous aggregate). All statements are
	// idempotent (IF NOT EXISTS), so this is safe on every startup.
	if err := timescaledb.ApplySchema(context.Background(), tsPool); err != nil {
		tsPool.Close()
		metricsCancel()
		return nil, fmt.Errorf("apply timescaledb schema: %w", err)
	}
	logrus.Info("timescaledb schema verified")

	// ── infrastructure: Redis ─────────────────────────────────────────────────
	redisClient, err := platformredis.New(redisCfg)
	if err != nil {
		metricsCancel()
		tsPool.Close()
		return nil, fmt.Errorf("init redis client: %w", err)
	}
	logrus.Infof("redis client connected to %s (db=%d)", redisCfg.Addr, redisCfg.DB)

	// ── outbound adapters ─────────────────────────────────────────────────────
	// TelemetryStore and AggregationStore share the same pgxpool to avoid
	// opening two separate connection pools.
	telemetryStore, aggregationStore := timescaledb.NewStores(tsPool)

	// SnapshotStore uses the underlying go-redis client directly.
	snapshotStore := redisadapter.NewSnapshotStore(redisClient.Client(), 0)

	eventPublisher := kafka.NewEventPublisher(kafkaCfg)

	// ── application layer ─────────────────────────────────────────────────────
	app := application.NewApplication(application.Dependencies{
		TelemetryRepo:   telemetryStore,
		SnapshotRepo:    snapshotStore,
		AggregationRepo: aggregationStore,
		EventPublisher:  eventPublisher,
		Metrics:         metricsClient,
	})

	// ── gRPC service ──────────────────────────────────────────────────────────
	telemetrySvc := grpcpkg.NewServer(app)

	grpcSrv := platformserver.NewGRPCServer()
	reflection.Register(grpcSrv)
	telemetrypb.RegisterTelemetryServiceServer(grpcSrv, telemetrySvc)

	return &telemetryServer{
		grpcSrv:        grpcSrv,
		cfg:            appCfg,
		metricsClient:  metricsClient,
		metricsCancel:  metricsCancel,
		redisClient:    redisClient,
		tsPool:         tsPool,
		kafkaPublisher: eventPublisher,
	}, nil
}

// ─── lifecycle ────────────────────────────────────────────────────────────────

// PrepareRun returns a preparedServer. Add pre-flight checks (DB ping,
// readiness probe registration) here before calling Run.
func (s *telemetryServer) PrepareRun() *preparedServer {
	return &preparedServer{s}
}

// Run starts the gRPC server and blocks until a shutdown signal or a fatal
// server error, then performs an ordered graceful shutdown.
//
// Supervision model: gRPC and the metrics error watcher run inside an
// errgroup. Any component returning a non-nil error — or a SIGINT/SIGTERM —
// cancels the shared context and triggers the shutdown coordinator.
//
// Shutdown order:
//  1. Stop accepting new traffic  — gRPC GracefulStop
//  2. Drain infrastructure        — Redis close, TimescaleDB pool close
//  3. Stop metrics last           — /metrics stays scrapeable during drain
func (s *preparedServer) Run() error {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	eg, egCtx := errgroup.WithContext(rootCtx)

	// ── gRPC ──────────────────────────────────────────────────────────────────
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

	// ── Metrics error watcher ─────────────────────────────────────────────────
	eg.Go(func() error {
		select {
		case err := <-s.metricsClient.Errors():
			return fmt.Errorf("metrics server: %w", err)
		case <-egCtx.Done():
			return nil
		}
	})

	// ── Signal watcher ────────────────────────────────────────────────────────
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

	// ── Shutdown coordinator ──────────────────────────────────────────────────
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-egCtx.Done()

		_, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// 1. Stop accepting new traffic.
		s.grpcSrv.GracefulStop()

		// 2. Close infrastructure.
		if err := s.redisClient.Close(); err != nil {
			logrus.WithError(err).Warn("redis close error")
		}
		if err := s.kafkaPublisher.Close(); err != nil {
			logrus.WithError(err).Warn("kafka publisher close error")
		}
		s.tsPool.Close()

		// 3. Stop metrics last.
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
