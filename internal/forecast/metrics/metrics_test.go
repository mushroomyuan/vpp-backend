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

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

func TestMetrics_CycleTargetPredictAndWrites(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}

	m.ObserveCycle(120*time.Millisecond, nil)
	m.ObserveCycle(50*time.Millisecond, errForTest{})
	m.ObserveTarget(true)
	m.ObserveTarget(true)
	m.ObserveTarget(false)
	m.ObservePredict(string(model.AlgorithmMovingAverage), 8*time.Millisecond, nil)
	m.ObservePredict(string(model.AlgorithmSamePeriodPrior), 12*time.Millisecond, errForTest{})
	m.ObserveTelemetryQuery(true)
	m.ObserveTelemetryQuery(false)
	m.ObservePostgresWrite(true)
	m.ObservePostgresWrite(false)
	m.ObserveRedisWrite(true)
	m.ObserveRedisWrite(false)

	body := scrape(t, reg)
	for _, want := range []string{
		`forecast_cycle_total{result="ok"} 1`,
		`forecast_cycle_total{result="error"} 1`,
		`forecast_cycle_duration_seconds_count 2`,
		`forecast_targets_total{result="success"} 2`,
		`forecast_targets_total{result="failure"} 1`,
		`forecast_predict_total{algorithm_version="moving_average",result="success"} 1`,
		`forecast_predict_total{algorithm_version="same_period_prior",result="failure"} 1`,
		`forecast_predict_duration_seconds_count{algorithm_version="moving_average"} 1`,
		`forecast_telemetry_query_total{result="success"} 1`,
		`forecast_telemetry_query_total{result="failure"} 1`,
		`forecast_postgres_write_total{result="success"} 1`,
		`forecast_postgres_write_total{result="failure"} 1`,
		`forecast_redis_write_total{result="success"} 1`,
		`forecast_redis_write_total{result="failure"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q\n%s", want, body)
		}
	}
}

func TestMetrics_EmptyAlgorithmVersionIsUnknown(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}
	m.ObservePredict("", time.Millisecond, nil)
	body := scrape(t, reg)
	if !strings.Contains(body, `forecast_predict_total{algorithm_version="unknown",result="success"} 1`) {
		t.Fatalf("empty algorithm_version must become unknown\n%s", body)
	}
}

func TestMetrics_NilSafe(t *testing.T) {
	t.Parallel()
	var m *Metrics
	m.ObserveCycle(time.Millisecond, nil)
	m.ObserveTarget(true)
	m.ObservePredict(string(model.AlgorithmMovingAverage), time.Millisecond, nil)
	m.ObserveTelemetryQuery(true)
	m.ObservePostgresWrite(true)
	m.ObserveRedisWrite(false)
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
