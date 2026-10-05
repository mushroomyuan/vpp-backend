// Package metrics holds Decision Prometheus instruments.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	ResultOK    = "ok"
	ResultError = "error"
)

var cycleBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// Metrics records one decision pass. Methods are nil-safe.
type Metrics struct {
	cycleDuration *prometheus.HistogramVec
	cycleTotal    *prometheus.CounterVec
	plans         *prometheus.CounterVec
	policies      *prometheus.CounterVec
	scopeResolve  *prometheus.CounterVec
	collectors    []prometheus.Collector
}

// New registers decision-specific series. Dispatch is not counted in this slice.
func New() *Metrics {
	m := &Metrics{
		cycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "decision_cycle_duration_seconds",
			Help:    "Wall time of one decision pass over enabled policies.",
			Buckets: cycleBuckets,
		}, []string{}),
		cycleTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "decision_cycle_total",
			Help: "Decision passes finished. result=error when any policy failed. Skipped policies still count as ok.",
		}, []string{"result"}),
		plans: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "decision_plans_total",
			Help: "Plans produced by a decision pass, partitioned by feasibility.",
		}, []string{"feasibility"}),
		policies: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "decision_policy_outcomes_total",
			Help: "Per-policy outcome of a decision pass. fired means a plan was built. Other values are skips.",
		}, []string{"result"}),
		scopeResolve: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "decision_scope_resolve_total",
			Help: "Scope cache outcomes. skipped means Resource failed and the last-known-good copy was too old.",
		}, []string{"result"}),
	}
	m.collectors = []prometheus.Collector{
		m.cycleDuration, m.cycleTotal, m.plans, m.policies, m.scopeResolve,
	}
	m.cycleTotal.WithLabelValues(ResultOK).Add(0)
	m.cycleTotal.WithLabelValues(ResultError).Add(0)
	for _, feasibility := range []string{"feasible", "partially_feasible", "infeasible"} {
		m.plans.WithLabelValues(feasibility).Add(0)
	}
	for _, result := range []string{
		"fired", "in_band", "cooldown", "empty_scope", "invalid_capability",
		"unusable_state", "invalid_scope", "scope_unavailable", "precheck", "error",
	} {
		m.policies.WithLabelValues(result).Add(0)
	}
	for _, result := range []string{"cache", "fresh", "fallback", "skipped"} {
		m.scopeResolve.WithLabelValues(result).Add(0)
	}
	_ = m.cycleDuration.WithLabelValues()
	return m
}

// Collector exposes the decision instruments on the shared /metrics endpoint.
func (m *Metrics) Collector() prometheus.Collector {
	if m == nil {
		return collectorGroup(nil)
	}
	return collectorGroup(m.collectors)
}

// ObserveCycle records one pass.
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

// ObservePlan records one plan produced by the pass.
func (m *Metrics) ObservePlan(feasibility string) {
	if m == nil || feasibility == "" {
		return
	}
	m.plans.WithLabelValues(feasibility).Inc()
}

// ObservePolicy records one policy outcome.
func (m *Metrics) ObservePolicy(result string) {
	if m == nil || result == "" {
		return
	}
	m.policies.WithLabelValues(result).Inc()
}

// ObserveScopeResolve records one scope-cache outcome.
func (m *Metrics) ObserveScopeResolve(result string) {
	if m == nil || result == "" {
		return
	}
	m.scopeResolve.WithLabelValues(result).Inc()
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
