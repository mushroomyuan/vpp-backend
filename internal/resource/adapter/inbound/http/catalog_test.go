package ports

import (
	"net/http"
	"testing"
)

func TestActionOf(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{http.MethodGet, "/api/tenants/default/sites", "read"},
		{http.MethodHead, "/api/tenants/default/sites", "read"},
		{http.MethodPost, "/api/tenants/default/sites", "write"},
		{http.MethodPut, "/api/tenants/default/cus/1", "write"},
		{http.MethodDelete, "/api/tenants/default/resources/x", "delete"},
		{http.MethodPost, "/api/tenants/default/resources/r:changeLifecycle", "change-lifecycle"},
		{http.MethodOptions, "/api/tenants/default/sites", ""},
	}
	for _, tc := range cases {
		if got := actionOf(tc.method, tc.path); got != tc.want {
			t.Fatalf("%s %s: got %q want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestAuthzCatalog_CoversMappedObjects(t *testing.T) {
	cat := AuthzCatalog("default", "default/vpp-rbac")
	if cat.Owner != "default" || cat.Model != "default/vpp-rbac" || cat.Service != "resource" {
		t.Fatalf("meta=%+v", cat)
	}
	want := map[string]map[string]bool{
		"resource:sites":       {"read": true, "write": true},
		"resource:assets":      {"read": true, "write": true, "delete": true},
		"resource:cus":         {"read": true, "write": true},
		"resource:points":      {"read": true, "write": true, "delete": true},
		"resource:tree":        {"read": true, "write": true, "change-lifecycle": true},
		"resource:import-jobs": {"read": true, "write": true},
	}
	if len(cat.Entries) != len(want) {
		t.Fatalf("entries=%d want %d", len(cat.Entries), len(want))
	}
	for _, e := range cat.Entries {
		acts, ok := want[e.Object]
		if !ok {
			t.Fatalf("unexpected object %q", e.Object)
		}
		if len(e.Actions) != len(acts) {
			t.Fatalf("%s actions=%v", e.Object, e.Actions)
		}
		for _, a := range e.Actions {
			if !acts[a] {
				t.Fatalf("%s unexpected action %q", e.Object, a)
			}
		}
	}
}

func TestResourceOf(t *testing.T) {
	cases := []struct {
		path, want string
	}{
		{"/api/tenants/default/sites", "resource:sites"},
		{"/api/tenants/default/sites/s1", "resource:sites"},
		{"/api/tenants/default/sites/s1/resources", "resource:assets"},
		{"/api/tenants/default/resources/x", "resource:assets"},
		{"/api/tenants/default/resources/x:move", "resource:tree"},
		{"/api/tenants/default/resources/x:rename", "resource:tree"},
		{"/api/tenants/default/resources/x:changeLifecycle", "resource:tree"},
		{"/api/tenants/default/resources:batchMove", "resource:tree"},
		{"/api/tenants/default/resources/x/detail", "resource:tree"},
		{"/api/tenants/default/resources/x/children", "resource:tree"},
		{"/api/tenants/default/cus", "resource:cus"},
		{"/api/tenants/default/resources/p/cus", "resource:cus"},
		{"/api/tenants/default/points", "resource:points"},
		{"/api/tenants/default/cus/c1/points", "resource:points"},
		{"/api/import-jobs/abc", "resource:import-jobs"},
		{"/api/import-jobs:submit", "resource:import-jobs"},
	}
	for _, tc := range cases {
		if got := resourceOf(tc.path); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.path, got, tc.want)
		}
	}
}
