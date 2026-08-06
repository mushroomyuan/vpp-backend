package authz

import (
	"context"
	"fmt"
	"strings"
)

// CatalogEntry is one logical resource and the actions the service may request.
type CatalogEntry struct {
	Object  string   // e.g. resource:sites
	Actions []string // e.g. read, write, delete
}

// Catalog is a service's declared permission inventory (§7.1).
// RegisterCatalog upserts one Casdoor Permission per (Object, Action); role
// bindings stay in Casdoor and are never overwritten.
type Catalog struct {
	Owner   string // Casdoor owner / VPP tenant, e.g. default
	Model   string // e.g. default/vpp-rbac
	Service string // short service name for docs, e.g. resource
	Entries []CatalogEntry
}

// PermissionAdmin is the Casdoor write side used by catalog registration.
type PermissionAdmin interface {
	PermissionSource
	AddPermission(ctx context.Context, p RemotePermission) error
	UpdatePermission(ctx context.Context, p RemotePermission) error
}

// RegisterResult summarizes a catalog upsert.
type RegisterResult struct {
	Added   int
	Updated int
	Skipped int
}

// CatalogPermissionName builds the Casdoor Permission.Name for a catalog pair.
// Example: resource:sites + read → catalog-resource-sites-read
func CatalogPermissionName(object, action string) string {
	obj := strings.ReplaceAll(strings.TrimSpace(object), ":", "-")
	act := strings.TrimSpace(action)
	return "catalog-" + obj + "-" + act
}

// FullPermissionID is the human/audit id from §7.1 (not a Casbin column).
func FullPermissionID(object, action string) string {
	return strings.TrimSpace(object) + ":" + strings.TrimSpace(action)
}

// RegisterCatalog upserts catalog Permissions into Casdoor.
// Existing Roles / Users / Groups / Domains are preserved on update.
func RegisterCatalog(ctx context.Context, admin PermissionAdmin, catalog Catalog) (RegisterResult, error) {
	var out RegisterResult
	if admin == nil {
		return out, fmt.Errorf("authz catalog: admin required")
	}
	owner := strings.TrimSpace(catalog.Owner)
	if owner == "" {
		return out, fmt.Errorf("authz catalog: Owner required")
	}
	model := strings.TrimSpace(catalog.Model)
	if model == "" {
		return out, fmt.Errorf("authz catalog: Model required")
	}

	existing, err := admin.FetchPermissions(ctx, owner)
	if err != nil {
		return out, fmt.Errorf("authz catalog fetch: %w", err)
	}
	byName := make(map[string]RemotePermission, len(existing))
	for _, p := range existing {
		byName[p.Name] = p
	}

	for _, entry := range catalog.Entries {
		obj := strings.TrimSpace(entry.Object)
		if obj == "" {
			continue
		}
		for _, act := range entry.Actions {
			act = strings.TrimSpace(act)
			if act == "" {
				continue
			}
			name := CatalogPermissionName(obj, act)
			fullID := FullPermissionID(obj, act)
			desired := RemotePermission{
				Owner:        owner,
				Name:         name,
				DisplayName:  fullID,
				Description:  catalogDescription(catalog.Service, fullID),
				Users:        []string{},
				Groups:       []string{},
				Roles:        []string{},
				Domains:      []string{},
				Model:        model,
				ResourceType: "Custom",
				Resources:    []string{obj},
				Actions:      []string{act},
				Effect:       "Allow",
				IsEnabled:    true,
				State:        "Approved",
			}

			old, exists := byName[name]
			if !exists {
				if err := admin.AddPermission(ctx, desired); err != nil {
					return out, fmt.Errorf("authz catalog add %s: %w", name, err)
				}
				out.Added++
				continue
			}

			merged := mergeCatalogPermission(old, desired)
			if catalogPermissionEqual(old, merged) {
				out.Skipped++
				continue
			}
			if err := admin.UpdatePermission(ctx, merged); err != nil {
				return out, fmt.Errorf("authz catalog update %s: %w", name, err)
			}
			out.Updated++
		}
	}
	return out, nil
}

func catalogDescription(service, fullID string) string {
	svc := strings.TrimSpace(service)
	if svc == "" {
		svc = "service"
	}
	return fmt.Sprintf("VPP authz catalog (%s): %s — bind roles in Casdoor; do not rely on empty Roles for enforce", svc, fullID)
}

// mergeCatalogPermission copies catalog fields onto an existing permission while
// preserving operator-managed subject bindings.
func mergeCatalogPermission(old, desired RemotePermission) RemotePermission {
	out := old
	out.DisplayName = desired.DisplayName
	out.Description = desired.Description
	out.Model = desired.Model
	out.ResourceType = desired.ResourceType
	out.Resources = append([]string(nil), desired.Resources...)
	out.Actions = append([]string(nil), desired.Actions...)
	out.Effect = desired.Effect
	out.IsEnabled = desired.IsEnabled
	out.State = desired.State
	// Preserve Roles / Users / Groups / Domains from old.
	return out
}

func catalogPermissionEqual(a, b RemotePermission) bool {
	return a.DisplayName == b.DisplayName &&
		a.Description == b.Description &&
		a.Model == b.Model &&
		a.ResourceType == b.ResourceType &&
		a.Effect == b.Effect &&
		a.IsEnabled == b.IsEnabled &&
		a.State == b.State &&
		stringSliceEqual(a.Resources, b.Resources) &&
		stringSliceEqual(a.Actions, b.Actions)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
