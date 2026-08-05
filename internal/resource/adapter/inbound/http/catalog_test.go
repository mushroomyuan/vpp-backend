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
