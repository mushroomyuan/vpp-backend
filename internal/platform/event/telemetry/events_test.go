package telemetry

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSOEPayload_JSONContract(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 8, 19, 9, 38, 0, 123456789, time.UTC)
	got, err := json.Marshal(SOEPayload{
		TenantID:   "tenant-a",
		CUCode:     "cu-1",
		MetricName: "switch_pos",
		OldValue:   0,
		NewValue:   1,
		OccurredAt: ts,
	})
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"tenant_id":"tenant-a","cu_code":"cu-1","metric_name":"switch_pos","old_value":0,"new_value":1,"occurred_at":"2026-08-19T09:38:00.123456789Z"}`
	if string(got) != want {
		t.Fatalf("wire JSON mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSOEPayload_UnmarshalProducerJSON(t *testing.T) {
	t.Parallel()

	// Sample matching telemetry/adapter/outbound/kafka soePayload tags.
	raw := []byte(`{
		"tenant_id": "t1",
		"cu_code": "AB",
		"metric_name": "C",
		"old_value": 0.0,
		"new_value": 1.5,
		"occurred_at": "2026-08-19T01:02:03.000000004Z"
	}`)

	var p SOEPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.TenantID != "t1" || p.CUCode != "AB" || p.MetricName != "C" {
		t.Fatalf("ids = %+v", p)
	}
	if p.OldValue != 0 || p.NewValue != 1.5 {
		t.Fatalf("values old=%v new=%v", p.OldValue, p.NewValue)
	}
	want := time.Date(2026, 8, 19, 1, 2, 3, 4, time.UTC)
	if !p.OccurredAt.Equal(want) {
		t.Fatalf("occurred_at = %s, want %s", p.OccurredAt, want)
	}
}

func TestTopicAndVersion(t *testing.T) {
	t.Parallel()
	if TopicSOEEvents != "vpp.soe.events" {
		t.Fatalf("topic = %q", TopicSOEEvents)
	}
	if VersionV1 != "v1" {
		t.Fatalf("version = %q", VersionV1)
	}
}
