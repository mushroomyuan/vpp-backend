package application

import (
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/command"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/query"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// Application is the composition root of the telemetry service's use-case layer.
// All business logic lives in Commands and Queries; the inbound adapter (gRPC)
// depends only on this struct.
type Application struct {
	Commands Commands
	Queries  Queries
}

type Commands struct {
	IngestTelemetry command.IngestTelemetryHandler
}

type Queries struct {
	// QueryTelemetry returns raw time-series records for a single CU.
	QueryTelemetry query.QueryTelemetryHandler
	// GetSnapshot returns the current real-time state for a single CU.
	GetSnapshot query.GetSnapshotHandler
	// GetFleetSnapshot returns real-time state for every CU in a tenant.
	GetFleetSnapshot query.GetFleetSnapshotHandler
	// QueryAggregation returns downsampled time-series data for a single CU metric.
	QueryAggregation query.QueryAggregationHandler
}

// Dependencies groups all outbound port implementations required by the
// application. The composition root (server wiring) assembles this struct
// from concrete adapter implementations.
type Dependencies struct {
	TelemetryRepo   port.TelemetryRepository
	SnapshotRepo    port.SnapshotRepository
	AggregationRepo port.AggregationRepository
	EventPublisher  port.EventPublisher

	// Metrics is optional; pass nil to disable metrics decoration.
	Metrics decorator.MetricsClient
}

func NewApplication(deps Dependencies) Application {
	if deps.TelemetryRepo == nil {
		panic("NewApplication: TelemetryRepo is required")
	}
	if deps.SnapshotRepo == nil {
		panic("NewApplication: SnapshotRepo is required")
	}
	if deps.AggregationRepo == nil {
		panic("NewApplication: AggregationRepo is required")
	}
	if deps.EventPublisher == nil {
		panic("NewApplication: EventPublisher is required")
	}

	return Application{
		Commands: Commands{
			IngestTelemetry: command.NewIngestTelemetryHandler(
				deps.TelemetryRepo,
				deps.SnapshotRepo,
				deps.EventPublisher,
				deps.Metrics,
			),
		},
		Queries: Queries{
			QueryTelemetry:   query.NewQueryTelemetryHandler(deps.TelemetryRepo, deps.Metrics),
			GetSnapshot:      query.NewGetSnapshotHandler(deps.SnapshotRepo, deps.Metrics),
			GetFleetSnapshot: query.NewGetFleetSnapshotHandler(deps.SnapshotRepo, deps.Metrics),
			QueryAggregation: query.NewQueryAggregationHandler(deps.AggregationRepo, deps.Metrics),
		},
	}
}
