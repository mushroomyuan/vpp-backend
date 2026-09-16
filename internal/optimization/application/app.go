package application

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/optimization/application/command"
	appport "github.com/mushroomyuan/vpp-backend/optimization/application/port"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// Application is the composition root of the Optimization use-case layer.
// The process-level composition root (server.go) depends only on this
// struct to start the decision loop.
type Application struct {
	Commands     Commands
	DecisionLoop *command.DecisionLoop
}

// Commands groups the use-case handlers. v1 has a single command: one
// decision cycle. There is no inbound query surface.
type Commands struct {
	RunDecisionCycle command.RunDecisionCycleHandler
}

// Dependencies groups outbound ports and tunables. The composition root
// (server.go) assembles this from concrete adapters.
type Dependencies struct {
	Telemetry port.TelemetryPort
	Resource  port.ResourcePort
	Dispatch  appport.DispatchPort

	// Forecast is optional. v1 rules do not call it; the composition root
	// still constructs forecast_stub so the slot is occupied (design plan
	// §7). Reserved for a future forecast-dependent rule kind.
	Forecast port.ForecastPort

	Rules            model.Rules
	TenantIDs        []string
	DecisionInterval time.Duration // default 60s; must stay > Telemetry cycle
	DefaultCooldown  time.Duration // default 2× DecisionInterval

	// Metrics is optional; pass nil to disable decorator metrics (tests).
	// Production always passes the process-wide platform/metrics.Client.
	Metrics decorator.MetricsClient

	// Observer is optional; pass nil to disable Optimization-specific
	// Prometheus instruments (cycle duration, rules fired, SubmitTask,
	// ForecastPort). Production always passes *optimization/metrics.Metrics.
	Observer port.Observer
}

// NewApplication wires Evaluator, the decision-cycle handler, and the
// ticker loop. Panics if a required outbound port is missing.
func NewApplication(deps Dependencies) Application {
	if deps.Telemetry == nil {
		panic("NewApplication: Telemetry is required")
	}
	if deps.Resource == nil {
		panic("NewApplication: Resource is required")
	}
	if deps.Dispatch == nil {
		panic("NewApplication: Dispatch is required")
	}
	if deps.DecisionInterval <= 0 {
		deps.DecisionInterval = 60 * time.Second
	}
	if deps.DefaultCooldown <= 0 {
		deps.DefaultCooldown = 2 * deps.DecisionInterval
	}

	evaluator := service.NewEvaluator(deps.Rules, deps.Telemetry, deps.DefaultCooldown)
	handler := command.NewRunDecisionCycleHandler(evaluator, deps.Resource, deps.Dispatch, deps.Metrics, deps.Observer)

	return Application{
		Commands: Commands{
			RunDecisionCycle: handler,
		},
		DecisionLoop: command.NewDecisionLoop(handler, deps.DecisionInterval, deps.TenantIDs),
	}
}
