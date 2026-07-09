package dispatch

import (
	"context"
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
	"github.com/mushroomyuan/vpp-backend/platform/metrics"
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

	dispatchSvc := grpcpkg.NewServer(app)
	grpcSrv := platformserver.NewGRPCServer()
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
	}, nil
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
