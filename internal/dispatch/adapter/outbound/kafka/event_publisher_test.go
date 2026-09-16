package kafka

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

func TestNewTaskLifecyclePayload_CopiesTriggerType(t *testing.T) {
	t.Parallel()
	task := &model.DispatchTask{
		ID:          "task-1",
		TenantID:    "tenant-1",
		Name:        "opt:internal_rule:cu-1",
		Status:      model.TaskStatusFailed,
		TriggerType: model.TriggerAutomatic,
	}

	p := newTaskLifecyclePayload(task)
	if p.TaskID != "task-1" || p.TenantID != "tenant-1" || p.Name != task.Name {
		t.Errorf("identity fields: %+v", p)
	}
	if p.Status != "failed" {
		t.Errorf("Status = %q, want failed", p.Status)
	}
	if p.TriggerType != "automatic" {
		t.Errorf("TriggerType = %q, want automatic", p.TriggerType)
	}

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"trigger_type":"automatic"`) {
		t.Errorf("wire JSON missing trigger_type: %s", raw)
	}
}

func TestNewTaskLifecyclePayload_EmptyTriggerTypeOnLegacyTask(t *testing.T) {
	t.Parallel()
	p := newTaskLifecyclePayload(&model.DispatchTask{ID: "task-1", Status: model.TaskStatusFailed})
	if p.TriggerType != "" {
		t.Errorf("TriggerType = %q, want empty for a task that never set one", p.TriggerType)
	}
}
