package authz

import (
	"reflect"
	"testing"
)

func TestPoliciesFromPermissions_C5SeedShape(t *testing.T) {
	perms := []RemotePermission{
		{
			Name:      "vpp-resource-read",
			Roles:     []string{"default/viewer", "default/operator", "default/admin"},
			Resources: []string{"resource:*"},
			Actions:   []string{"read"},
			Model:     "default/vpp-rbac",
			Effect:    "Allow",
			IsEnabled: true,
			State:     "Approved",
		},
		{
			Name:      "vpp-resource-write",
			Roles:     []string{"default/operator", "default/admin"},
			Resources: []string{"resource:*"},
			Actions:   []string{"write"},
			Model:     "default/vpp-rbac",
			Effect:    "Allow",
			IsEnabled: true,
			State:     "Approved",
		},
		{
			Name:      "disabled",
			Roles:     []string{"default/admin"},
			Resources: []string{"resource:*"},
			Actions:   []string{"read"},
			Model:     "default/vpp-rbac",
			IsEnabled: false,
			State:     "Approved",
		},
		{
			Name:      "other-model",
			Roles:     []string{"default/admin"},
			Resources: []string{"resource:*"},
			Actions:   []string{"read"},
			Model:     "default/other",
			IsEnabled: true,
			State:     "Approved",
		},
	}
	got := PoliciesFromPermissions(perms, "default/vpp-rbac")
	want := []PolicyRule{
		{"viewer", "resource:*", "read"},
		{"operator", "resource:*", "read"},
		{"admin", "resource:*", "read"},
		{"operator", "resource:*", "write"},
		{"admin", "resource:*", "write"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBareRole(t *testing.T) {
	if bareRole("default/admin") != "admin" {
		t.Fatal(bareRole("default/admin"))
	}
	if bareRole("admin") != "admin" {
		t.Fatal(bareRole("admin"))
	}
}
