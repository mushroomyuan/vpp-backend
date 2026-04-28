package types

// ── Payload types (stored in import_jobs.payload; tenant comes from Job.TenantID) ──

// ResourceDeletePayload is the JSON-encoded payload for delete jobs with model.JobTargetResource.
type ResourceDeletePayload struct {
	BatchSize int      `json:"batch_size,omitempty"`
	IDs       []string `json:"ids"`
}

// CUDeletePayload is the JSON-encoded payload for delete jobs with model.JobTargetCU.
type CUDeletePayload struct {
	BatchSize int      `json:"batch_size,omitempty"`
	IDs       []string `json:"ids"`
}

// PointDeletePayload is the JSON-encoded payload for delete jobs with model.JobTargetPoint.
type PointDeletePayload struct {
	BatchSize int      `json:"batch_size,omitempty"`
	IDs       []string `json:"ids"`
}

// ── Payload types (stored in import_jobs.payload; tenant comes from Job.TenantID) ──

// ResourceImportPayload is the JSON-encoded payload for import jobs with
// model.JobTargetResource. Use model.Job.TenantID at execution time.
type ResourceImportPayload struct {
	SiteID    string         `json:"site_id"`
	BatchSize int            `json:"batch_size,omitempty"`
	Items     []ResourceItem `json:"items"`
}

// CUImportPayload is the JSON-encoded payload for import jobs with model.JobTargetCU.
type CUImportPayload struct {
	ResourceID string   `json:"resource_id"`
	BatchSize  int      `json:"batch_size,omitempty"`
	Items      []CUItem `json:"items"`
}

// PointImportPayload is the JSON-encoded payload for import jobs with model.JobTargetPoint.
type PointImportPayload struct {
	ResourceID string      `json:"resource_id"`
	CUID       string      `json:"cu_id"`
	BatchSize  int         `json:"batch_size,omitempty"`
	Items      []PointItem `json:"items"`
}
