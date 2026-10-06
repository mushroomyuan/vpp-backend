package kafka

import (
	"encoding/json"
	"testing"
	"time"

	telEvent "github.com/mushroomyuan/vpp-backend/platform/event/telemetry"
)

func TestSOEPayload_MatchesPlatformContract(t *testing.T) {
	t.Parallel()
	prev := 0.0
	ts := time.Date(2026, 8, 19, 9, 38, 0, 123456789, time.UTC)
	local, err := json.Marshal(soePayload{
		SchemaVersion: telEvent.SchemaVersionV2,
		TenantID:      "tenant-a",
		CUCode:        "cu-1",
		MetricID:      "electrical.active_power.v1",
		Kind:          telEvent.KindDiscreteChange,
		Quality:       telEvent.QualityGood,
		Value:         1,
		PreviousValue: &prev,
		ObservedAt:    ts,
	})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := json.Marshal(telEvent.SOEPayload{
		SchemaVersion: telEvent.SchemaVersionV2,
		TenantID:      "tenant-a",
		CUCode:        "cu-1",
		MetricID:      "electrical.active_power.v1",
		Kind:          telEvent.KindDiscreteChange,
		Quality:       telEvent.QualityGood,
		Value:         1,
		PreviousValue: &prev,
		ObservedAt:    ts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(local) != string(remote) {
		t.Fatalf("producer JSON drifted from platform contract\nlocal:  %s\nremote: %s", local, remote)
	}
}
