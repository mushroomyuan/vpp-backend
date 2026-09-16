package optimization

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	dispatchgrpc "github.com/mushroomyuan/vpp-backend/optimization/adapter/outbound/dispatch_grpc"
	forecaststub "github.com/mushroomyuan/vpp-backend/optimization/adapter/outbound/forecast_stub"
	resourcegrpc "github.com/mushroomyuan/vpp-backend/optimization/adapter/outbound/resource_grpc"
	telemetrygrpc "github.com/mushroomyuan/vpp-backend/optimization/adapter/outbound/telemetry_grpc"
	"github.com/mushroomyuan/vpp-backend/optimization/application"
	"github.com/mushroomyuan/vpp-backend/optimization/application/command"
	"github.com/mushroomyuan/vpp-backend/optimization/config"
	optmetrics "github.com/mushroomyuan/vpp-backend/optimization/metrics"
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type optimizationServer struct {
	httpSrv       *http.Server
	cfg           *config.Config
	metricsClient *metrics.Client
	metricsCancel context.CancelFunc
	telemetry     *telemetrygrpc.Client
	resource      *resourcegrpc.Client
	dispatch      *dispatchgrpc.Client
	decisionLoop  *command.DecisionLoop
}

type preparedServer struct {
	*optimizationServer
}

func createServer(appCfg *config.Config) (*optimizationServer, error) {
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

	optObs := optmetrics.New()
	if err := metricsClient.RegisterCollector(optObs.Collector()); err != nil {
		metricsCancel()
		return nil, fmt.Errorf("register optimization metrics: %w", err)
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

	app := application.NewApplication(application.Dependencies{
		Telemetry:        telClient,
		Resource:         resClient,
		Dispatch:         disClient,
		Forecast:         forecaststub.NewObserved(forecaststub.New(), optObs),
		Rules:            cfg.Rules,
		TenantIDs:        cfg.TenantIDs,
		DecisionInterval: cfg.DecisionInterval,
		DefaultCooldown:  cfg.DefaultCooldown,
		Metrics:          metricsClient,
		Observer:         optObs,
	})

	logger := logrus.NewEntry(logrus.StandardLogger())
	ginEngine := platformserver.NewGinEngine(cfg.ServiceName, logger)
	httpSrv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ginEngine,
	}

	return &optimizationServer{
		httpSrv:       httpSrv,
		cfg:           cfg,
		metricsClient: metricsClient,
		metricsCancel: metricsCancel,
		telemetry:     telClient,
		resource:      resClient,
		dispatch:      disClient,
		decisionLoop:  app.DecisionLoop,
	}, nil
}

func (s *optimizationServer) PrepareRun() *preparedServer {
	return &preparedServer{s}
}

func (s *preparedServer) Run() error {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	eg, egCtx := errgroup.WithContext(rootCtx)

	eg.Go(func() error {
		logrus.Infof("HTTP server listening on %s (healthz only; no inbound business API)", s.cfg.HTTPAddr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		if err := s.decisionLoop.Run(egCtx); err != nil {
			return fmt.Errorf("decision loop: %w", err)
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

		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			logrus.WithError(err).Error("HTTP graceful shutdown error")
		}
		if err := s.telemetry.Close(); err != nil {
			logrus.WithError(err).Warn("telemetry gRPC client close error")
		}
		if err := s.resource.Close(); err != nil {
			logrus.WithError(err).Warn("resource gRPC client close error")
		}
		if err := s.dispatch.Close(); err != nil {
			logrus.WithError(err).Warn("dispatch gRPC client close error")
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
