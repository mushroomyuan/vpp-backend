package types

// ── Scope specs (command input; tenant on job row, not in JSON payload) ──
// Each Spec embeds the corresponding Payload (the exact JSON stored in the DB),
// plus TenantID which lives on the Job row and must not appear in the serialized payload.

type ResourceImportSpec struct {
	TenantID string
	ResourceImportPayload
}

type CUImportSpec struct {
	TenantID string
	CUImportPayload
}

type PointImportSpec struct {
	TenantID string
	PointImportPayload
}

type ResourceDeleteSpec struct {
	TenantID string
	IDs      []string
}

type CUDeleteSpec struct {
	TenantID string
	IDs      []string
}

type PointDeleteSpec struct {
	TenantID string
	IDs      []string
}
