package port

import "time"

// Observer is process-local Forecast instrumentation. All methods must
// be nil-safe at the call site (a nil interface value is a no-op);
// concrete implementations (internal/forecast/metrics) are also
// nil-receiver safe.
//
// v1 records: batch-cycle duration, per-target success/failure, Predict
// duration/errors by algorithm_version, Telemetry QueryAggregation
// success/failure, and Redis/Postgres write success/failure
// (design plan §9).
type Observer interface {
	ObserveCycle(d time.Duration, err error)
	ObserveTarget(success bool)
	ObservePredict(algorithmVersion string, d time.Duration, err error)
	ObserveTelemetryQuery(success bool)
	ObservePostgresWrite(success bool)
	ObserveRedisWrite(success bool)
}
