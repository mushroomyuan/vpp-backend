package http

import "time"

// ─── Telemetry ───────────────────────────────────────────────────────────────

// IngestTelemetryRequest is the JSON body for POST /api/v1/tenants/:tenant_id/telemetry:ingest.
// TenantID is taken from the URL path, not the body.
type IngestTelemetryRequest struct {
	ExternalSystem string          `json:"external_system" binding:"required"`
	ExternalID     string          `json:"external_id" binding:"required"`
	// Timestamp is optional. When omitted the server uses time.Now().
	Timestamp *time.Time          `json:"timestamp"`
	Metrics   []MetricValueRequest `json:"metrics" binding:"required,min=1"`
}

type MetricValueRequest struct {
	Name  string  `json:"name" binding:"required"`
	Value float64 `json:"value"`
}

// ─── Mapping CRUD ─────────────────────────────────────────────────────────────

type CreateMappingRequest struct {
	ExternalSystem string `json:"external_system" binding:"required"`
	ExternalID     string `json:"external_id" binding:"required"`
	CUCode         string `json:"cu_code" binding:"required"`
}

// MappingResponse is the JSON representation of a DeviceMapping returned by
// create and list endpoints.
type MappingResponse struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenant_id"`
	ExternalSystem string `json:"external_system"`
	ExternalID     string `json:"external_id"`
	CUCode         string `json:"cu_code"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ListMappingsResponse wraps a slice of MappingResponse for GET /mappings.
type ListMappingsResponse struct {
	Mappings []*MappingResponse `json:"mappings"`
}
