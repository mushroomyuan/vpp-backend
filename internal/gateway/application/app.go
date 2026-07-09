package application

import (
	"github.com/mushroomyuan/vpp-backend/gateway/application/command"
	"github.com/mushroomyuan/vpp-backend/gateway/application/query"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// Application is the composition root of the gateway use-case layer.
// Inbound adapters (HTTP handlers, gRPC server) depend only on this struct.
type Application struct {
	Commands Commands
	Queries  Queries
}

type Commands struct {
	ReceiveTelemetry       command.ReceiveTelemetryHandler
	ExecuteCommand         command.ExecuteCommandHandler
	CreateMapping          command.CreateMappingHandler
	DeleteMapping          command.DeleteMappingHandler
	DisableMapping         command.DisableMappingHandler
	DisableMappingByCUCode command.DisableMappingByCUCodeHandler
}

type Queries struct {
	ListMappings query.ListMappingsHandler
}

// Dependencies groups all outbound port implementations required by the
// application. The composition root (server.go) assembles this from concrete
// adapter implementations.
type Dependencies struct {
	MappingRepo     port.MappingRepository
	TelemetryClient port.TelemetryClient
	EMSClient       port.EMSClient
	CommandEvents   port.CommandEventPublisher

	// Metrics is optional; pass nil to disable metrics decoration.
	Metrics decorator.MetricsClient
}

func NewApplication(deps Dependencies) Application {
	if deps.MappingRepo == nil {
		panic("NewApplication: MappingRepo is required")
	}
	if deps.TelemetryClient == nil {
		panic("NewApplication: TelemetryClient is required")
	}
	if deps.EMSClient == nil {
		panic("NewApplication: EMSClient is required")
	}
	if deps.CommandEvents == nil {
		panic("NewApplication: CommandEvents is required")
	}

	return Application{
		Commands: Commands{
			ReceiveTelemetry: command.NewReceiveTelemetryHandler(deps.MappingRepo, deps.TelemetryClient, deps.Metrics),
			ExecuteCommand: command.NewExecuteCommandHandler(
				deps.MappingRepo, deps.EMSClient, deps.CommandEvents, deps.Metrics,
			),
			CreateMapping:          command.NewCreateMappingHandler(deps.MappingRepo, deps.Metrics),
			DeleteMapping:          command.NewDeleteMappingHandler(deps.MappingRepo, deps.Metrics),
			DisableMapping:         command.NewDisableMappingHandler(deps.MappingRepo, deps.Metrics),
			DisableMappingByCUCode: command.NewDisableMappingByCUCodeHandler(deps.MappingRepo, deps.Metrics),
		},
		Queries: Queries{
			ListMappings: query.NewListMappingsHandler(deps.MappingRepo, deps.Metrics),
		},
	}
}
