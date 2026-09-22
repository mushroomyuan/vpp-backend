package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
)

var _ port.Observer = (*Metrics)(nil)

const (
	ResultOK      = "ok"
	ResultError   = "error"
	ResultSuccess = "success"
	ResultFailure = "failure"
)

var (
	cycleBuckets   = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}
	predictBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}
)

// Metrics holds Forecast-specific Prometheus instruments. Register
// Collector() on the shared platform/metrics registry. Methods are nil-safe.
type Metrics struct {
	cycleDuration   *prometheus.HistogramVec
	cycleTotal      *prometheus.CounterVec
	targetsTotal    *prometheus.CounterVec
	predictDuration *prometheus.HistogramVec
	predictTotal    *prometheus.CounterVec
	telemetryTotal  *prometheus.CounterVec
	postgresTotal   *prometheus.CounterVec
	redisTotal      *prometheus.CounterVec
	collectors      []prometheus.Collector
}

func New() *Metrics {
	m := &Metrics{
		cycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "forecast_cycle_duration_seconds",
			Help:    "Wall time of one ForecastLoop tick (every enabled target: pull, predict, write).",
			Buckets: cycleBuckets,
		}, []string{}),
		cycleTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forecast_cycle_total",
			Help: "Forecast cycles finished. result=error if any target failed (partial success still counts as error).",
		}, []string{"result"}),
		targetsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forecast_targets_total",
			Help: "Per-target outcomes inside a cycle. Redis write failure still counts as success (Postgres is the authority).",
		}, []string{"result"}),
		predictDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "forecast_predict_duration_seconds",
			Help:    "Predictor.Predict wall time, partitioned by algorithm_version.",
			Buckets: predictBuckets,
		}, []string{"algorithm_version"}),
		predictTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forecast_predict_total",
			Help: "Predictor.Predict calls, partitioned by algorithm_version and result.",
		}, []string{"algorithm_version", "result"}),
		telemetryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forecast_telemetry_query_total",
			Help: "Telemetry QueryAggregation attempts from a forecast cycle.",
		}, []string{"result"}),
		postgresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forecast_postgres_write_total",
			Help: "forecast_history SaveBatch attempts from a forecast cycle.",
		}, []string{"result"}),
		redisTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "forecast_redis_write_total",
			Help: "Redis SetLatestBatch attempts. Only counted after Postgres succeeded.",
		}, []string{"result"}),
	}
	m.collectors = []prometheus.Collector{
		m.cycleDuration,
		m.cycleTotal,
		m.targetsTotal,
		m.predictDuration,
		m.predictTotal,
		m.telemetryTotal,
		m.postgresTotal,
		m.redisTotal,
	}
	m.cycleTotal.WithLabelValues(ResultOK).Add(0)
	m.cycleTotal.WithLabelValues(ResultError).Add(0)
	m.targetsTotal.WithLabelValues(ResultSuccess).Add(0)
	m.targetsTotal.WithLabelValues(ResultFailure).Add(0)
	m.telemetryTotal.WithLabelValues(ResultSuccess).Add(0)
	m.telemetryTotal.WithLabelValues(ResultFailure).Add(0)
	m.postgresTotal.WithLabelValues(ResultSuccess).Add(0)
	m.postgresTotal.WithLabelValues(ResultFailure).Add(0)
	m.redisTotal.WithLabelValues(ResultSuccess).Add(0)
	m.redisTotal.WithLabelValues(ResultFailure).Add(0)
	_ = m.cycleDuration.WithLabelValues()
	for _, ver := range []string{string(model.AlgorithmMovingAverage), string(model.AlgorithmSamePeriodPrior)} {
		_ = m.predictDuration.WithLabelValues(ver)
		m.predictTotal.WithLabelValues(ver, ResultSuccess).Add(0)
		m.predictTotal.WithLabelValues(ver, ResultFailure).Add(0)
	}
	return m
}

// Collector exposes all Forecast instruments on the shared /metrics endpoint.
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

func (m *Metrics) ObserveTarget(success bool) {
	if m == nil {
		return
	}
	m.targetsTotal.WithLabelValues(writeResult(success)).Inc()
}

func (m *Metrics) ObservePredict(algorithmVersion string, d time.Duration, err error) {
	if m == nil {
		return
	}
	if algorithmVersion == "" {
		algorithmVersion = "unknown"
	}
	m.predictDuration.WithLabelValues(algorithmVersion).Observe(d.Seconds())
	m.predictTotal.WithLabelValues(algorithmVersion, writeResult(err == nil)).Inc()
}

func (m *Metrics) ObserveTelemetryQuery(success bool) {
	if m == nil {
		return
	}
	m.telemetryTotal.WithLabelValues(writeResult(success)).Inc()
}

func (m *Metrics) ObservePostgresWrite(success bool) {
	if m == nil {
		return
	}
	m.postgresTotal.WithLabelValues(writeResult(success)).Inc()
}

func (m *Metrics) ObserveRedisWrite(success bool) {
	if m == nil {
		return
	}
	m.redisTotal.WithLabelValues(writeResult(success)).Inc()
}

func writeResult(success bool) string {
	if success {
		return ResultSuccess
	}
	return ResultFailure
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
