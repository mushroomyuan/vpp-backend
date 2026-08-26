package http

import (
	"net/http"
	"testing"
)

func TestActionOf(t *testing.T) {
	t.Parallel()
	cases := []struct {
		method, path, want string
	}{
		{http.MethodGet, "/api/v1/tenants/t/alarms", "read"},
		{http.MethodHead, "/api/v1/tenants/t/alarms/id", "read"},
		{http.MethodPost, "/api/v1/tenants/t/alarms/id/ack", "ack"},
		{http.MethodPost, "/api/v1/tenants/t/alarms/id/close", "close"},
		{http.MethodPost, "/api/v1/tenants/t/alarms", ""},
		{http.MethodDelete, "/api/v1/tenants/t/alarms/id", ""},
	}
	for _, tc := range cases {
		if got := actionOf(tc.method, tc.path); got != tc.want {
			t.Fatalf("%s %s: got %q want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestResourceOf(t *testing.T) {
	t.Parallel()
	if resourceOf("/api/v1/tenants/t/alarms") != catalogObject {
		t.Fatal(resourceOf("/api/v1/tenants/t/alarms"))
	}
	if resourceOf("/api/v1/tenants/t/other") != "" {
		t.Fatal("non-alarm path must not map")
	}
}

func TestAuthzCatalog(t *testing.T) {
	t.Parallel()
	cat := AuthzCatalog("default", "default/vpp-rbac")
	if cat.Service != "alarm" || len(cat.Entries) != 1 {
		t.Fatalf("%+v", cat)
	}
	e := cat.Entries[0]
	if e.Object != catalogObject {
		t.Fatal(e.Object)
	}
	want := map[string]bool{"read": true, "ack": true, "close": true}
	if len(e.Actions) != len(want) {
		t.Fatalf("actions=%v", e.Actions)
	}
	for _, a := range e.Actions {
		if !want[a] {
			t.Fatalf("unexpected %q", a)
		}
	}
}
