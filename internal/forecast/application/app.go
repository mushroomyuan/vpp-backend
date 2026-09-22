package application

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/forecast/application/command"
	"github.com/mushroomyuan/vpp-backend/forecast/application/query"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// Application is the composition root of the Forecast use-case layer.
// The process-level composition root (server.go) depends on this struct
// to start ForecastLoop and to register inboundgrpc.Server.
type Application struct {
	Commands     Commands
	Queries      Queries
	ForecastLoop *command.ForecastLoop
}

// Commands groups the write-side handlers. v1's only command is the batch cycle.
type Commands struct {
	RunForecastCycle command.RunForecastCycleHandler
}

// Queries groups the read-side handlers behind ForecastService.
type Queries struct {
	GetLatestPrediction  query.GetLatestPredictionHandler
	QueryForecastHistory query.QueryForecastHistoryHandler
}

// Dependencies groups outbound ports and tunables. The composition root
// (server.go) assembles this from concrete adapters.
type Dependencies struct {
	Telemetry port.TelemetryPort
	History   port.HistoryPort
	Cache     port.CachePort

	// Registry nil uses service.DefaultRegistry (moving_average +
	// same_period_prior). Tests inject a narrower registry.
	Registry *service.Registry

	Targets       []model.ForecastTarget
	CycleInterval time.Duration
	HorizonSteps  int
	StepSeconds   int64
	HistoryWindow time.Duration

	// Metrics is optional; pass nil to disable decorator metrics (tests).
	// Production always passes the process-wide platform/metrics.Client.
	Metrics decorator.MetricsClient

	// Observer is optional; pass nil to disable Forecast-specific
	// Prometheus instruments. Production always passes *forecast/metrics.Metrics.
	Observer port.Observer
}

// NewApplication wires the cycle handler, read queries, and ticker loop.
// Panics if a required outbound port is missing.
func NewApplication(deps Dependencies) Application {
	if deps.Telemetry == nil {
		panic("NewApplication: Telemetry is required")
	}
	if deps.History == nil {
		panic("NewApplication: History is required")
	}
	if deps.Cache == nil {
		panic("NewApplication: Cache is required")
	}
	if deps.CycleInterval <= 0 {
		deps.CycleInterval = 15 * time.Minute
	}

	if enabledCount(deps.Targets) == 0 {
		logging.Warnf(context.Background(), logrus.Fields{
			"component": "ForecastLoop",
			"interval":  deps.CycleInterval.String(),
		}, "no enabled forecast targets configured; ticks will be no-ops")
	}

	handler := command.NewRunForecastCycleHandler(
		deps.Telemetry,
		deps.History,
		deps.Cache,
		deps.Registry,
		deps.Targets,
		deps.HorizonSteps,
		deps.StepSeconds,
		deps.HistoryWindow,
		deps.Metrics,
		deps.Observer,
	)

	return Application{
		Commands: Commands{
			RunForecastCycle: handler,
		},
		Queries: Queries{
			GetLatestPrediction:  query.NewGetLatestPredictionHandler(deps.Cache, deps.History, deps.Metrics),
			QueryForecastHistory: query.NewQueryForecastHistoryHandler(deps.History, deps.Metrics),
		},
		ForecastLoop: command.NewForecastLoop(handler, deps.CycleInterval),
	}
}

func enabledCount(targets []model.ForecastTarget) int {
	n := 0
	for _, t := range targets {
		if t.Enabled {
			n++
		}
	}
	return n
}
