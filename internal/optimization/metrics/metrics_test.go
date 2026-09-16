package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
)

func TestMetrics_CycleRulesSubmitForecast(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}

	m.ObserveCycle(120*time.Millisecond, nil)
	m.ObserveCycle(50*time.Millisecond, errForTest{})
	m.ObserveRulesFired(string(model.RuleSOCThreshold), 2)
	m.ObserveSubmit(true)
	m.ObserveSubmit(true)
	m.ObserveSubmit(false)
	m.ObserveForecast(ResultNotImplemented)
	m.ObserveForecast(ResultNotImplemented)
	m.ObserveForecast(ResultError)

	body := scrape(t, reg)
	for _, want := range []string{
		`optimization_decision_cycle_total{result="ok"} 1`,
		`optimization_decision_cycle_total{result="error"} 1`,
		`optimization_decision_cycle_duration_seconds_count 2`,
		`optimization_rules_fired_total{rule_id="soc_threshold"} 2`,
		`optimization_submit_task_total{result="success"} 2`,
		`optimization_submit_task_total{result="failure"} 1`,
		`optimization_forecast_calls_total{result="not_implemented"} 2`,
		`optimization_forecast_calls_total{result="error"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q\n%s", want, body)
		}
	}
}

func TestMetrics_IgnoresUnknownForecastResult(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}
	m.ObserveForecast("bogus")
	body := scrape(t, reg)
	if strings.Contains(body, `result="bogus"`) {
		t.Fatalf("unknown forecast result must not create a series\n%s", body)
	}
}

func TestMetrics_NilSafe(t *testing.T) {
	t.Parallel()
	var m *Metrics
	m.ObserveCycle(time.Millisecond, nil)
	m.ObserveRulesFired("soc_threshold", 1)
	m.ObserveSubmit(true)
	m.ObserveForecast(ResultNotImplemented)
	if m.Collector() == nil {
		t.Fatal("nil collector")
	}
}

type errForTest struct{}

func (errForTest) Error() string { return "boom" }

func scrape(t *testing.T, reg *prometheus.Registry) string {
	t.Helper()
	srv := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
