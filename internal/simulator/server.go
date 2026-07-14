package simulator

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

	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
	"github.com/mushroomyuan/vpp-backend/simulator/api"
	gatewayclient "github.com/mushroomyuan/vpp-backend/simulator/client/gateway"
	resourceclient "github.com/mushroomyuan/vpp-backend/simulator/client/resource"
	"github.com/mushroomyuan/vpp-backend/simulator/config"
	"github.com/mushroomyuan/vpp-backend/simulator/fault"
	"github.com/mushroomyuan/vpp-backend/simulator/runtime"
	"github.com/mushroomyuan/vpp-backend/simulator/telemetry"
	"github.com/mushroomyuan/vpp-backend/simulator/tick"
)

type simulatorServer struct {
	httpSrv       *http.Server
	cfg           *config.Config
	metricsClient *metrics.Client
	metricsCancel context.CancelFunc
	resourceCli   *resourceclient.Client
	tickEngine    *tick.Engine
	manager       *runtime.Manager
}

type preparedServer struct {
	*simulatorServer
}

func createServer(appCfg *config.Config) (*simulatorServer, error) {
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

	resourceCli, err := resourceclient.New(resourceclient.Config{Addr: appCfg.ResourceGRPCAddr})
	if err != nil {
		metricsCancel()
		return nil, fmt.Errorf("init resource client: %w", err)
	}

	gatewayCli, err := gatewayclient.New(gatewayclient.Config{BaseURL: appCfg.GatewayHTTPAddr})
	if err != nil {
		metricsCancel()
		_ = resourceCli.Close()
		return nil, fmt.Errorf("init gateway client: %w", err)
	}

	faults := fault.NewEngine()
	manager := runtime.NewManager(faults)

	load := func(ctx context.Context) error {
		specs, err := resourceCli.LoadDeviceSpecs(ctx, resourceclient.LoadFilter{
			TenantID:        appCfg.TenantID,
			SiteIDs:         appCfg.SiteIDs,
			CUIDs:           appCfg.CUIDs,
			RequireProvider: appCfg.RequireProvider,
		})
		if err != nil {
			return err
		}
		manager.Load(specs)
		logrus.Infof("loaded %d simulatable devices (provider=%q)", len(specs), appCfg.RequireProvider)
		return nil
	}

	// Retry: run-all starts services in parallel; Resource may not be ready yet.
	if err := loadWithRetry(load, 60*time.Second, 2*time.Second); err != nil {
		metricsCancel()
		_ = resourceCli.Close()
		return nil, fmt.Errorf("load devices from resource: %w", err)
	}

	var publisher *telemetry.Publisher
	if appCfg.PublishEnabled {
		publisher = telemetry.NewPublisher(appCfg.TenantID, gatewayCli, manager, faults)
		logrus.Infof("telemetry publish enabled interval=%s", appCfg.TickInterval)
	} else {
		logrus.Warn("telemetry publish disabled (runtime.publish-enabled=false); Tick still runs in-memory")
	}
	tickEngine := tick.NewEngine(appCfg.TickInterval, manager, publisher, appCfg.TraceSampleEvery)

	logger := logrus.NewEntry(logrus.StandardLogger())
	ginEngine := platformserver.NewGinEngine(appCfg.ServiceName, logger)
	api.RegisterRoutes(ginEngine, api.NewHandler(manager, faults, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return load(ctx)
	}))

	httpSrv := &http.Server{
		Addr:    appCfg.HTTPAddr,
		Handler: ginEngine,
	}

	return &simulatorServer{
		httpSrv:       httpSrv,
		cfg:           appCfg,
		metricsClient: metricsClient,
		metricsCancel: metricsCancel,
		resourceCli:   resourceCli,
		tickEngine:    tickEngine,
		manager:       manager,
	}, nil
}

// loadWithRetry calls load until it succeeds or the overall timeout elapses.
func loadWithRetry(load func(context.Context) error, overall time.Duration, interval time.Duration) error {
	deadline := time.Now().Add(overall)
	var lastErr error
	attempt := 0
	for {
		attempt++
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if lastErr != nil {
				return fmt.Errorf("timed out after %s (%d attempts): %w", overall, attempt-1, lastErr)
			}
			return fmt.Errorf("timed out after %s waiting for resource", overall)
		}
		ctxTimeout := 10 * time.Second
		if remaining < ctxTimeout {
			ctxTimeout = remaining
		}
		ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
		err := load(ctx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		logrus.WithError(err).Warnf("resource not ready (attempt %d), retrying in %s", attempt, interval)

		sleep := interval
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		if sleep <= 0 {
			return fmt.Errorf("timed out after %s (%d attempts): %w", overall, attempt, lastErr)
		}
		time.Sleep(sleep)
	}
}

func (s *simulatorServer) PrepareRun() *preparedServer {
	return &preparedServer{s}
}

func (s *preparedServer) Run() error {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	eg, egCtx := errgroup.WithContext(rootCtx)

	eg.Go(func() error {
		logrus.Infof("HTTP server listening on %s", s.cfg.HTTPAddr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		return s.tickEngine.Run(egCtx)
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

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			logrus.WithError(err).Error("HTTP graceful shutdown error")
		}
		if err := s.resourceCli.Close(); err != nil {
			logrus.WithError(err).Warn("resource client close error")
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
