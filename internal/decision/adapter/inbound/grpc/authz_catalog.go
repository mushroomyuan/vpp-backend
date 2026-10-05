package grpc

import "github.com/mushroomyuan/vpp-backend/platform/authz"

// AuthzCatalog is the decision-service permission inventory.
func AuthzCatalog(owner, model string) authz.Catalog {
	return authz.Catalog{
		Owner:   owner,
		Model:   model,
		Service: "decision",
		Entries: []authz.CatalogEntry{
			{Object: "decision:policies", Actions: []string{"read", "write"}},
		},
	}
}
