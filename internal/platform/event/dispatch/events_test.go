package dispatch

import (
	"encoding/json"
	"testing"
)

func TestTaskLifecyclePayload_LegacyJSONStillDecodes(t *testing.T) {
	t.Parallel()
	// Events published before TriggerType existed must keep decoding.
	const legacy = `{"task_id":"task-1","tenant_id":"t1","name":"shed","status":"failed"}`
	var p TaskLifecyclePayload
	if err := json.Unmarshal([]byte(legacy), &p); err != nil {
		t.Fatal(err)
	}
	if p.TaskID != "task-1" || p.Name != "shed" || p.Status != "failed" {
		t.Errorf("legacy fields: %+v", p)
	}
	if p.TriggerType != "" {
		t.Errorf("TriggerType = %q, want empty on pre-field events", p.TriggerType)
	}
}

func TestTaskLifecyclePayload_RoundTripTriggerType(t *testing.T) {
	t.Parallel()
	in := TaskLifecyclePayload{
		TaskID: "task-1", TenantID: "t1", Name: "opt:cu-1",
		Status: "failed", TriggerType: "automatic",
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out TaskLifecyclePayload
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("round-trip %+v, want %+v", out, in)
	}
}
