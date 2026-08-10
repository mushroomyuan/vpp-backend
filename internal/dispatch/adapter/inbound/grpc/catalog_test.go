package grpc

import "testing"

func TestCatalogOf(t *testing.T) {
	cases := []struct {
		method, obj, act string
		ok               bool
	}{
		{"/dispatchpb.DispatchService/SubmitTask", "dispatch:tasks", "submit", true},
		{"/dispatchpb.DispatchService/GetTask", "dispatch:tasks", "read", true},
		{"/dispatchpb.DispatchService/CancelTask", "dispatch:tasks", "cancel", true},
		{"/dispatchpb.DispatchService/Unknown", "", "", false},
	}
	for _, tc := range cases {
		obj, act, ok := CatalogOf(tc.method)
		if obj != tc.obj || act != tc.act || ok != tc.ok {
			t.Fatalf("%s: got (%q,%q,%v) want (%q,%q,%v)",
				tc.method, obj, act, ok, tc.obj, tc.act, tc.ok)
		}
	}
}

func TestAuthzCatalog(t *testing.T) {
	cat := AuthzCatalog("default", "default/vpp-rbac")
	if cat.Service != "dispatch" || len(cat.Entries) != 1 {
		t.Fatalf("%+v", cat)
	}
	e := cat.Entries[0]
	if e.Object != "dispatch:tasks" || len(e.Actions) != 3 {
		t.Fatalf("%+v", e)
	}
}
