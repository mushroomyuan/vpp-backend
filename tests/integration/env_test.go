// Package integration exercises the SubmitTask -> ExecuteCommand -> Kafka
// callback chain across the real dispatch and gateway application layers,
// wired to ephemeral Postgres + Kafka containers. Dispatch talks to gateway
// over a real gRPC server bound to an in-memory bufconn listener (no host
// ports, no network flakiness).
//
// Both tests in this package share a single environment (one Kafka
// container, one Postgres container per service) built once in TestMain:
// each container takes tens of seconds to become fully usable, so per-test
// containers would make the suite several times slower and needlessly
// contend for the local Docker daemon's resources. Isolation between tests
// is achieved via distinct TenantID/CUCode values, not distinct containers.
package integration

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	dispatchkafkain "github.com/mushroomyuan/vpp-backend/dispatch/adapter/inbound/kafka"
	dispatchgatewaygrpc "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/gateway_grpc"
	dispatchkafkaout "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/kafka"
	dispatchpg "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/postgres"
	dispatchapp "github.com/mushroomyuan/vpp-backend/dispatch/application"
	dispatchinfrapg "github.com/mushroomyuan/vpp-backend/dispatch/infrastructure/persistent/postgres"

	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	gatewayinboundgrpc "github.com/mushroomyuan/vpp-backend/gateway/adapter/inbound/grpc"
	gatewayemslog "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/ems_log"
	gatewaykafkaout "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/kafka"
	gatewaypg "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/postgres"
	gatewayapp "github.com/mushroomyuan/vpp-backend/gateway/application"
	gatewaymodel "github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	gatewayinfrapg "github.com/mushroomyuan/vpp-backend/gateway/infrastructure/persistent/postgres"

	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

const bufSize = 1024 * 1024

// noopMetrics satisfies decorator.MetricsClient without a running HTTP/
// prometheus server, which would otherwise collide across the two
// applications wired into the same test process.
type noopMetrics struct{}

func (noopMetrics) Count(string, string, string)           {}
func (noopMetrics) CountN(string, string, string, float64) {}
func (noopMetrics) Observe(string, string, time.Duration)  {}
func (noopMetrics) TrackInFlight(string, string) func()    { return func() {} }

// stubTelemetryClient satisfies gateway's port.TelemetryClient. The SubmitTask
// -> ExecuteCommand -> CommandCompleted chain exercised here never calls
// ReceiveTelemetry, so this is never invoked; it only exists to satisfy
// gatewayapp.NewApplication's non-nil dependency check.
type stubTelemetryClient struct{}

func (stubTelemetryClient) Ingest(context.Context, *gatewaymodel.StandardTelemetry) error {
	return nil
}

// env wires real dispatch + gateway application layers on top of ephemeral
// Postgres/Kafka containers, mirroring the composition roots in
// internal/dispatch/server.go and internal/gateway/server.go as closely as
// possible so the integration test exercises production wiring, not test doubles.
type env struct {
	Dispatch dispatchapp.Application
	Gateway  gatewayapp.Application
}

var sharedEnv *env

func TestMain(m *testing.M) {
	e, teardown, err := buildEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration test env setup failed:", err)
		os.Exit(1)
	}
	sharedEnv = e

	code := m.Run()
	teardown()
	os.Exit(code)
}

func buildEnv() (*env, func(), error) {
	ctx := context.Background()
	var closers []func()
	teardown := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	fail := func(err error) (*env, func(), error) {
		teardown()
		return nil, nil, err
	}

	dispatchDSN, dispatchClose, err := startPostgres(ctx, "dispatch", "../../migrations/dispatch/000001_init.up.sql")
	if err != nil {
		return fail(fmt.Errorf("start dispatch postgres: %w", err))
	}
	closers = append(closers, dispatchClose)

	gatewayDSN, gatewayClose, err := startPostgres(ctx, "gateway", "../../migrations/gateway/000001_init.up.sql")
	if err != nil {
		return fail(fmt.Errorf("start gateway postgres: %w", err))
	}
	closers = append(closers, gatewayClose)

	brokers, kafkaClose, err := startKafka(ctx)
	if err != nil {
		return fail(fmt.Errorf("start kafka: %w", err))
	}
	closers = append(closers, kafkaClose)

	// The KRaft single-node broker reports its listener port open (and
	// testcontainers' wait strategy succeeds) slightly before it can service
	// admin/metadata requests for topic creation and consumer group
	// coordination. Retrying CreateTopics rides out that window instead of
	// racing PublishTaskStarted's first write against broker readiness.
	commandTopic := "vpp.command.events"
	dispatchTopic := "vpp.dispatch.events"
	if err := ensureKafkaTopics(ctx, brokers, commandTopic, dispatchTopic); err != nil {
		return fail(fmt.Errorf("create kafka topics: %w", err))
	}

	// --- Gateway application (mapping repo + log-only EMS + real Kafka publisher) ---
	gwPG := gatewayinfrapg.NewPostgres(platformpostgres.Config{DSN: gatewayDSN})
	mappingRepo := gatewaypg.NewMappingRepositoryPostgres(gatewayinfrapg.NewMappingRepository(gwPG))
	commandEvents := gatewaykafkaout.NewCommandEventPublisher(gatewaykafkaout.CommandEventPublisherConfig{
		Brokers: brokers,
		Topic:   commandTopic,
	})
	closers = append(closers, func() { _ = commandEvents.Close() })

	gatewayApplication := gatewayapp.NewApplication(gatewayapp.Dependencies{
		MappingRepo:     mappingRepo,
		TelemetryClient: stubTelemetryClient{},
		EMSClient:       gatewayemslog.NewEMSLogClient(),
		CommandEvents:   commandEvents,
		Metrics:         noopMetrics{},
	})

	// --- Gateway gRPC server, served over an in-memory bufconn listener ---
	lis := bufconn.Listen(bufSize)
	grpcSrv := platformserver.NewGRPCServer()
	gatewaypb.RegisterGatewayServiceServer(grpcSrv, gatewayinboundgrpc.NewServer(gatewayApplication))
	go func() { _ = grpcSrv.Serve(lis) }()
	closers = append(closers, grpcSrv.Stop)

	bufDialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	gatewayClient, err := dispatchgatewaygrpc.NewClient(dispatchgatewaygrpc.Config{
		Addr:        "passthrough:///bufnet",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(bufDialer)},
	})
	if err != nil {
		return fail(fmt.Errorf("dial gateway over bufconn: %w", err))
	}
	closers = append(closers, func() { _ = gatewayClient.Close() })

	// --- Dispatch application (task/action/command repos + gRPC gateway client + real Kafka publisher) ---
	dpPG := dispatchinfrapg.NewPostgres(platformpostgres.Config{DSN: dispatchDSN})
	taskRepo := dispatchpg.NewTaskRepositoryPostgres(dispatchinfrapg.NewTaskRepository(dpPG))
	actionRepo := dispatchpg.NewActionRepositoryPostgres(dispatchinfrapg.NewActionRepository(dpPG))
	commandRepo := dispatchpg.NewCommandRepositoryPostgres(dispatchinfrapg.NewCommandRepository(dpPG))

	taskEvents := dispatchkafkaout.NewEventPublisher(dispatchkafkaout.Config{Brokers: brokers, Topic: dispatchTopic})
	closers = append(closers, func() { _ = taskEvents.Close() })

	dispatchApplication := dispatchapp.NewApplication(dispatchapp.Dependencies{
		TaskRepo:    taskRepo,
		ActionRepo:  actionRepo,
		CommandRepo: commandRepo,
		Gateway:     gatewayClient,
		Publisher:   taskEvents,
		Metrics:     noopMetrics{},
	})

	// --- Dispatch's Kafka consumer, driving HandleCommandResult from gateway's callback ---
	consumer := dispatchkafkain.NewCommandResultConsumer(
		dispatchkafkain.CommandResultConsumerConfig{Brokers: brokers, Topic: commandTopic},
		dispatchApplication.Commands.HandleCommandResult,
	)
	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	go func() { _ = consumer.Run(consumerCtx) }()
	closers = append(closers, func() {
		cancelConsumer()
		_ = consumer.Close()
	})

	return &env{Dispatch: dispatchApplication, Gateway: gatewayApplication}, teardown, nil
}

// startPostgres runs a fresh Postgres container seeded with the given
// migration file and returns a DSN string usable by gorm.io/driver/postgres.
func startPostgres(ctx context.Context, dbName, migrationFile string) (string, func(), error) {
	abs, err := filepath.Abs(migrationFile)
	if err != nil {
		return "", nil, err
	}

	c, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres123"),
		tcpostgres.WithInitScripts(abs),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start postgres container for %s: %w", dbName, err)
	}
	closeFn := func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres(%s) container: %v\n", dbName, err)
		}
	}

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		closeFn()
		return "", nil, err
	}
	return dsn, closeFn, nil
}

// startKafka runs a single-broker KRaft Kafka container and returns the
// host-reachable broker address(es).
func startKafka(ctx context.Context) ([]string, func(), error) {
	c, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.5.0", tckafka.WithClusterID("vpp-it"))
	if err != nil {
		return nil, nil, fmt.Errorf("start kafka container: %w", err)
	}
	closeFn := func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			fmt.Fprintf(os.Stderr, "terminate kafka container: %v\n", err)
		}
	}

	brokers, err := c.Brokers(ctx)
	if err != nil {
		closeFn()
		return nil, nil, err
	}
	return brokers, closeFn, nil
}

// ensureKafkaTopics pre-creates topics on the controller broker, retrying
// for a few seconds while the just-started KRaft node finishes initialising
// its metadata/coordinator machinery.
func ensureKafkaTopics(ctx context.Context, brokers []string, topics ...string) error {
	configs := make([]kafka.TopicConfig, len(topics))
	for i, topic := range topics {
		configs[i] = kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}
	}

	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := createTopicsOnce(ctx, brokers[0], configs); err != nil {
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}
		return nil
	}
	return fmt.Errorf("timed out creating kafka topics, last error: %w", lastErr)
}

func createTopicsOnce(ctx context.Context, addr string, configs []kafka.TopicConfig) error {
	conn, err := kafka.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}

	controllerConn, err := kafka.DialContext(ctx, "tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = controllerConn.Close() }()

	return controllerConn.CreateTopics(configs...)
}

func requireEventuallyf(t *testing.T, condition func() bool, msgAndArgs ...interface{}) {
	t.Helper()
	require.Eventually(t, condition, 20*time.Second, 200*time.Millisecond, msgAndArgs...)
}
