package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_RecordsCyclePlanAndScopeSkip(t *testing.T) {
	m := New()
	reg := prometheus.NewRegistry()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}
	m.ObserveCycle(100*time.Millisecond, nil)
	m.ObservePlan("feasible")
	m.ObservePolicy("fired")
	m.ObservePolicy("in_band")
	m.ObserveScopeResolve("skipped")

	if got := testutil.ToFloat64(m.cycleTotal.WithLabelValues(ResultOK)); got != 1 {
		t.Fatalf("cycles = %v", got)
	}
	if got := testutil.ToFloat64(m.plans.WithLabelValues("feasible")); got != 1 {
		t.Fatalf("plans = %v", got)
	}
	if got := testutil.ToFloat64(m.policies.WithLabelValues("in_band")); got != 1 {
		t.Fatalf("in_band = %v", got)
	}
	if got := testutil.ToFloat64(m.scopeResolve.WithLabelValues("skipped")); got != 1 {
		t.Fatalf("skipped = %v", got)
	}
}
