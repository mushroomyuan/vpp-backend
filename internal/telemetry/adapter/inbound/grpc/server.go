package grpc

import (
	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/telemetry/application"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/command"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/query"
)

// Server implements telemetrypb.TelemetryServiceServer.
// Handlers are sourced from the pre-wired application.Application (CQRS layer).
type Server struct {
	telemetrypb.UnimplementedTelemetryServiceServer

	// command handlers
	ingestTelemetry command.IngestTelemetryHandler

	// query handlers
	queryTelemetry   query.QueryTelemetryHandler
	getSnapshot      query.GetSnapshotHandler
	getFleetSnapshot query.GetFleetSnapshotHandler
	queryAggregation query.QueryAggregationHandler
}

// NewServer constructs a Server from a fully-wired application.Application.
func NewServer(app application.Application) *Server {
	return &Server{
		ingestTelemetry:  app.Commands.IngestTelemetry,
		queryTelemetry:   app.Queries.QueryTelemetry,
		getSnapshot:      app.Queries.GetSnapshot,
		getFleetSnapshot: app.Queries.GetFleetSnapshot,
		queryAggregation: app.Queries.QueryAggregation,
	}
}
