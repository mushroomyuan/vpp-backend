package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

const (
	defaultHorizonSteps  = 4
	defaultStepSeconds   = int64(900)
	defaultHistoryWindow = 168 * time.Hour
)

// RunForecastCycle is one pass of "pull history → Predict → Postgres then
// Redis" over every enabled ForecastTarget. ForecastLoop issues one of
// these per tick. Per-target failures are joined onto the handler error
// and do not skip the remaining targets (design plan §4).
type RunForecastCycle struct {
	// Now is the tick time. Zero means time.Now(); the loop always fills
	// this so GeneratedAt and horizon alignment stay deterministic.
	Now time.Time
}

// RunForecastCycleResult is what one cycle produced.
type RunForecastCycleResult struct {
	TargetsAttempted int
	TargetsSucceeded int
	TargetsFailed    int
	// RedisWriteFailed counts targets whose Postgres write succeeded but
	// the subsequent cache write did not. Those are still Succeeded:
	// Postgres is the authority and GetLatestPrediction falls back.
	RedisWriteFailed int
}

// RunForecastCycleHandler is the decorated command handler for one cycle.
type RunForecastCycleHandler = decorator.CommandHandler[RunForecastCycle, *RunForecastCycleResult]

type runForecastCycleHandler struct {
	telemetry     port.TelemetryPort
	history       port.HistoryPort
	cache         port.CachePort
	registry      *service.Registry
	observer      port.Observer
	targets       []model.ForecastTarget
	horizonSteps  int
	stepSeconds   int64
	historyWindow time.Duration
}

// NewRunForecastCycleHandler builds the use-case handler. registry nil
// uses service.DefaultRegistry. metricsClient nil disables decorator
// metrics (tests); production always passes the process-wide client.
// observer nil disables Forecast-specific Prometheus instruments.
func NewRunForecastCycleHandler(
	telemetry port.TelemetryPort,
	history port.HistoryPort,
	cache port.CachePort,
	registry *service.Registry,
	targets []model.ForecastTarget,
	horizonSteps int,
	stepSeconds int64,
	historyWindow time.Duration,
	metricsClient decorator.MetricsClient,
	observer port.Observer,
) RunForecastCycleHandler {
	return newRunForecastCycleHandler(telemetry, history, cache, registry, targets, horizonSteps, stepSeconds, historyWindow, metricsClient, observer)
}

func newRunForecastCycleHandler(
	telemetry port.TelemetryPort,
	history port.HistoryPort,
	cache port.CachePort,
	registry *service.Registry,
	targets []model.ForecastTarget,
	horizonSteps int,
	stepSeconds int64,
	historyWindow time.Duration,
	metricsClient decorator.MetricsClient,
	observer port.Observer,
) RunForecastCycleHandler {
	if telemetry == nil {
		panic("NewRunForecastCycleHandler: telemetry is required")
	}
	if history == nil {
		panic("NewRunForecastCycleHandler: history is required")
	}
	if cache == nil {
		panic("NewRunForecastCycleHandler: cache is required")
	}
	if registry == nil {
		registry = service.DefaultRegistry()
	}
	if horizonSteps < 1 {
		horizonSteps = defaultHorizonSteps
	}
	if stepSeconds <= 0 {
		stepSeconds = defaultStepSeconds
	}
	if historyWindow <= 0 {
		historyWindow = defaultHistoryWindow
	}
	if metricsClient == nil {
		metricsClient = nopMetrics{}
	}
	copied := make([]model.ForecastTarget, len(targets))
	copy(copied, targets)
	return decorator.ApplyCommandDecorators[RunForecastCycle, *RunForecastCycleResult](
		runForecastCycleHandler{
			telemetry:     telemetry,
			history:       history,
			cache:         cache,
			registry:      registry,
			observer:      observer,
			targets:       copied,
			horizonSteps:  horizonSteps,
			stepSeconds:   stepSeconds,
			historyWindow: historyWindow,
		},
		metricsClient,
	)
}

func (h runForecastCycleHandler) Handle(ctx context.Context, cmd RunForecastCycle) (out *RunForecastCycleResult, err error) {
	start := time.Now()
	defer func() {
		if h.observer != nil {
			h.observer.ObserveCycle(time.Since(start), err)
		}
	}()

	now := cmd.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	future := alignedFutureBuckets(now, h.stepSeconds, h.horizonSteps)
	out = &RunForecastCycleResult{}
	var errs []error

	for _, target := range h.targets {
		if !target.Enabled {
			continue
		}
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		out.TargetsAttempted++
		fields := targetLogFields(target, now)
		redisFailed, targetErr := h.forecastTarget(ctx, target, now, future)
		if targetErr != nil {
			logging.Errorf(ctx, fields, "forecast target failed: %v", targetErr)
			out.TargetsFailed++
			if h.observer != nil {
				h.observer.ObserveTarget(false)
			}
			errs = append(errs, fmt.Errorf("%s/%s: %w", target.CUCode, target.MetricName, targetErr))
			continue
		}
		out.TargetsSucceeded++
		if h.observer != nil {
			h.observer.ObserveTarget(true)
		}
		if redisFailed {
			out.RedisWriteFailed++
		}
		fields["redis_write_failed"] = redisFailed
		logging.Infof(ctx, fields, "forecast target completed")
	}

	err = errors.Join(errs...)
	logging.InfoWithCost(ctx, logrus.Fields{
		"component":          "ForecastLoop",
		"targets_attempted":  out.TargetsAttempted,
		"targets_succeeded":  out.TargetsSucceeded,
		"targets_failed":     out.TargetsFailed,
		"redis_write_failed": out.RedisWriteFailed,
		"generated_at":       now.Format(time.RFC3339),
	}, start, "forecast cycle completed")
	return out, err
}

func (h runForecastCycleHandler) forecastTarget(
	ctx context.Context,
	target model.ForecastTarget,
	now time.Time,
	future []time.Time,
) (redisFailed bool, err error) {
	if err := requireTargetIdentity(target); err != nil {
		return false, err
	}
	predictor, ok := h.registry.Get(target.Algorithm)
	if !ok {
		return false, fmt.Errorf("unknown algorithm %q", target.Algorithm)
	}

	points, err := h.telemetry.QueryAggregation(
		ctx,
		target.TenantID, target.CUCode, target.MetricName,
		now.Add(-h.historyWindow), now, h.stepSeconds,
	)
	if h.observer != nil {
		h.observer.ObserveTelemetryQuery(err == nil)
	}
	if err != nil {
		return false, fmt.Errorf("QueryAggregation: %w", err)
	}

	history := historyFromAggregation(target, points, now)
	predStart := time.Now()
	predicted, err := predictor.Predict(ctx, history, future)
	if h.observer != nil {
		h.observer.ObservePredict(predictor.AlgorithmVersion(), time.Since(predStart), err)
	}
	if err != nil {
		return false, fmt.Errorf("Predict: %w", err)
	}

	batch := make([]model.Prediction, len(predicted))
	version := predictor.AlgorithmVersion()
	for i, p := range predicted {
		batch[i] = model.Prediction{
			TenantID:         target.TenantID,
			CUCode:           target.CUCode,
			MetricName:       target.MetricName,
			GeneratedAt:      now,
			TargetTimestamp:  p.TargetTimestamp.UTC(),
			PredictedValue:   p.Value,
			AlgorithmVersion: version,
		}
	}

	if err := h.history.SaveBatch(ctx, batch); err != nil {
		if h.observer != nil {
			h.observer.ObservePostgresWrite(false)
		}
		return false, fmt.Errorf("SaveBatch: %w", err)
	}
	if h.observer != nil {
		h.observer.ObservePostgresWrite(true)
	}
	if err := h.cache.SetLatestBatch(ctx, batch); err != nil {
		if h.observer != nil {
			h.observer.ObserveRedisWrite(false)
		}
		logging.Errorf(ctx, targetLogFields(target, now), "SetLatestBatch failed (Postgres already written): %v", err)
		return true, nil
	}
	if h.observer != nil {
		h.observer.ObserveRedisWrite(true)
	}
	return false, nil
}

func requireTargetIdentity(t model.ForecastTarget) error {
	if t.TenantID == "" || t.CUCode == "" || t.MetricName == "" {
		return fmt.Errorf("tenant_id, cu_code, and metric_name are required")
	}
	return nil
}

func targetLogFields(t model.ForecastTarget, generatedAt time.Time) logrus.Fields {
	return logrus.Fields{
		"component":    "ForecastLoop",
		"tenant_id":    t.TenantID,
		"cu_code":      t.CUCode,
		"metric_name":  t.MetricName,
		"algorithm":    string(t.Algorithm),
		"generated_at": generatedAt.UTC().Format(time.RFC3339),
	}
}

type nopMetrics struct{}

func (nopMetrics) Count(string, string, string)           {}
func (nopMetrics) CountN(string, string, string, float64) {}
func (nopMetrics) Observe(string, string, time.Duration)  {}
func (nopMetrics) TrackInFlight(string, string) func()    { return func() {} }
