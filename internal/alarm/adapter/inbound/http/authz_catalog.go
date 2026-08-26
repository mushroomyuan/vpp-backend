package http

import "github.com/mushroomyuan/vpp-backend/platform/authz"

// AuthzCatalog is the alarm-service permission inventory.
// Role bindings stay in Casdoor. v1 placeholder intent:
//
//	viewer   = read
//	operator = read + ack + close
//	admin    = all of the above
func AuthzCatalog(owner, model string) authz.Catalog {
	return authz.Catalog{
		Owner:   owner,
		Model:   model,
		Service: "alarm",
		Entries: []authz.CatalogEntry{
			{Object: catalogObject, Actions: []string{"read", "ack", "close"}},
		},
	}
}
