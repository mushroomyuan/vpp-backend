package telemetry

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSOEPayload_JSONContract(t *testing.T) {
	t.Parallel()

	prev := 0.0
	ts := time.Date(2026, 8, 19, 9, 38, 0, 123456789, time.UTC)
	got, err := json.Marshal(SOEPayload{
		SchemaVersion: SchemaVersionV2,
		TenantID:      "tenant-a",
		CUCode:        "cu-1",
		MetricID:      "electrical.active_power.v1",
		Kind:          KindDiscreteChange,
		Quality:       QualityGood,
		Value:         1,
		PreviousValue: &prev,
		ObservedAt:    ts,
	})
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"schema_version":"v2","tenant_id":"tenant-a","cu_code":"cu-1","metric_id":"electrical.active_power.v1","kind":"discrete_change","quality":"GOOD","value":1,"previous_value":0,"observed_at":"2026-08-19T09:38:00.123456789Z"}`
	if string(got) != want {
		t.Fatalf("wire JSON mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSOEPayload_UnmarshalV2(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"schema_version": "v2",
		"tenant_id": "t1",
		"cu_code": "AB",
		"metric_id": "electrical.active_power.v1",
		"kind": "quality_bad",
		"quality": "BAD",
		"value": 1.5,
		"observed_at": "2026-08-19T01:02:03.000000004Z"
	}`)

	var p SOEPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.SchemaVersion != SchemaVersionV2 || p.TenantID != "t1" || p.CUCode != "AB" || p.MetricID != "electrical.active_power.v1" {
		t.Fatalf("ids = %+v", p)
	}
	if p.Kind != KindQualityBad || p.Quality != QualityBad || p.Value != 1.5 || p.PreviousValue != nil {
		t.Fatalf("body = %+v", p)
	}
	want := time.Date(2026, 8, 19, 1, 2, 3, 4, time.UTC)
	if !p.ObservedAt.Equal(want) {
		t.Fatalf("observed_at = %s, want %s", p.ObservedAt, want)
	}
}

func TestSOEPayload_RetiredV1DoesNotFillMetricID(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"tenant_id":"t1","cu_code":"cu","metric_name":"switch_pos","old_value":0,"new_value":1,"occurred_at":"2026-08-19T01:02:03Z"}`)
	var p SOEPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.SchemaVersion != "" || p.MetricID != "" || p.Kind != "" {
		t.Fatalf("v1 payload must not alias into v2 fields: %+v", p)
	}
}

func TestTopicAndVersion(t *testing.T) {
	t.Parallel()
	if TopicSOEEvents != "vpp.soe.events" {
		t.Fatalf("topic = %q", TopicSOEEvents)
	}
	if SchemaVersionV2 != "v2" {
		t.Fatalf("version = %q", SchemaVersionV2)
	}
}
