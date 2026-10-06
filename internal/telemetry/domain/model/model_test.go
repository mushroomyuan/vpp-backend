package model

import (
	"strings"
	"testing"
	"time"
)

const (
	metricPower = "electrical.active_power.v1"
	metricSOC   = "energy_storage.state_of_charge.v1"
	metricQ     = "electrical.reactive_power.v1"
)

func TestMetric_Validate(t *testing.T) {
	t.Parallel()

	if err := NewMetric(metricPower, 1.2, Analog).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Metric{MetricID: "", Value: 1, Type: Analog, Quality: QualityGood}).Validate(); err == nil {
		t.Fatal("empty metric id")
	}
	if err := (Metric{MetricID: "soc", Value: 1, Type: Analog, Quality: QualityGood}).Validate(); err == nil {
		t.Fatal("non-canonical metric id")
	}
	if err := (Metric{MetricID: metricPower, Type: "NOPE", Quality: QualityGood}).Validate(); err == nil {
		t.Fatal("bad type")
	}
	if err := (Metric{MetricID: metricPower, Type: Analog, Quality: "NOPE"}).Validate(); err == nil {
		t.Fatal("bad quality")
	}
	m := NewMetricWithQuality(metricPower, 1, Discrete, QualityBad)
	if m.IsGood() || !m.IsDiscrete() || m.IsAnalog() {
		t.Fatalf("flags: %+v", m)
	}
}

func TestTelemetryRecord(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	r, err := NewTelemetryRecord("t", "cu", ts, []Metric{NewMetric(metricPower, 1, Analog)})
	if err != nil {
		t.Fatal(err)
	}
	if r.MetricByID(metricPower) == nil || r.MetricByID(metricSOC) != nil {
		t.Fatal("MetricByID")
	}

	mixed := &TelemetryRecord{
		TenantID: "t", CUCode: "cu", Timestamp: ts,
		Metrics: []Metric{
			NewMetric(metricPower, 1, Analog),
			NewMetricWithQuality(metricSOC, 2, Analog, QualityBad),
		},
	}
	if len(mixed.GoodMetrics()) != 1 || mixed.GoodMetrics()[0].MetricID != metricPower {
		t.Fatalf("GoodMetrics = %+v", mixed.GoodMetrics())
	}

	cases := []struct {
		name string
		r    TelemetryRecord
		want string
	}{
		{"tenant", TelemetryRecord{CUCode: "c", Timestamp: ts, Metrics: []Metric{NewMetric(metricPower, 1, Analog)}}, "tenant"},
		{"cu", TelemetryRecord{TenantID: "t", Timestamp: ts, Metrics: []Metric{NewMetric(metricPower, 1, Analog)}}, "cu_code"},
		{"ts", TelemetryRecord{TenantID: "t", CUCode: "c", Metrics: []Metric{NewMetric(metricPower, 1, Analog)}}, "timestamp"},
		{"metrics", TelemetryRecord{TenantID: "t", CUCode: "c", Timestamp: ts}, "metric"},
		{"metric id", TelemetryRecord{TenantID: "t", CUCode: "c", Timestamp: ts, Metrics: []Metric{NewMetric("soc", 1, Analog)}}, "invalid metric id"},
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
	prev := 0.0
	e := NewSOEEvent("t", "cu", metricPower, SOEKindDiscreteChange, QualityGood, 1, &prev, time.Now())
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := *e
	bad.MetricID = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("want metric_id error")
	}
	vendor := *e
	vendor.MetricID = "switch_pos"
	if err := vendor.Validate(); err == nil {
		t.Fatal("vendor point name must not validate")
	}
}

func TestQueryCondition_Validate(t *testing.T) {
	t.Parallel()
	start := time.Unix(1, 0)
	end := time.Unix(2, 0)
	ok := NewQueryCondition("t", "cu", metricPower, start, end)
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	// Domain does NOT enforce 30-day window.
	wide := NewQueryCondition("t", "cu", metricPower, start, start.Add(40*24*time.Hour))
	if err := wide.Validate(); err != nil {
		t.Fatalf("wide range should pass domain: %v", err)
	}
	if err := NewQueryCondition("", "cu", metricPower, start, end).Validate(); err == nil {
		t.Fatal("empty tenant")
	}
	if err := NewQueryCondition("t", "cu", metricPower, end, start).Validate(); err == nil {
		t.Fatal("start after end")
	}
	if err := NewQueryCondition("t", "cu", "soc", start, end).Validate(); err == nil {
		t.Fatal("non-canonical metric id")
	}
	if err := NewQueryCondition("t", "cu", "", start, end).Validate(); err != nil {
		t.Fatal("empty metric filter is allowed")
	}
}

func TestAggregationQuery_Validate(t *testing.T) {
	t.Parallel()
	start, end := time.Unix(1, 0), time.Unix(100, 0)
	ok := AggregationQuery{
		TenantID: "t", CUCode: "cu", MetricID: metricPower,
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
	rec1 := mustRecord(t, "t", "cu", ts, []Metric{NewMetric(metricPower, 10, Analog)})
	if ev := s.Apply(rec1, DefaultMetricStaleAge); len(ev) != 0 {
		t.Fatalf("analog SOE = %d", len(ev))
	}
	if state, ok := s.Get(metricPower); !ok || state.Value != 10 || state.Quality != QualityGood {
		t.Fatalf("power = %+v %v", state, ok)
	}

	// Discrete first write: set value, no SOE
	rec2 := mustRecord(t, "t", "cu", ts.Add(time.Second), []Metric{NewMetric(metricQ, 0, Discrete)})
	if ev := s.Apply(rec2, DefaultMetricStaleAge); len(ev) != 0 {
		t.Fatalf("first discrete SOE = %d", len(ev))
	}
	if state, _ := s.Get(metricQ); state.Value != 0 {
		t.Fatal(state)
	}

	// Discrete change → SOE
	rec3 := mustRecord(t, "t", "cu", ts.Add(2*time.Second), []Metric{NewMetric(metricQ, 1, Discrete)})
	ev := s.Apply(rec3, DefaultMetricStaleAge)
	if len(ev) != 1 || ev[0].Kind != SOEKindDiscreteChange || ev[0].PreviousValue == nil || *ev[0].PreviousValue != 0 || ev[0].Value != 1 || ev[0].MetricID != metricQ {
		t.Fatalf("SOE = %+v", ev)
	}

	// Same discrete value → no SOE
	rec4 := mustRecord(t, "t", "cu", ts.Add(3*time.Second), []Metric{NewMetric(metricQ, 1, Discrete)})
	if ev := s.Apply(rec4, DefaultMetricStaleAge); len(ev) != 0 {
		t.Fatal("same value should not SOE")
	}

	// Bad quality replaces the previous good value and is visible as BAD.
	badAt := ts.Add(4 * time.Second)
	rec5 := mustRecord(t, "t", "cu", badAt, []Metric{
		NewMetricWithQuality(metricPower, 99, Analog, QualityBad),
	})
	ev = s.Apply(rec5, DefaultMetricStaleAge)
	if len(ev) != 1 || ev[0].Kind != SOEKindQualityBad || ev[0].Quality != QualityBad || ev[0].Value != 99 {
		t.Fatalf("bad quality event = %+v", ev)
	}
	state, ok := s.Get(metricPower)
	if !ok || state.Value != 99 || state.Quality != QualityBad || !state.ObservedAt.Equal(badAt) {
		t.Fatalf("bad sample should replace good state, got %+v ok=%v", state, ok)
	}
	if !s.UpdatedAt.Equal(badAt) {
		t.Fatalf("UpdatedAt = %v", s.UpdatedAt)
	}

	// Mixed: power recovers to GOOD, discrete value changes.
	rec6 := mustRecord(t, "t", "cu", ts.Add(5*time.Second), []Metric{
		NewMetric(metricPower, 11, Analog),
		NewMetric(metricQ, 0, Discrete),
	})
	ev = s.Apply(rec6, DefaultMetricStaleAge)
	if len(ev) != 2 || ev[0].Kind != SOEKindRecovery || ev[0].MetricID != metricPower || ev[1].Kind != SOEKindDiscreteChange || ev[1].MetricID != metricQ {
		t.Fatalf("mixed SOE = %+v", ev)
	}
	if state, _ := s.Get(metricPower); state.Value != 11 || state.Quality != QualityGood {
		t.Fatalf("power after recovery = %+v", state)
	}
}

func TestSnapshot_StaleGapIsNotAHealthyChange(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	s := NewSnapshot("t", "cu")
	first := mustRecord(t, "t", "cu", ts, []Metric{NewMetric(metricQ, 0, Discrete)})
	if ev := s.Apply(first, DefaultMetricStaleAge); len(ev) != 0 {
		t.Fatal(ev)
	}
	later := mustRecord(t, "t", "cu", ts.Add(2*time.Minute), []Metric{NewMetric(metricQ, 1, Discrete)})
	ev := s.Apply(later, DefaultMetricStaleAge)
	if len(ev) != 2 || ev[0].Kind != SOEKindStale || ev[1].Kind != SOEKindRecovery {
		t.Fatalf("events = %+v", kinds(ev))
	}
	if ev[0].Value != 0 || ev[0].Quality != QualityGood || !ev[0].ObservedAt.Equal(ts) {
		t.Fatalf("stale event must describe the old observation, got %+v", ev[0])
	}
	if ev[1].Value != 1 || ev[1].Quality != QualityGood {
		t.Fatalf("recovery = %+v", ev[1])
	}
	for _, e := range ev {
		if e.Kind == SOEKindDiscreteChange {
			t.Fatal("a value change across a stale gap is not a healthy discrete change")
		}
	}
	state, ok := s.Get(metricQ)
	if !ok || state.Value != 1 || state.Quality != QualityGood {
		t.Fatalf("current state = %+v", state)
	}
}

func TestSnapshot_StaleThenBadDoesNotRecover(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	s := NewSnapshot("t", "cu")
	_ = s.Apply(mustRecord(t, "t", "cu", ts, []Metric{NewMetric(metricPower, 10, Analog)}), DefaultMetricStaleAge)
	ev := s.Apply(mustRecord(t, "t", "cu", ts.Add(2*time.Minute), []Metric{
		NewMetricWithQuality(metricPower, 1, Analog, QualityBad),
	}), DefaultMetricStaleAge)
	if len(ev) != 2 || ev[0].Kind != SOEKindStale || ev[1].Kind != SOEKindQualityBad {
		t.Fatalf("events = %+v", kinds(ev))
	}
	state, _ := s.Get(metricPower)
	if state.Quality != QualityBad || state.Value != 1 {
		t.Fatalf("bad sample must replace the old good value, got %+v", state)
	}
}

func TestSnapshot_UncertainAndRepeatBad(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	s := NewSnapshot("t", "cu")
	_ = s.Apply(mustRecord(t, "t", "cu", ts, []Metric{NewMetric(metricPower, 10, Analog)}), 0)
	ev := s.Apply(mustRecord(t, "t", "cu", ts.Add(time.Second), []Metric{
		NewMetricWithQuality(metricPower, 11, Analog, QualityUncertain),
	}), 0)
	if len(ev) != 1 || ev[0].Kind != SOEKindQualityUncertain {
		t.Fatalf("uncertain = %+v", ev)
	}
	ev = s.Apply(mustRecord(t, "t", "cu", ts.Add(2*time.Second), []Metric{
		NewMetricWithQuality(metricPower, 12, Analog, QualityBad),
	}), 0)
	if len(ev) != 1 || ev[0].Kind != SOEKindQualityBad {
		t.Fatalf("bad = %+v", ev)
	}
	ev = s.Apply(mustRecord(t, "t", "cu", ts.Add(3*time.Second), []Metric{
		NewMetricWithQuality(metricPower, 13, Analog, QualityBad),
	}), 0)
	if len(ev) != 0 {
		t.Fatalf("repeat bad must not emit, got %+v", ev)
	}
	state, _ := s.Get(metricPower)
	if state.Value != 13 || state.Quality != QualityBad {
		t.Fatalf("state = %+v", state)
	}
}

func TestSnapshot_ZeroStaleAgeKeepsDiscreteChange(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	s := NewSnapshot("t", "cu")
	_ = s.Apply(mustRecord(t, "t", "cu", ts, []Metric{NewMetric(metricQ, 0, Discrete)}), 0)
	ev := s.Apply(mustRecord(t, "t", "cu", ts.Add(time.Hour), []Metric{NewMetric(metricQ, 1, Discrete)}), 0)
	if len(ev) != 1 || ev[0].Kind != SOEKindDiscreteChange {
		t.Fatalf("events = %+v", kinds(ev))
	}
}

func kinds(events []*SOEEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Kind
	}
	return out
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
