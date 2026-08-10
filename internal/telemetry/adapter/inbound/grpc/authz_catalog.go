package grpc

import "github.com/mushroomyuan/vpp-backend/platform/authz"

// AuthzCatalog is the telemetry-service permission inventory for human-facing
// read APIs (C10c). IngestTelemetry is intentionally excluded.
func AuthzCatalog(owner, model string) authz.Catalog {
	return authz.Catalog{
		Owner:   owner,
		Model:   model,
		Service: "telemetry",
		Entries: []authz.CatalogEntry{
			{Object: "telemetry:telemetry", Actions: []string{"read"}},
			{Object: "telemetry:snapshots", Actions: []string{"read"}},
			{Object: "telemetry:aggregation", Actions: []string{"read"}},
		},
	}
}
