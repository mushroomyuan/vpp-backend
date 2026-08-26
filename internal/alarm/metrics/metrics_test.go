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
)

func TestMetrics_IngestSingleCounter(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}

	m.ObserveIngest(SourceDispatch, ResultOK, ReasonNone, 12*time.Millisecond)
	m.ObserveIngest(SourceSOE, ResultPoison, ReasonDecode, time.Millisecond)
	m.ObserveIngest(SourceDispatch, ResultPoison, ReasonFingerprintCollision, time.Millisecond)
	m.ObserveIngest(SourceSOE, ResultDropped, ReasonRule, time.Millisecond)
	m.ObserveIngest(SourceDispatch, ResultRetry, ReasonTransient, time.Millisecond)
	m.ObserveIngest(SourceSOE, ResultDedupHit, ReasonNone, time.Millisecond)

	body := scrape(t, reg)
	for _, want := range []string{
		`alarm_ingest_total{reason="none",result="ok",source="dispatch"} 1`,
		`alarm_ingest_total{reason="decode",result="poison",source="soe"} 1`,
		`alarm_ingest_total{reason="fingerprint_collision",result="poison",source="dispatch"} 1`,
		`alarm_ingest_total{reason="rule",result="dropped",source="soe"} 1`,
		`alarm_ingest_total{reason="transient",result="retry",source="dispatch"} 1`,
		`alarm_ingest_total{reason="none",result="dedup_hit",source="soe"} 1`,
		`alarm_ingest_duration_seconds_count{source="dispatch"} 3`,
		`alarm_ingest_duration_seconds_count{source="soe"} 3`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q\n%s", want, body)
		}
	}
	for _, forbid := range []string{
		"alarm_ingest_poison_total",
		"alarm_ingest_dropped_total",
		"alarm_ingest_retry_total",
		"alarm_ingest_dedup_total",
	} {
		if strings.Contains(body, forbid) {
			t.Fatalf("ingest must stay on one counter, found %s", forbid)
		}
	}
}

func TestMetrics_OpenGaugeProcessLocal(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}

	m.SetOpenCount(SourceDispatch, 2)
	m.SetOpenCount(SourceSOE, 1)
	m.AlarmOpened(SourceSOE) // new ticket
	m.AlarmOpened(SourceSOE) // would be wrong on merge; caller must not
	m.AlarmClosed(SourceSOE)

	body := scrape(t, reg)
	if !strings.Contains(body, `alarm_open_alarms{source="dispatch"} 2`) {
		t.Fatalf("dispatch\n%s", body)
	}
	if !strings.Contains(body, `alarm_open_alarms{source="soe"} 2`) {
		t.Fatalf("soe after +2 -1\n%s", body)
	}
}

func TestMetrics_AckCloseConflictAndConsumer(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}

	m.AckConflict()
	m.AckConflict()
	m.CloseConflict()
	m.IncConsumerMessages(SourceDispatch)
	m.IncConsumerHandlerErrors(SourceDispatch)
	m.SetConsumerLag(SourceSOE, 7)

	body := scrape(t, reg)
	for _, want := range []string{
		`alarm_ack_conflict_total 2`,
		`alarm_close_conflict_total 1`,
		`alarm_consumer_messages_total{source="dispatch"} 1`,
		`alarm_consumer_handler_errors_total{source="dispatch"} 1`,
		`alarm_consumer_lag{source="soe"} 7`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q\n%s", want, body)
		}
	}
}

func TestMetrics_NilSafe(t *testing.T) {
	t.Parallel()
	var m *Metrics
	m.ObserveIngest(SourceDispatch, ResultOK, ReasonNone, time.Millisecond)
	m.AlarmOpened(SourceDispatch)
	m.AlarmClosed(SourceDispatch)
	m.SetOpenCount(SourceDispatch, 1)
	m.AckConflict()
	m.CloseConflict()
	m.IncConsumerMessages(SourceSOE)
	m.IncConsumerHandlerErrors(SourceSOE)
	m.SetConsumerLag(SourceSOE, 1)
	if m.Collector() == nil {
		t.Fatal("nil collector")
	}
}

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
