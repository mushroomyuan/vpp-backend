package application

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/dispatch/application/command"
	appport "github.com/mushroomyuan/vpp-backend/dispatch/application/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/application/query"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// Application is the composition root of the dispatch use-case layer.
// Inbound adapters (gRPC, Kafka consumer) depend only on this struct.
type Application struct {
	Commands       Commands
	Queries        Queries
	TimeoutScanner *command.TimeoutScanner
}

type Commands struct {
	SubmitTask          command.SubmitTaskHandler
	CancelTask          command.CancelTaskHandler
	HandleCommandResult command.HandleCommandResultHandler
}

type Queries struct {
	GetTask query.GetTaskHandler
}

// Dependencies groups all outbound port implementations required by the
// application. The composition root (server.go) assembles this from concrete
// adapter implementations.
type Dependencies struct {
	TaskRepo    port.TaskRepository
	ActionRepo  port.ActionRepository
	CommandRepo port.CommandRepository
	Gateway     appport.GatewayPort
	Publisher   port.TaskEventPublisher

	// Metrics is optional; pass nil to disable metrics decoration.
	Metrics decorator.MetricsClient

	TimeoutScanInterval   time.Duration // default 10s
	DefaultCommandTimeout time.Duration // default 30s
	DefaultMaxRetries     int           // default 3
}

func NewApplication(deps Dependencies) Application {
	if deps.TaskRepo == nil {
		panic("NewApplication: TaskRepo is required")
	}
	if deps.ActionRepo == nil {
		panic("NewApplication: ActionRepo is required")
	}
	if deps.CommandRepo == nil {
		panic("NewApplication: CommandRepo is required")
	}
	if deps.Gateway == nil {
		panic("NewApplication: Gateway is required")
	}
	if deps.Publisher == nil {
		panic("NewApplication: Publisher is required")
	}
	if deps.TimeoutScanInterval <= 0 {
		deps.TimeoutScanInterval = 10 * time.Second
	}
	if deps.DefaultCommandTimeout <= 0 {
		deps.DefaultCommandTimeout = 30 * time.Second
	}
	if deps.DefaultMaxRetries <= 0 {
		deps.DefaultMaxRetries = 3
	}

	dispatcher := service.NewDispatcher()
	validator := service.NewValidator()

	return Application{
		Commands: Commands{
			SubmitTask: command.NewSubmitTaskHandler(
				deps.TaskRepo,
				deps.ActionRepo,
				deps.CommandRepo,
				deps.Gateway,
				deps.Publisher,
				dispatcher,
				validator,
				deps.DefaultCommandTimeout,
				deps.DefaultMaxRetries,
				deps.Metrics,
			),
			CancelTask: command.NewCancelTaskHandler(
				deps.TaskRepo,
				deps.ActionRepo,
				deps.CommandRepo,
				deps.Gateway,
				deps.Publisher,
				dispatcher,
				deps.Metrics,
			),
			HandleCommandResult: command.NewHandleCommandResultHandler(
				deps.TaskRepo,
				deps.ActionRepo,
				deps.CommandRepo,
				deps.Gateway,
				deps.Publisher,
				dispatcher,
				deps.Metrics,
			),
		},
		Queries: Queries{
			GetTask: query.NewGetTaskHandler(deps.TaskRepo, deps.Metrics),
		},
		TimeoutScanner: command.NewTimeoutScanner(
			deps.TaskRepo,
			deps.ActionRepo,
			deps.CommandRepo,
			deps.Gateway,
			deps.Publisher,
			dispatcher,
			deps.TimeoutScanInterval,
		),
	}
}
