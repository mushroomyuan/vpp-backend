package ports

import "github.com/mushroomyuan/vpp-backend/platform/authz"

// AuthzCatalog is the resource-service permission inventory (§7.1).
// Actions match what actionOf / resourceOf can emit for each obj.
func AuthzCatalog(owner, model string) authz.Catalog {
	return authz.Catalog{
		Owner:   owner,
		Model:   model,
		Service: "resource",
		Entries: []authz.CatalogEntry{
			{Object: "resource:sites", Actions: []string{"read", "write"}},
			{Object: "resource:assets", Actions: []string{"read", "write", "delete"}},
			{Object: "resource:cus", Actions: []string{"read", "write"}},
			{Object: "resource:points", Actions: []string{"read", "write", "delete"}},
			{Object: "resource:tree", Actions: []string{"read", "write", "change-lifecycle"}},
			{Object: "resource:import-jobs", Actions: []string{"read", "write"}},
		},
	}
}
