package model

import (
	"strings"
	"testing"
	"time"
)

func TestMetric_Validate(t *testing.T) {
	t.Parallel()

	if err := NewMetric("power", 1.2, Analog).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Metric{Name: "", Value: 1, Type: Analog, Quality: QualityGood}).Validate(); err == nil {
		t.Fatal("empty name")
	}
	if err := (Metric{Name: "x", Type: "NOPE", Quality: QualityGood}).Validate(); err == nil {
		t.Fatal("bad type")
	}
	if err := (Metric{Name: "x", Type: Analog, Quality: "NOPE"}).Validate(); err == nil {
		t.Fatal("bad quality")
	}
	m := NewMetricWithQuality("brk", 1, Discrete, QualityBad)
	if m.IsGood() || !m.IsDiscrete() || m.IsAnalog() {
		t.Fatalf("flags: %+v", m)
	}
}

func TestTelemetryRecord(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	r, err := NewTelemetryRecord("t", "cu", ts, []Metric{NewMetric("p", 1, Analog)})
	if err != nil {
		t.Fatal(err)
	}
	if r.MetricByName("p") == nil || r.MetricByName("missing") != nil {
		t.Fatal("MetricByName")
	}

	mixed := &TelemetryRecord{
		TenantID: "t", CUCode: "cu", Timestamp: ts,
		Metrics: []Metric{
			NewMetric("good", 1, Analog),
			NewMetricWithQuality("bad", 2, Analog, QualityBad),
		},
	}
	if len(mixed.GoodMetrics()) != 1 || mixed.GoodMetrics()[0].Name != "good" {
		t.Fatalf("GoodMetrics = %+v", mixed.GoodMetrics())
	}

	cases := []struct {
		name string
		r    TelemetryRecord
		want string
	}{
		{"tenant", TelemetryRecord{CUCode: "c", Timestamp: ts, Metrics: []Metric{NewMetric("p", 1, Analog)}}, "tenant"},
		{"cu", TelemetryRecord{TenantID: "t", Timestamp: ts, Metrics: []Metric{NewMetric("p", 1, Analog)}}, "cu_code"},
		{"ts", TelemetryRecord{TenantID: "t", CUCode: "c", Metrics: []Metric{NewMetric("p", 1, Analog)}}, "timestamp"},
		{"metrics", TelemetryRecord{TenantID: "t", CUCode: "c", Timestamp: ts}, "metric"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}
}

func TestSOEEvent_Validate(t *testing.T) {
	t.Parallel()
	e := NewSOEEvent("t", "cu", "brk", 0, 1, time.Now())
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := *e
	bad.MetricName = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("want metric_name error")
	}
}

func TestQueryCondition_Validate(t *testing.T) {
	t.Parallel()
	start := time.Unix(1, 0)
	end := time.Unix(2, 0)
	ok := NewQueryCondition("t", "cu", "p", start, end)
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	// Domain does NOT enforce 30-day window.
	wide := NewQueryCondition("t", "cu", "p", start, start.Add(40*24*time.Hour))
	if err := wide.Validate(); err != nil {
		t.Fatalf("wide range should pass domain: %v", err)
	}
	if err := NewQueryCondition("", "cu", "p", start, end).Validate(); err == nil {
		t.Fatal("empty tenant")
	}
	if err := NewQueryCondition("t", "cu", "p", end, start).Validate(); err == nil {
		t.Fatal("start after end")
	}
}

func TestAggregationQuery_Validate(t *testing.T) {
	t.Parallel()
	start, end := time.Unix(1, 0), time.Unix(100, 0)
	ok := AggregationQuery{
		TenantID: "t", CUCode: "cu", MetricName: "p",
		StartTime: start, EndTime: end, Step: time.Minute, Functions: []AggFunction{AggAvg},
	}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := ok
	bad.Step = 0
	if err := bad.Validate(); err == nil {
		t.Fatal("step")
	}
	bad = ok
	bad.Functions = nil
	if err := bad.Validate(); err == nil {
		t.Fatal("functions")
	}
}

func TestSnapshot_Apply(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	s := NewSnapshot("t", "cu")

	// Analog good: update, no SOE
	rec1 := mustRecord(t, "t", "cu", ts, []Metric{NewMetric("power", 10, Analog)})
	if ev := s.Apply(rec1); len(ev) != 0 {
		t.Fatalf("analog SOE = %d", len(ev))
	}
	if v, ok := s.Get("power"); !ok || v != 10 {
		t.Fatalf("power = %v %v", v, ok)
	}

	// Discrete first write: set value, no SOE
	rec2 := mustRecord(t, "t", "cu", ts.Add(time.Second), []Metric{NewMetric("brk", 0, Discrete)})
	if ev := s.Apply(rec2); len(ev) != 0 {
		t.Fatalf("first discrete SOE = %d", len(ev))
	}
	if v, _ := s.Get("brk"); v != 0 {
		t.Fatal(v)
	}

	// Discrete change → SOE
	rec3 := mustRecord(t, "t", "cu", ts.Add(2*time.Second), []Metric{NewMetric("brk", 1, Discrete)})
	ev := s.Apply(rec3)
	if len(ev) != 1 || ev[0].OldValue != 0 || ev[0].NewValue != 1 || ev[0].MetricName != "brk" {
		t.Fatalf("SOE = %+v", ev)
	}

	// Same discrete value → no SOE
	rec4 := mustRecord(t, "t", "cu", ts.Add(3*time.Second), []Metric{NewMetric("brk", 1, Discrete)})
	if ev := s.Apply(rec4); len(ev) != 0 {
		t.Fatal("same value should not SOE")
	}

	// Bad quality skipped; previous kept
	rec5 := mustRecord(t, "t", "cu", ts.Add(4*time.Second), []Metric{
		NewMetricWithQuality("power", 99, Analog, QualityBad),
	})
	if ev := s.Apply(rec5); len(ev) != 0 {
		t.Fatal(ev)
	}
	if v, _ := s.Get("power"); v != 10 {
		t.Fatalf("power should stay 10, got %v", v)
	}
	if !s.UpdatedAt.Equal(ts.Add(4 * time.Second)) {
		t.Fatalf("UpdatedAt = %v", s.UpdatedAt)
	}

	// Mixed: analog + discrete change → one SOE
	rec6 := mustRecord(t, "t", "cu", ts.Add(5*time.Second), []Metric{
		NewMetric("power", 11, Analog),
		NewMetric("brk", 0, Discrete),
	})
	ev = s.Apply(rec6)
	if len(ev) != 1 || ev[0].MetricName != "brk" {
		t.Fatalf("mixed SOE = %+v", ev)
	}
}

func TestSnapshot_IsStale(t *testing.T) {
	t.Parallel()
	s := NewSnapshot("t", "cu")
	s.UpdatedAt = time.Now().Add(-10 * time.Minute)
	if !s.IsStale(5 * time.Minute) {
		t.Fatal("want stale")
	}
	s.UpdatedAt = time.Now()
	if s.IsStale(5 * time.Minute) {
		t.Fatal("want fresh")
	}
}

func mustRecord(t *testing.T, tenant, cu string, ts time.Time, metrics []Metric) *TelemetryRecord {
	t.Helper()
	r, err := NewTelemetryRecord(tenant, cu, ts, metrics)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
