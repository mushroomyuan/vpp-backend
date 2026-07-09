package dispatch

// TaskLifecyclePayload is a lightweight snapshot published when a DispatchTask
// transitions to Started, Completed, or Failed.
type TaskLifecyclePayload struct {
	TaskID   string `json:"task_id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}
