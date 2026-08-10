package grpc

import "github.com/mushroomyuan/vpp-backend/platform/authz"

// AuthzCatalog is the dispatch-service permission inventory (§7.1 / C10a).
func AuthzCatalog(owner, model string) authz.Catalog {
	return authz.Catalog{
		Owner:   owner,
		Model:   model,
		Service: "dispatch",
		Entries: []authz.CatalogEntry{
			{Object: "dispatch:tasks", Actions: []string{"submit", "read", "cancel"}},
		},
	}
}
