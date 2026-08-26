package alarm

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

	httppkg "github.com/mushroomyuan/vpp-backend/alarm/adapter/inbound/http"
	kafkasub "github.com/mushroomyuan/vpp-backend/alarm/adapter/inbound/kafka"
	"github.com/mushroomyuan/vpp-backend/alarm/adapter/outbound/notify"
	adapterpostgres "github.com/mushroomyuan/vpp-backend/alarm/adapter/outbound/postgres"
	"github.com/mushroomyuan/vpp-backend/alarm/application"
	"github.com/mushroomyuan/vpp-backend/alarm/config"
	alarmmetrics "github.com/mushroomyuan/vpp-backend/alarm/metrics"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type alarmServer struct {
	httpSrv              *http.Server
	cfg                  *config.Config
	metricsClient        *metrics.Client
	metricsCancel        context.CancelFunc
	dispatchConsumer     *kafkasub.DispatchConsumer
	soeConsumer          *kafkasub.SOEConsumer
	authzSyncer          *authz.Syncer
	authzAdmin           authz.PermissionAdmin
	authzCatalog         authz.Catalog
	authzRegisterCatalog bool
}

type preparedServer struct {
	*alarmServer
}

func createServer(appCfg *config.Config, dbCfg platformpostgres.Config) (*alarmServer, error) {
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

	pg := platformpostgres.NewPostgres(dbCfg)
	if sqlDB, err := pg.SQLDb(); err != nil {
		logrus.WithError(err).Warn("skipping DB metrics: could not obtain sql.DB")
	} else if err := metricsClient.RegisterCollector(
		metrics.NewDBCollector(sqlDB, dbCfg.Driver, "primary"),
	); err != nil {
		logrus.WithError(err).Warn("skipping DB metrics: collector registration failed")
	}

	alarmObs := alarmmetrics.New()
	if err := metricsClient.RegisterCollector(alarmObs.Collector()); err != nil {
		metricsCancel()
		return nil, fmt.Errorf("register alarm metrics: %w", err)
	}

	repo := adapterpostgres.NewAlarmRepository(pg)
	app := application.NewApplication(application.Dependencies{
		Repo:     repo,
		Notifier: notify.NewLogNotifier(),
		Rules:    &cfg.Rules,
		Metrics:  metricsClient,
		Observer: alarmObs,
	})

	if err := app.CalibrateOpenAlarms(context.Background()); err != nil {
		metricsCancel()
		return nil, err
	}

	dispatchConsumer := kafkasub.NewDispatchConsumer(kafkasub.DispatchConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.DispatchTopic,
		GroupID: cfg.Kafka.DispatchGroupID,
	}, app.Commands.IngestEvent, alarmObs)

	soeConsumer := kafkasub.NewSOEConsumer(kafkasub.SOEConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.SOETopic,
		GroupID: cfg.Kafka.SOEGroupID,
	}, app.Commands.IngestEvent, alarmObs)

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
			_ = dispatchConsumer.Close()
			_ = soeConsumer.Close()
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

	return &alarmServer{
		httpSrv:              httpSrv,
		cfg:                  cfg,
		metricsClient:        metricsClient,
		metricsCancel:        metricsCancel,
		dispatchConsumer:     dispatchConsumer,
		soeConsumer:          soeConsumer,
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

func (s *alarmServer) PrepareRun() *preparedServer {
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
		if err := s.dispatchConsumer.Run(egCtx); err != nil {
			return fmt.Errorf("dispatch consumer: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		if err := s.soeConsumer.Run(egCtx); err != nil {
			return fmt.Errorf("soe consumer: %w", err)
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

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			logrus.WithError(err).Error("HTTP graceful shutdown error")
		}
		if err := s.dispatchConsumer.Close(); err != nil {
			logrus.WithError(err).Warn("dispatch consumer close error")
		}
		if err := s.soeConsumer.Close(); err != nil {
			logrus.WithError(err).Warn("soe consumer close error")
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
