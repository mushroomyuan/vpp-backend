package authz

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Decision result label values for authz_decision_total.
const (
	DecisionAllow         = "allow"
	DecisionDeny          = "deny"
	DecisionDegradedAllow = "degraded_allow"
	DecisionDegradedDeny  = "degraded_deny"
)

// Metrics holds Prometheus instruments for authz sync + decisions (C8).
type Metrics struct {
	service string

	syncLastSuccess prometheus.Gauge
	syncFailures    prometheus.Counter
	syncSuccesses   prometheus.Counter
	staleSeconds    prometheus.Gauge
	hasPolicies     prometheus.Gauge
	tier            *prometheus.GaugeVec
	decisions       *prometheus.CounterVec
	collectors      []prometheus.Collector
	lastLoggedTier  atomic.Int32 // Tier+1 so 0 means unset
}

// NewMetrics builds instruments labeled with the owning service name.
func NewMetrics(service string) *Metrics {
	if service == "" {
		service = "unknown"
	}
	constLabels := prometheus.Labels{"service": service}

	m := &Metrics{
		service: service,
		syncLastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "authz_policy_sync_last_success_timestamp",
			Help:        "Unix timestamp of the last successful authz policy sync.",
			ConstLabels: constLabels,
		}),
		syncFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "authz_policy_sync_failures_total",
			Help:        "Total number of failed authz policy sync attempts.",
			ConstLabels: constLabels,
		}),
		syncSuccesses: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "authz_policy_sync_successes_total",
			Help:        "Total number of successful authz policy sync attempts.",
			ConstLabels: constLabels,
		}),
		staleSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "authz_policy_stale_seconds",
			Help:        "Seconds since last successful authz policy sync; -1 if never synced.",
			ConstLabels: constLabels,
		}),
		hasPolicies: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "authz_policy_loaded",
			Help:        "1 if the local Casbin enforcer has at least one policy rule, else 0.",
			ConstLabels: constLabels,
		}),
		tier: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "authz_policy_tier",
			Help:        "Authz policy sync health tier (1 for the active tier).",
			ConstLabels: constLabels,
		}, []string{"tier"}),
		decisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "authz_decision_total",
			Help:        "Authz Allow decisions partitioned by result.",
			ConstLabels: constLabels,
		}, []string{"result"}),
	}
	m.collectors = []prometheus.Collector{
		m.syncLastSuccess,
		m.syncFailures,
		m.syncSuccesses,
		m.staleSeconds,
		m.hasPolicies,
		m.tier,
		m.decisions,
	}
	m.staleSeconds.Set(-1)
	m.setTierGauges(TierInvalid)
	return m
}

// Collector returns a single prometheus.Collector that exposes all authz metrics.
func (m *Metrics) Collector() prometheus.Collector {
	if m == nil {
		return nil
	}
	return collectorGroup(m.collectors)
}

// ObserveSyncSuccess records a successful sync at t.
func (m *Metrics) ObserveSyncSuccess(t time.Time, _ int) {
	if m == nil {
		return
	}
	m.syncSuccesses.Inc()
	if !t.IsZero() {
		m.syncLastSuccess.Set(float64(t.Unix()))
	}
}

// ObserveSyncFailure increments the sync failure counter.
func (m *Metrics) ObserveSyncFailure() {
	if m == nil {
		return
	}
	m.syncFailures.Inc()
}

// ObserveDecision increments authz_decision_total for the outcome.
func (m *Metrics) ObserveDecision(allowed, degraded bool) {
	if m == nil {
		return
	}
	switch {
	case allowed && degraded:
		m.decisions.WithLabelValues(DecisionDegradedAllow).Inc()
	case allowed:
		m.decisions.WithLabelValues(DecisionAllow).Inc()
	case degraded:
		m.decisions.WithLabelValues(DecisionDegradedDeny).Inc()
	default:
		m.decisions.WithLabelValues(DecisionDeny).Inc()
	}
}

// RefreshFromChecker updates gauges derived from checker state (staleness, tier, loaded).
// Returns previous and current tier for transition logging.
func (m *Metrics) RefreshFromChecker(c *Checker) (prev, cur Tier) {
	if m == nil || c == nil {
		return TierInvalid, TierInvalid
	}
	cur = c.Tier()
	stale := c.Staleness()
	if stale < 0 {
		m.staleSeconds.Set(-1)
	} else {
		m.staleSeconds.Set(stale.Seconds())
	}
	if c.HasPolicies() {
		m.hasPolicies.Set(1)
	} else {
		m.hasPolicies.Set(0)
	}
	if ls := c.LastSuccess(); !ls.IsZero() {
		m.syncLastSuccess.Set(float64(ls.Unix()))
	}
	m.setTierGauges(cur)

	prevStored := m.lastLoggedTier.Swap(int32(cur) + 1)
	if prevStored == 0 {
		return cur, cur // first observation — no transition
	}
	prev = Tier(prevStored - 1)
	return prev, cur
}

func (m *Metrics) setTierGauges(active Tier) {
	for _, t := range []Tier{TierInvalid, TierStale, TierHealthy} {
		v := 0.0
		if t == active {
			v = 1
		}
		m.tier.WithLabelValues(t.String()).Set(v)
	}
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

// DecisionResult formats allow/degraded into a stable label (tests/helpers).
func DecisionResult(allowed, degraded bool) string {
	switch {
	case allowed && degraded:
		return DecisionDegradedAllow
	case allowed:
		return DecisionAllow
	case degraded:
		return DecisionDegradedDeny
	default:
		return DecisionDeny
	}
}
