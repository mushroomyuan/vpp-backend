// Package integration exercises three chains across the real application
// layers of dispatch, gateway, resource, and telemetry, wired to ephemeral
// Postgres/TimescaleDB/Redis/Kafka containers:
//
//  1. SubmitTask -> ExecuteCommand -> Kafka callback (dispatch <-> gateway).
//  2. TimeoutScanner circuit-breaking a stuck Sending command.
//  3. Resource lifecycle event -> gateway mapping disable (resource -> gateway).
//  4. ReceiveTelemetry -> IngestTelemetry -> TimescaleDB/Redis (gateway <-> telemetry).
//
// Dispatch talks to gateway, and gateway talks to telemetry, over real gRPC
// servers bound to in-memory bufconn listeners (no host ports, no network
// flakiness).
//
// All tests in this package share a single environment (one Kafka container,
// one Postgres/TimescaleDB container per service, one Redis container) built
// once in TestMain: each container takes tens of seconds to become fully
// usable, so per-test containers would make the suite several times slower
// and needlessly contend for the local Docker daemon's resources. Isolation
// between tests is achieved via distinct TenantID/CUCode values, not
// distinct containers.
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

	goredis "github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	dispatchkafkain "github.com/mushroomyuan/vpp-backend/dispatch/adapter/inbound/kafka"
	dispatchgatewaygrpc "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/gateway_grpc"
	dispatchkafkaout "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/kafka"
	dispatchpg "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/postgres"
	dispatchapp "github.com/mushroomyuan/vpp-backend/dispatch/application"
	dispatchport "github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	dispatchinfrapg "github.com/mushroomyuan/vpp-backend/dispatch/infrastructure/persistent/postgres"

	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	gatewayinboundgrpc "github.com/mushroomyuan/vpp-backend/gateway/adapter/inbound/grpc"
	gatewaylifecyclekafka "github.com/mushroomyuan/vpp-backend/gateway/adapter/inbound/kafka"
	gatewayemslog "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/ems_log"
	gatewaykafkaout "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/kafka"
	gatewaypg "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/postgres"
	gatewaytelemetrygrpc "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/telemetry_grpc"
	gatewayapp "github.com/mushroomyuan/vpp-backend/gateway/application"
	gatewayinfrapg "github.com/mushroomyuan/vpp-backend/gateway/infrastructure/persistent/postgres"

	resourcekafkaout "github.com/mushroomyuan/vpp-backend/resource/adapter/outbound/kafka"
	resourceport "github.com/mushroomyuan/vpp-backend/resource/domain/port"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	telemetryinboundgrpc "github.com/mushroomyuan/vpp-backend/telemetry/adapter/inbound/grpc"
	telemetrykafkaout "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/kafka"
	telemetryredis "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/redis"
	telemetrytimescaledb "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/timescaledb"
	telemetryapp "github.com/mushroomyuan/vpp-backend/telemetry/application"

	resourceEventConst "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

const bufSize = 1024 * 1024

// noopMetrics satisfies decorator.MetricsClient without a running HTTP/
// prometheus server, which would otherwise collide across the applications
// wired into the same test process.
type noopMetrics struct{}

func (noopMetrics) Count(string, string, string)           {}
func (noopMetrics) CountN(string, string, string, float64) {}
func (noopMetrics) Observe(string, string, time.Duration)  {}
func (noopMetrics) TrackInFlight(string, string) func()    { return func() {} }

// env wires real dispatch + gateway + resource(publisher-only) + telemetry
// application layers on top of ephemeral Postgres/TimescaleDB/Redis/Kafka
// containers, mirroring the composition roots in each service's server.go as
// closely as possible so the integration tests exercise production wiring,
// not test doubles.
type env struct {
	Dispatch  dispatchapp.Application
	Gateway   gatewayapp.Application
	Telemetry telemetryapp.Application

	// TaskRepo is exposed so tests can seed a task tree directly (bypassing
	// SubmitTask) for scenarios that require pre-existing state, e.g. a
	// command already stuck in Sending past its deadline.
	TaskRepo dispatchport.TaskRepository

	// ResourceEvents lets tests act as the resource service and publish real
	// lifecycle events onto vpp.resource.events without standing up
	// resource's full domain/hierarchy.
	ResourceEvents resourceport.ResourceEventPublisher
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

	dispatchDSN, dispatchClose, err := startPostgres(ctx, "postgres:16-alpine", "dispatch", "../../migrations/dispatch/000001_init.up.sql")
	if err != nil {
		return fail(fmt.Errorf("start dispatch postgres: %w", err))
	}
	closers = append(closers, dispatchClose)

	gatewayDSN, gatewayClose, err := startPostgres(ctx, "postgres:16-alpine", "gateway", "../../migrations/gateway/000001_init.up.sql")
	if err != nil {
		return fail(fmt.Errorf("start gateway postgres: %w", err))
	}
	closers = append(closers, gatewayClose)

	// TimescaleDB is a strict superset of Postgres; the same testcontainers
	// module works, but the image is swapped and no init script is needed
	// since telemetry.ApplySchema (real production code) creates the schema.
	telemetryDSN, telemetryClose, err := startPostgres(ctx, "timescale/timescaledb:latest-pg16", "telemetry", "")
	if err != nil {
		return fail(fmt.Errorf("start telemetry timescaledb: %w", err))
	}
	closers = append(closers, telemetryClose)

	redisAddr, redisClose, err := startRedis(ctx)
	if err != nil {
		return fail(fmt.Errorf("start telemetry redis: %w", err))
	}
	closers = append(closers, redisClose)

	brokers, kafkaClose, err := startKafka(ctx)
	if err != nil {
		return fail(fmt.Errorf("start kafka: %w", err))
	}
	closers = append(closers, kafkaClose)

	// The KRaft single-node broker reports its listener port open (and
	// testcontainers' wait strategy succeeds) slightly before it can service
	// admin/metadata requests for topic creation and consumer group
	// coordination. Retrying CreateTopics rides out that window instead of
	// racing the first publish against broker readiness.
	commandTopic := "vpp.command.events"
	dispatchTopic := "vpp.dispatch.events"
	resourceTopic := resourceEventConst.TopicResourceEvents
	soeTopic := "vpp.soe.events"
	if err := ensureKafkaTopics(ctx, brokers, commandTopic, dispatchTopic, resourceTopic, soeTopic); err != nil {
		return fail(fmt.Errorf("create kafka topics: %w", err))
	}

	// --- Telemetry application (TimescaleDB + Redis + Kafka SOE publisher) ---
	tsPool, err := telemetrytimescaledb.NewPool(ctx, telemetrytimescaledb.Config{DSN: telemetryDSN})
	if err != nil {
		return fail(fmt.Errorf("connect telemetry timescaledb pool: %w", err))
	}
	closers = append(closers, tsPool.Close)

	if _, err := tsPool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS timescaledb"); err != nil {
		return fail(fmt.Errorf("enable timescaledb extension: %w", err))
	}
	if err := telemetrytimescaledb.ApplySchema(ctx, tsPool); err != nil {
		return fail(fmt.Errorf("apply telemetry schema: %w", err))
	}
	telemetryStore, aggregationStore := telemetrytimescaledb.NewStores(tsPool)

	redisOpts, err := goredis.ParseURL(redisAddr)
	if err != nil {
		return fail(fmt.Errorf("parse redis connection string: %w", err))
	}
	redisClient := goredis.NewClient(redisOpts)
	closers = append(closers, func() { _ = redisClient.Close() })
	snapshotStore := telemetryredis.NewSnapshotStore(redisClient, 0)

	soeEvents := telemetrykafkaout.NewEventPublisher(telemetrykafkaout.Config{Brokers: brokers, Topic: soeTopic})
	closers = append(closers, func() { _ = soeEvents.Close() })

	telemetryApplication := telemetryapp.NewApplication(telemetryapp.Dependencies{
		TelemetryRepo:   telemetryStore,
		SnapshotRepo:    snapshotStore,
		AggregationRepo: aggregationStore,
		EventPublisher:  soeEvents,
		Metrics:         noopMetrics{},
	})

	// --- Telemetry gRPC server, served over an in-memory bufconn listener ---
	telemetryLis := bufconn.Listen(bufSize)
	telemetryGRPCSrv := platformserver.NewGRPCServer()
	telemetrypb.RegisterTelemetryServiceServer(telemetryGRPCSrv, telemetryinboundgrpc.NewServer(telemetryApplication))
	go func() { _ = telemetryGRPCSrv.Serve(telemetryLis) }()
	closers = append(closers, telemetryGRPCSrv.Stop)

	telemetryBufDialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return telemetryLis.DialContext(ctx)
	}
	telemetryClient, err := gatewaytelemetrygrpc.NewTelemetryGRPCClient(gatewaytelemetrygrpc.Config{
		Addr:        "passthrough:///buftelemetry",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(telemetryBufDialer)},
	})
	if err != nil {
		return fail(fmt.Errorf("dial telemetry over bufconn: %w", err))
	}
	closers = append(closers, func() { _ = telemetryClient.Close() })

	// --- Gateway application (mapping repo + log-only EMS + real telemetry gRPC client + real Kafka publisher) ---
	gwPG := gatewayinfrapg.NewPostgres(platformpostgres.Config{DSN: gatewayDSN})
	mappingRepo := gatewaypg.NewMappingRepositoryPostgres(gatewayinfrapg.NewMappingRepository(gwPG))
	commandEvents := gatewaykafkaout.NewCommandEventPublisher(gatewaykafkaout.CommandEventPublisherConfig{
		Brokers: brokers,
		Topic:   commandTopic,
	})
	closers = append(closers, func() { _ = commandEvents.Close() })

	gatewayApplication := gatewayapp.NewApplication(gatewayapp.Dependencies{
		MappingRepo:     mappingRepo,
		TelemetryClient: telemetryClient,
		EMSClient:       gatewayemslog.NewEMSLogClient(),
		CommandEvents:   commandEvents,
		Metrics:         noopMetrics{},
	})

	// --- Gateway's lifecycle consumer, driving DisableMappingByCUCode from resource's events ---
	lifecycleConsumer := gatewaylifecyclekafka.NewLifecycleConsumer(
		gatewaylifecyclekafka.LifecycleConsumerConfig{Brokers: brokers, Topic: resourceTopic, GroupID: "it-gateway-lifecycle"},
		gatewayApplication.Commands.DisableMappingByCUCode,
	)
	lifecycleCtx, cancelLifecycle := context.WithCancel(context.Background())
	go func() { _ = lifecycleConsumer.Run(lifecycleCtx) }()
	closers = append(closers, func() {
		cancelLifecycle()
		_ = lifecycleConsumer.Close()
	})

	// resourceEvents lets tests act as the resource service's real Kafka
	// producer adapter, without needing resource's full domain/hierarchy.
	resourceEvents := resourcekafkaout.NewEventPublisher(resourcekafkaout.Config{Brokers: brokers, Topic: resourceTopic})
	closers = append(closers, func() { _ = resourceEvents.Close() })

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
		// Short interval so TestTimeoutScanner_* observes expired commands
		// quickly without slowing down the whole suite; safe to run
		// continuously alongside the other tests since FindExpiredSending
		// only matches commands whose deadline has already passed.
		TimeoutScanInterval: 500 * time.Millisecond,
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

	// --- Dispatch's TimeoutScanner, driving OnCommandTimeout for stuck Sending commands ---
	scannerCtx, cancelScanner := context.WithCancel(context.Background())
	go func() { _ = dispatchApplication.TimeoutScanner.Run(scannerCtx) }()
	closers = append(closers, cancelScanner)

	return &env{
		Dispatch:       dispatchApplication,
		Gateway:        gatewayApplication,
		Telemetry:      telemetryApplication,
		TaskRepo:       taskRepo,
		ResourceEvents: resourceEvents,
	}, teardown, nil
}

// startPostgres runs a fresh Postgres-compatible container seeded with the
// given migration file (skipped when empty) and returns a DSN string usable
// by gorm.io/driver/postgres and pgxpool alike.
func startPostgres(ctx context.Context, image, dbName, migrationFile string) (string, func(), error) {
	opts := []testcontainers.ContainerCustomizer{
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres123"),
		tcpostgres.BasicWaitStrategies(),
	}
	if migrationFile != "" {
		abs, err := filepath.Abs(migrationFile)
		if err != nil {
			return "", nil, err
		}
		opts = append(opts, tcpostgres.WithInitScripts(abs))
	}

	c, err := tcpostgres.Run(ctx, image, opts...)
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

// startRedis runs a fresh Redis container and returns its redis:// connection URI.
func startRedis(ctx context.Context) (string, func(), error) {
	c, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		return "", nil, fmt.Errorf("start redis container: %w", err)
	}
	closeFn := func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			fmt.Fprintf(os.Stderr, "terminate redis container: %v\n", err)
		}
	}

	uri, err := c.ConnectionString(ctx)
	if err != nil {
		closeFn()
		return "", nil, err
	}
	return uri, closeFn, nil
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
