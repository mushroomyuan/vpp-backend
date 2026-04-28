package resource

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
	googlegrpc "google.golang.org/grpc"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
	"github.com/mushroomyuan/vpp-backend/resource/adapters"
	"github.com/mushroomyuan/vpp-backend/resource/application"
	"github.com/mushroomyuan/vpp-backend/resource/config"
	infradb "github.com/mushroomyuan/vpp-backend/resource/infrastructure/db"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
	grpcpkg "github.com/mushroomyuan/vpp-backend/resource/ports/grpc"

	// gateway.go declares `package ports` — alias for clarity
	gatewaypkg "github.com/mushroomyuan/vpp-backend/resource/ports/http"
)

// ─── server structs ───────────────────────────────────────────────────────────

type resourceServer struct {
	grpcSrv       *googlegrpc.Server
	httpSrv       *http.Server
	app           application.Application
	cfg           *config.Config
	metricsClient *metrics.Client
	metricsCancel context.CancelFunc
}

type preparedServer struct {
	*resourceServer
}

// ─── wiring ───────────────────────────────────────────────────────────────────

// createServer wires every layer (postgres → infra repos → adapters →
// application → gRPC/HTTP servers) and returns a fully initialised but not-yet-
// started server. No goroutines are spawned here except for the metrics HTTP
// server, which has its own cancel function stored in resourceServer.
//
// dbCfg is driver-agnostic and intentionally separate from appCfg so that
// infrastructure details never leak into the application config type.
func createServer(appCfg *config.Config, dbCfg infradb.Config) (*resourceServer, error) {
	cfg := appCfg

	// ── metrics ───────────────────────────────────────────────────────────────
	// The metrics server owns a dedicated context so it can be shut down
	// independently and cleanly before the process exits.
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

	// ── infrastructure layer ──────────────────────────────────────────────────
	pg := postgres.NewPostgres(dbCfg)

	// Register DB connection-pool metrics on the same /metrics endpoint.
	if sqlDB, err := pg.SQLDb(); err != nil {
		logrus.WithError(err).Warn("skipping DB metrics: could not obtain sql.DB")
	} else if err := metricsClient.RegisterCollector(
		metrics.NewDBCollector(sqlDB, dbCfg.Driver, "primary"),
	); err != nil {
		logrus.WithError(err).Warn("skipping DB metrics: collector registration failed")
	}

	siteInfra := postgres.NewSiteRepository(pg)
	resourceInfra := postgres.NewResourceRepository(pg)
	cuInfra := postgres.NewCURepository(pg)
	pointInfra := postgres.NewPointRepository(pg)
	jobInfra := postgres.NewJobRepository(pg)

	// ── adapters (port.XxxRepository implementations) ─────────────────────────
	siteRepo := adapters.NewSiteRepositoryPostgres(siteInfra)
	resourceRepo := adapters.NewResourceRepositoryPostgres(resourceInfra)
	cuRepo := adapters.NewCURepositoryPostgres(cuInfra)
	pointRepo := adapters.NewPointRepositoryPostgres(pointInfra)
	jobRepo := adapters.NewJobRepositoryPostgres(jobInfra)

	// ── application layer (all CQRS handlers + background worker) ─────────────
	app := application.NewApplication(application.Dependencies{
		SiteRepo:           siteRepo,
		ResourceRepo:       resourceRepo,
		CURepo:             cuRepo,
		PointRepo:          pointRepo,
		JobRepo:            jobRepo,
		Metrics:            metricsClient,
		ImportWorkerConfig: cfg.WorkerConfig,
	})

	// ── gRPC service implementation ───────────────────────────────────────────
	// resourceRepo, cuRepo, pointRepo are passed explicitly because the gRPC
	// batch handlers (BatchCreateResources / CUs / Points) call repo methods
	// directly rather than going through CQRS command handlers.
	resourceSvc := grpcpkg.NewServer(app, resourceRepo, cuRepo, pointRepo)

	grpcSrv := platformserver.NewGRPCServer()
	resourcepb.RegisterResourceServiceServer(grpcSrv, resourceSvc)

	// ── HTTP layer (gin + grpc-gateway in-process) ────────────────────────────
	logger := logrus.NewEntry(logrus.StandardLogger())
	ginEngine := platformserver.NewGinEngine(cfg.ServiceName, logger)

	if err := gatewaypkg.MountGateway(context.Background(), ginEngine, resourceSvc); err != nil {
		metricsCancel()
		return nil, fmt.Errorf("mount grpc-gateway: %w", err)
	}

	httpSrv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ginEngine,
	}

	return &resourceServer{
		grpcSrv:       grpcSrv,
		httpSrv:       httpSrv,
		app:           app,
		cfg:           cfg,
		metricsClient: metricsClient,
		metricsCancel: metricsCancel,
	}, nil
}

// ─── lifecycle ────────────────────────────────────────────────────────────────

// PrepareRun returns a preparedServer. Add pre-flight checks (DB ping,
// readiness probe registration) here before calling Run.
func (s *resourceServer) PrepareRun() *preparedServer {
	return &preparedServer{s}
}

// Run starts all servers and blocks until a shutdown signal or a fatal server
// error, then performs an ordered graceful shutdown.
//
// Supervision model: gRPC, HTTP, and the metrics error watcher all run inside
// an errgroup. Any component returning a non-nil error — or a SIGINT/SIGTERM —
// cancels the shared context and triggers the shutdown coordinator.
//
// Shutdown order (so metrics stays live the longest):
//  1. Stop accepting new traffic  — gRPC GracefulStop + HTTP Shutdown
//  2. Drain background worker     — workerCancel
//  3. Stop metrics server         — metricsCancel
func (s *preparedServer) Run() error {
	// rootCtx is the single lifecycle context; cancelling it cascades to
	// workerCtx and the errgroup context simultaneously.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	workerCtx, workerCancel := context.WithCancel(rootCtx)
	defer workerCancel()

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

	// ── HTTP ──────────────────────────────────────────────────────────────────
	eg.Go(func() error {
		logrus.Infof("HTTP server listening on %s", s.cfg.HTTPAddr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	})

	// ── Metrics error watcher ─────────────────────────────────────────────────
	// Exits cleanly when egCtx is cancelled; propagates a real server error
	// into the errgroup so it triggers the same shutdown path as a signal.
	eg.Go(func() error {
		select {
		case err := <-s.metricsClient.Errors():
			return fmt.Errorf("metrics server: %w", err)
		case <-egCtx.Done():
			return nil
		}
	})

	// ── Signal watcher ────────────────────────────────────────────────────────
	// Cancels rootCtx (and therefore egCtx) on SIGINT / SIGTERM, which wakes
	// the shutdown coordinator below.
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

	// ── Import worker (not in errgroup — it manages its own stop via context) ─
	go s.app.Workers.ImportWorker.Start(workerCtx)

	// ── Shutdown coordinator ──────────────────────────────────────────────────
	// Fires as soon as egCtx is cancelled (signal or any server error).
	// Runs concurrently with eg.Wait() so that the goroutines above can return.
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-egCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 1. Stop accepting new traffic first.
		s.grpcSrv.GracefulStop()
		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			logrus.WithError(err).Error("HTTP graceful shutdown error")
		}

		// 2. Stop background worker after traffic drains.
		workerCancel()

		// 3. Stop metrics last — /metrics remains scrapeable during drain.
		s.metricsCancel()
	}()

	// Wait for all supervised goroutines to exit.
	if err := eg.Wait(); err != nil {
		logrus.WithError(err).Error("server stopped with error")
		<-shutdownDone
		return err
	}

	<-shutdownDone
	logrus.Info("server exited cleanly")
	return nil
}
