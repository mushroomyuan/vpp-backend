package resource

// SiteCreatedPayload carries identifying fields when a new site is created.
type SiteCreatedPayload struct {
	SiteID      string `json:"site_id"`
	TenantID    string `json:"tenant_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SiteUpdatedPayload carries identifying fields when a site is updated.
type SiteUpdatedPayload struct {
	SiteID   string `json:"site_id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
}

// AssetCreatedPayload carries identifying fields when a new asset is created.
type AssetCreatedPayload struct {
	AssetID  string `json:"asset_id"`
	TenantID string `json:"tenant_id"`
	SiteID   string `json:"site_id,omitempty"`
	Name     string `json:"name"`
}

// AssetUpdatedPayload carries identifying fields when an asset is updated.
type AssetUpdatedPayload struct {
	AssetID  string `json:"asset_id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
}

// CUCreatedPayload carries identifying and connection fields when a new
// Control Unit is created. Consumed primarily by the gateway service to
// set up ID mappings.
type CUCreatedPayload struct {
	CUID       string  `json:"cu_id"`
	TenantID   string  `json:"tenant_id"`
	Name       string  `json:"name"`
	ParentID   *string `json:"parent_id,omitempty"`
	Provider   *string `json:"provider,omitempty"`
	ExternalID *string `json:"external_id,omitempty"`
	Protocol   *string `json:"protocol,omitempty"`
}

// CUUpdatedPayload carries identifying fields when a Control Unit is updated.
type CUUpdatedPayload struct {
	CUID       string  `json:"cu_id"`
	TenantID   string  `json:"tenant_id"`
	Name       string  `json:"name"`
	Provider   *string `json:"provider,omitempty"`
	ExternalID *string `json:"external_id,omitempty"`
	Protocol   *string `json:"protocol,omitempty"`
}

// ResourceDeletedPayload is published when a node (site / asset / CU) is
// soft-deleted via the generic DeleteResource command.
type ResourceDeletedPayload struct {
	ResourceID         string `json:"resource_id"`
	TenantID           string `json:"tenant_id"`
	IncludeDescendants bool   `json:"include_descendants"`
}

// PointCreatedPayload carries identifying fields when a measurement point is
// created.
type PointCreatedPayload struct {
	PointID  string `json:"point_id"`
	TenantID string `json:"tenant_id"`
	AssetID  string `json:"asset_id,omitempty"`
	CUID     string `json:"cu_id,omitempty"`
	PointKey string `json:"point_key"`
}

// PointUpdatedPayload carries identifying fields when a measurement point is
// updated.
type PointUpdatedPayload struct {
	PointID  string `json:"point_id"`
	TenantID string `json:"tenant_id"`
	PointKey string `json:"point_key"`
}

// PointDeletedPayload is published when a measurement point is soft-deleted.
type PointDeletedPayload struct {
	PointID  string `json:"point_id"`
	TenantID string `json:"tenant_id"`
}

// ResourceRenamedPayload is published when any node is renamed without
// changing other attributes.
type ResourceRenamedPayload struct {
	ResourceID string `json:"resource_id"`
	TenantID   string `json:"tenant_id"`
	NewName    string `json:"new_name"`
}

// LifecycleChangedPayload is published when a node's lifecycle status changes
// (e.g. active → disabled).
type LifecycleChangedPayload struct {
	ResourceID string `json:"resource_id"`
	TenantID   string `json:"tenant_id"`
	Status     string `json:"status"`
}

// ImportCompletedPayload is published once per batch import job when it
// finishes (successfully or partially).
type ImportCompletedPayload struct {
	JobID      string `json:"job_id"`
	TenantID   string `json:"tenant_id"`
	Operation  string `json:"operation"`   // "import" | "delete"
	TargetType string `json:"target_type"` // "asset" | "cu" | "point"
	Total      int    `json:"total"`
	Succeeded  int    `json:"succeeded"`
	Failed     int    `json:"failed"`
}
