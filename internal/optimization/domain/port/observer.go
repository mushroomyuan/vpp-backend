package port

import "time"

// Observer is process-local Optimization instrumentation. All methods must
// be nil-safe at the call site (a nil interface value is a no-op); concrete
// implementations (internal/optimization/metrics) are also nil-receiver safe.
//
// v1 records: decision-cycle duration, rules fired, SubmitTask
// success/failure, and ForecastPort calls (even though v1 rules never
// call Forecast — the stub is wrapped so the series exists the day a
// forecast-dependent rule lands).
type Observer interface {
	ObserveCycle(d time.Duration, err error)
	ObserveRulesFired(ruleID string, n int)
	ObserveSubmit(success bool)
	// ObserveForecast records one ForecastPort call. result is
	// "ok", "error", or "not_implemented".
	ObserveForecast(result string)
}
