package http

import "github.com/mushroomyuan/vpp-backend/platform/authz"

// AuthzCatalog is the gateway-service permission inventory for human-managed
// mapping APIs (C10b). EMS telemetry:ingest is intentionally excluded.
func AuthzCatalog(owner, model string) authz.Catalog {
	return authz.Catalog{
		Owner:   owner,
		Model:   model,
		Service: "gateway",
		Entries: []authz.CatalogEntry{
			{Object: "gateway:mappings", Actions: []string{"read", "write", "delete"}},
		},
	}
}
