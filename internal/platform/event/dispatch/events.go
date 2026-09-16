package dispatch

// TaskLifecyclePayload is a lightweight snapshot published when a DispatchTask
// transitions to Started, Completed, Failed, or Cancelled.
type TaskLifecyclePayload struct {
	TaskID   string `json:"task_id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Status   string `json:"status"`

	// TriggerType is how the task was initiated ("manual" / "scheduled" /
	// "automatic"). Added so consumers (alarm, later) can distinguish a
	// human dispatch failure from Optimization's automatic one. Not part
	// of alarm fingerprint. Empty on events published before this field
	// existed (JSON decode default). Additive on VersionV1 — no bump.
	TriggerType string `json:"trigger_type"`
}
