package forecast

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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	forecastpb "github.com/mushroomyuan/vpp-backend/api/forecast/proto/gen"
	inboundgrpc "github.com/mushroomyuan/vpp-backend/forecast/adapter/inbound/grpc"
	postgreshistory "github.com/mushroomyuan/vpp-backend/forecast/adapter/outbound/postgres_history"
	rediscache "github.com/mushroomyuan/vpp-backend/forecast/adapter/outbound/redis_cache"
	telemetrygrpc "github.com/mushroomyuan/vpp-backend/forecast/adapter/outbound/telemetry_grpc"
	"github.com/mushroomyuan/vpp-backend/forecast/application"
	"github.com/mushroomyuan/vpp-backend/forecast/application/command"
	"github.com/mushroomyuan/vpp-backend/forecast/config"
	fcmetrics "github.com/mushroomyuan/vpp-backend/forecast/metrics"
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type forecastServer struct {
	grpcSrv       *googlegrpc.Server
	httpSrv       *http.Server
	cfg           *config.Config
	metricsClient *metrics.Client
	metricsCancel context.CancelFunc
	telemetry     *telemetrygrpc.Client
	pgPool        *pgxpool.Pool
	redis         *platformredis.Client
	forecastLoop  *command.ForecastLoop
}

type preparedServer struct {
	*forecastServer
}

func createServer(appCfg *config.Config) (*forecastServer, error) {
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

	fcObs := fcmetrics.New()
	if err := metricsClient.RegisterCollector(fcObs.Collector()); err != nil {
		metricsCancel()
		return nil, fmt.Errorf("register forecast metrics: %w", err)
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

	pgPool, err := postgreshistory.NewPool(context.Background(), cfg.Postgres)
	if err != nil {
		metricsCancel()
		_ = telClient.Close()
		return nil, fmt.Errorf("init postgres pool: %w", err)
	}

	redisClient, err := platformredis.New(cfg.Redis)
	if err != nil {
		metricsCancel()
		_ = telClient.Close()
		pgPool.Close()
		return nil, fmt.Errorf("init redis client: %w", err)
	}

	app := application.NewApplication(application.Dependencies{
		Telemetry:     telClient,
		History:       postgreshistory.NewStore(pgPool),
		Cache:         rediscache.NewCache(redisClient, cfg.RedisTTL),
		Targets:       cfg.Targets,
		CycleInterval: cfg.CycleInterval,
		HorizonSteps:  cfg.HorizonSteps,
		StepSeconds:   cfg.StepSeconds,
		HistoryWindow: cfg.HistoryWindow,
		Metrics:       metricsClient,
		Observer:      fcObs,
	})

	logger := logrus.NewEntry(logrus.StandardLogger())
	ginEngine := platformserver.NewGinEngine(cfg.ServiceName, logger)
	httpSrv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ginEngine,
	}

	grpcSrv := platformserver.NewGRPCServer()
	reflection.Register(grpcSrv)
	forecastpb.RegisterForecastServiceServer(grpcSrv, inboundgrpc.NewServer(app))

	return &forecastServer{
		grpcSrv:       grpcSrv,
		httpSrv:       httpSrv,
		cfg:           cfg,
		metricsClient: metricsClient,
		metricsCancel: metricsCancel,
		telemetry:     telClient,
		pgPool:        pgPool,
		redis:         redisClient,
		forecastLoop:  app.ForecastLoop,
	}, nil
}

func (s *forecastServer) PrepareRun() *preparedServer {
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
		logrus.Infof("HTTP server listening on %s (healthz only)", s.cfg.HTTPAddr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		if err := s.forecastLoop.Run(egCtx); err != nil {
			return fmt.Errorf("forecast loop: %w", err)
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
		if err := s.telemetry.Close(); err != nil {
			logrus.WithError(err).Warn("telemetry gRPC client close error")
		}
		if err := s.redis.Close(); err != nil {
			logrus.WithError(err).Warn("redis client close error")
		}
		s.pgPool.Close()
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
