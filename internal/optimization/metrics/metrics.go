package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
)

var _ port.Observer = (*Metrics)(nil)

const (
	ResultOK             = "ok"
	ResultError          = "error"
	ResultSuccess        = "success"
	ResultFailure        = "failure"
	ResultNotImplemented = "not_implemented"
)

var cycleBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// Metrics holds Optimization-specific Prometheus instruments. Register
// Collector() on the shared platform/metrics registry. Methods are nil-safe.
type Metrics struct {
	cycleDuration *prometheus.HistogramVec
	cycleTotal    *prometheus.CounterVec
	rulesFired    *prometheus.CounterVec
	submitTotal   *prometheus.CounterVec
	forecastTotal *prometheus.CounterVec
	collectors    []prometheus.Collector
}

func New() *Metrics {
	m := &Metrics{
		cycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "optimization_decision_cycle_duration_seconds",
			Help:    "Wall time of one per-tenant decision cycle (evaluate + allocate + submit).",
			Buckets: cycleBuckets,
		}, []string{}),
		cycleTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "optimization_decision_cycle_total",
			Help: "Decision cycles finished. result=error if any rule/allocate/submit failed (partial success still counts as error).",
		}, []string{"result"}),
		rulesFired: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "optimization_rules_fired_total",
			Help: "Targets produced by the rule engine, partitioned by rule_id. Incremented even if a later Allocate/SubmitTask fails.",
		}, []string{"rule_id"}),
		submitTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "optimization_submit_task_total",
			Help: "Dispatch SubmitTask attempts from a decision cycle.",
		}, []string{"result"}),
		forecastTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "optimization_forecast_calls_total",
			Help: "ForecastPort.GetLatestPrediction calls. v1 is always not_implemented; the series is seeded so a future rule does not start from a missing metric.",
		}, []string{"result"}),
	}
	m.collectors = []prometheus.Collector{
		m.cycleDuration,
		m.cycleTotal,
		m.rulesFired,
		m.submitTotal,
		m.forecastTotal,
	}
	m.cycleTotal.WithLabelValues(ResultOK).Add(0)
	m.cycleTotal.WithLabelValues(ResultError).Add(0)
	m.submitTotal.WithLabelValues(ResultSuccess).Add(0)
	m.submitTotal.WithLabelValues(ResultFailure).Add(0)
	m.forecastTotal.WithLabelValues(ResultOK).Add(0)
	m.forecastTotal.WithLabelValues(ResultError).Add(0)
	m.forecastTotal.WithLabelValues(ResultNotImplemented).Add(0)
	_ = m.rulesFired.WithLabelValues(string(model.RuleSOCThreshold))
	_ = m.cycleDuration.WithLabelValues()
	return m
}

// Collector exposes all Optimization instruments on the shared /metrics endpoint.
func (m *Metrics) Collector() prometheus.Collector {
	if m == nil {
		return collectorGroup(nil)
	}
	return collectorGroup(m.collectors)
}

func (m *Metrics) ObserveCycle(d time.Duration, err error) {
	if m == nil {
		return
	}
	m.cycleDuration.WithLabelValues().Observe(d.Seconds())
	result := ResultOK
	if err != nil {
		result = ResultError
	}
	m.cycleTotal.WithLabelValues(result).Inc()
}

func (m *Metrics) ObserveRulesFired(ruleID string, n int) {
	if m == nil || n <= 0 {
		return
	}
	if ruleID == "" {
		ruleID = "unknown"
	}
	m.rulesFired.WithLabelValues(ruleID).Add(float64(n))
}

func (m *Metrics) ObserveSubmit(success bool) {
	if m == nil {
		return
	}
	result := ResultSuccess
	if !success {
		result = ResultFailure
	}
	m.submitTotal.WithLabelValues(result).Inc()
}

func (m *Metrics) ObserveForecast(result string) {
	if m == nil {
		return
	}
	switch result {
	case ResultOK, ResultError, ResultNotImplemented:
	default:
		return
	}
	m.forecastTotal.WithLabelValues(result).Inc()
}

type collectorGroup []prometheus.Collector

func (g collectorGroup) Describe(ch chan<- *prometheus.Desc) {
	for _, c := range g {
		c.Describe(ch)
	}
}

func (g collectorGroup) Collect(ch chan<- prometheus.Metric) {
	for _, c := range g {
		c.Collect(ch)
	}
}
