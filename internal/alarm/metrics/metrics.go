package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
)

var _ port.Observer = (*Metrics)(nil)

// Label values for alarm_ingest_total. Ingest uses this single Counter;
// do not add alarm_ingest_poison_total / dropped_total / retry_total.
const (
	SourceDispatch = string(model.SourceDispatch)
	SourceSOE      = string(model.SourceSOE)

	ResultOK       = "ok"
	ResultDedupHit = "dedup_hit"
	ResultDropped  = "dropped"
	ResultPoison   = "poison"
	ResultRetry    = "retry"

	ReasonNone                 = "none"
	ReasonRule                 = "rule"
	ReasonDecode               = "decode"
	ReasonDB                   = "db"
	ReasonUnique               = "unique"
	ReasonFingerprintCollision = "fingerprint_collision"
	ReasonTransient            = "transient"
)

var ingestBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0}

// Metrics holds alarm-specific Prometheus instruments. Register Collector()
// on the shared platform/metrics registry. Methods are nil-safe.
type Metrics struct {
	ingestTotal    *prometheus.CounterVec
	ingestDuration *prometheus.HistogramVec
	openAlarms     *prometheus.GaugeVec
	ackConflict    prometheus.Counter
	closeConflict  prometheus.Counter
	consumerLag    *prometheus.GaugeVec
	consumerMsgs   *prometheus.CounterVec
	consumerErrors *prometheus.CounterVec
	collectors     []prometheus.Collector
}

func New() *Metrics {
	m := &Metrics{
		ingestTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alarm_ingest_total",
			Help: "Alarm ingest outcomes. One counter; partition by source, result, and reason. Poison vs fingerprint_collision are different reasons.",
		}, []string{"source", "result", "reason"}),
		ingestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "alarm_ingest_duration_seconds",
			Help:    "Time to classify and persist one Kafka message.",
			Buckets: ingestBuckets,
		}, []string{"source"}),
		openAlarms: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "alarm_open_alarms",
			Help: "Process-local count of non-closed alarms by source. Ingest new-open +1, close -1, merge unchanged. Calibrated at process start, not on every scrape.",
		}, []string{"source"}),
		ackConflict: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "alarm_ack_conflict_total",
			Help: "Optimistic-lock conflicts on ack (HTTP 409).",
		}),
		closeConflict: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "alarm_close_conflict_total",
			Help: "Optimistic-lock conflicts on close (HTTP 409).",
		}),
		consumerLag: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "alarm_consumer_lag",
			Help: "Last-fetch high-watermark lag from kafka-go Reader.Stats. Not consumer-group committed lag (ReadLag is unavailable with group consumers).",
		}, []string{"source"}),
		consumerMsgs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alarm_consumer_messages_total",
			Help: "Kafka messages fetched by the alarm consumer.",
		}, []string{"source"}),
		consumerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alarm_consumer_handler_errors_total",
			Help: "Kafka fetch/commit failures and transient ingest retries.",
		}, []string{"source"}),
	}
	m.collectors = []prometheus.Collector{
		m.ingestTotal,
		m.ingestDuration,
		m.openAlarms,
		m.ackConflict,
		m.closeConflict,
		m.consumerLag,
		m.consumerMsgs,
		m.consumerErrors,
	}
	for _, src := range []string{SourceDispatch, SourceSOE} {
		m.openAlarms.WithLabelValues(src).Set(0)
		m.consumerLag.WithLabelValues(src).Set(0)
		m.consumerMsgs.WithLabelValues(src).Add(0)
		m.consumerErrors.WithLabelValues(src).Add(0)
		_ = m.ingestDuration.WithLabelValues(src)
	}
	return m
}

// Collector exposes all alarm instruments on the shared /metrics endpoint.
func (m *Metrics) Collector() prometheus.Collector {
	if m == nil {
		return collectorGroup(nil)
	}
	return collectorGroup(m.collectors)
}

func (m *Metrics) ObserveIngest(source, result, reason string, d time.Duration) {
	if m == nil {
		return
	}
	if !knownSource(source) || result == "" {
		return
	}
	if reason == "" {
		reason = ReasonNone
	}
	m.ingestTotal.WithLabelValues(source, result, reason).Inc()
	m.ingestDuration.WithLabelValues(source).Observe(d.Seconds())
}

func (m *Metrics) AlarmOpened(source string) {
	if m == nil || !knownSource(source) {
		return
	}
	m.openAlarms.WithLabelValues(source).Inc()
}

func (m *Metrics) AlarmClosed(source string) {
	if m == nil || !knownSource(source) {
		return
	}
	m.openAlarms.WithLabelValues(source).Dec()
}

func (m *Metrics) SetOpenCount(source string, n int) {
	if m == nil || !knownSource(source) {
		return
	}
	if n < 0 {
		n = 0
	}
	m.openAlarms.WithLabelValues(source).Set(float64(n))
}

func (m *Metrics) AckConflict() {
	if m == nil {
		return
	}
	m.ackConflict.Inc()
}

func (m *Metrics) CloseConflict() {
	if m == nil {
		return
	}
	m.closeConflict.Inc()
}

func (m *Metrics) IncConsumerMessages(source string) {
	if m == nil || !knownSource(source) {
		return
	}
	m.consumerMsgs.WithLabelValues(source).Inc()
}

func (m *Metrics) IncConsumerHandlerErrors(source string) {
	if m == nil || !knownSource(source) {
		return
	}
	m.consumerErrors.WithLabelValues(source).Inc()
}

func (m *Metrics) SetConsumerLag(source string, lag int64) {
	if m == nil || !knownSource(source) {
		return
	}
	if lag < 0 {
		lag = 0
	}
	m.consumerLag.WithLabelValues(source).Set(float64(lag))
}

func knownSource(source string) bool {
	return source == SourceDispatch || source == SourceSOE
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
