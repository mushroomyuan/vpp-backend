package grpc

import (
	forecastpb "github.com/mushroomyuan/vpp-backend/api/forecast/proto/gen"
	"github.com/mushroomyuan/vpp-backend/forecast/application"
	"github.com/mushroomyuan/vpp-backend/forecast/application/query"
)

// Server implements forecastpb.ForecastServiceServer. Handlers come from
// the pre-wired application.Application (CQRS layer).
type Server struct {
	forecastpb.UnimplementedForecastServiceServer

	getLatest    query.GetLatestPredictionHandler
	queryHistory query.QueryForecastHistoryHandler
}

var _ forecastpb.ForecastServiceServer = (*Server)(nil)

// NewServer constructs a Server from a fully-wired application.Application.
func NewServer(app application.Application) *Server {
	if app.Queries.GetLatestPrediction == nil {
		panic("inbound grpc: GetLatestPrediction handler is required")
	}
	if app.Queries.QueryForecastHistory == nil {
		panic("inbound grpc: QueryForecastHistory handler is required")
	}
	return &Server{
		getLatest:    app.Queries.GetLatestPrediction,
		queryHistory: app.Queries.QueryForecastHistory,
	}
}
