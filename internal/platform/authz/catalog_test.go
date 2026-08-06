package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type memAdmin struct {
	mu    sync.Mutex
	perms map[string]RemotePermission
}

func newMemAdmin(seed ...RemotePermission) *memAdmin {
	m := &memAdmin{perms: map[string]RemotePermission{}}
	for _, p := range seed {
		m.perms[p.Name] = p
	}
	return m
}

func (m *memAdmin) FetchPermissions(context.Context, string) ([]RemotePermission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RemotePermission, 0, len(m.perms))
	for _, p := range m.perms {
		out = append(out, p)
	}
	return out, nil
}

func (m *memAdmin) AddPermission(_ context.Context, p RemotePermission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.perms[p.Name]; ok {
		return errAlreadyExists
	}
	m.perms[p.Name] = p
	return nil
}

func (m *memAdmin) UpdatePermission(_ context.Context, p RemotePermission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.perms[p.Name]; !ok {
		return errNotFound
	}
	m.perms[p.Name] = p
	return nil
}

var (
	errAlreadyExists = errString("exists")
	errNotFound      = errString("not found")
)

type errString string

func (e errString) Error() string { return string(e) }

func TestCatalogPermissionName(t *testing.T) {
	got := CatalogPermissionName("resource:sites", "read")
	if got != "catalog-resource-sites-read" {
		t.Fatalf("got %q", got)
	}
	if FullPermissionID("resource:sites", "read") != "resource:sites:read" {
		t.Fatal(FullPermissionID("resource:sites", "read"))
	}
}

func TestRegisterCatalog_AddAndPreserveRoles(t *testing.T) {
	admin := newMemAdmin(RemotePermission{
		Owner:       "default",
		Name:        "catalog-resource-sites-read",
		DisplayName: "old",
		Description: "old",
		Roles:       []string{"default/viewer"},
		Users:       []string{"default/alice"},
		Resources:   []string{"resource:sites"},
		Actions:     []string{"read"},
		Model:       "default/vpp-rbac",
		Effect:      "Allow",
		IsEnabled:   true,
		State:       "Approved",
	})

	cat := Catalog{
		Owner:   "default",
		Model:   "default/vpp-rbac",
		Service: "resource",
		Entries: []CatalogEntry{
			{Object: "resource:sites", Actions: []string{"read", "write"}},
		},
	}
	res, err := RegisterCatalog(context.Background(), admin, cat)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Updated != 1 || res.Skipped != 0 {
		t.Fatalf("result=%+v", res)
	}

	read := admin.perms["catalog-resource-sites-read"]
	if len(read.Roles) != 1 || read.Roles[0] != "default/viewer" {
		t.Fatalf("roles overwritten: %+v", read.Roles)
	}
	if len(read.Users) != 1 || read.Users[0] != "default/alice" {
		t.Fatalf("users overwritten: %+v", read.Users)
	}
	if read.DisplayName != "resource:sites:read" {
		t.Fatalf("displayName=%q", read.DisplayName)
	}

	write := admin.perms["catalog-resource-sites-write"]
	if write.Name == "" || len(write.Roles) != 0 {
		t.Fatalf("write perm: %+v", write)
	}
	if write.Resources[0] != "resource:sites" || write.Actions[0] != "write" {
		t.Fatalf("write resources/actions: %+v", write)
	}
}

func TestRegisterCatalog_SkipUnchanged(t *testing.T) {
	p := RemotePermission{
		Owner:        "default",
		Name:         "catalog-resource-sites-read",
		DisplayName:  "resource:sites:read",
		Description:  catalogDescription("resource", "resource:sites:read"),
		Roles:        []string{"default/viewer"},
		Model:        "default/vpp-rbac",
		ResourceType: "Custom",
		Resources:    []string{"resource:sites"},
		Actions:      []string{"read"},
		Effect:       "Allow",
		IsEnabled:    true,
		State:        "Approved",
	}
	admin := newMemAdmin(p)
	cat := Catalog{
		Owner:   "default",
		Model:   "default/vpp-rbac",
		Service: "resource",
		Entries: []CatalogEntry{{Object: "resource:sites", Actions: []string{"read"}}},
	}
	res, err := RegisterCatalog(context.Background(), admin, cat)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || res.Added != 0 || res.Updated != 0 {
		t.Fatalf("result=%+v", res)
	}
	if got := admin.perms[p.Name].Roles; len(got) != 1 || got[0] != "default/viewer" {
		t.Fatalf("roles=%v", got)
	}
}

func TestCasdoorClient_AddUpdatePermission(t *testing.T) {
	var (
		mu      sync.Mutex
		store   = map[string]RemotePermission{}
		adds    int
		updates int
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "casdoor_session_id", Value: "test"})
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/get-permissions", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		list := make([]RemotePermission, 0, len(store))
		for _, p := range store {
			list = append(list, p)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": list})
	})
	mux.HandleFunc("/api/add-permission", func(w http.ResponseWriter, r *http.Request) {
		var p RemotePermission
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		store[p.Name] = p
		adds++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": true})
	})
	mux.HandleFunc("/api/update-permission", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		var p RemotePermission
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		store[p.Name] = p
		updates++
		mu.Unlock()
		if id != p.Owner+"/"+p.Name {
			t.Errorf("id=%q want %s/%s", id, p.Owner, p.Name)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := NewCasdoorClient(CasdoorClientConfig{
		BaseURL:  srv.URL,
		Password: "123",
	})
	if err != nil {
		t.Fatal(err)
	}

	cat := Catalog{
		Owner:   "default",
		Model:   "default/vpp-rbac",
		Service: "resource",
		Entries: []CatalogEntry{{Object: "resource:sites", Actions: []string{"read"}}},
	}
	res, err := RegisterCatalog(context.Background(), client, cat)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Fatalf("first: %+v", res)
	}

	// Bind a role "in Casdoor", then re-register — must preserve roles via update path.
	mu.Lock()
	p := store["catalog-resource-sites-read"]
	p.Roles = []string{"default/operator"}
	p.DisplayName = "stale"
	store[p.Name] = p
	mu.Unlock()

	res, err = RegisterCatalog(context.Background(), client, cat)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("second: %+v adds=%d updates=%d", res, adds, updates)
	}
	mu.Lock()
	got := store["catalog-resource-sites-read"]
	mu.Unlock()
	if len(got.Roles) != 1 || got.Roles[0] != "default/operator" {
		t.Fatalf("roles=%v", got.Roles)
	}
	if got.DisplayName != "resource:sites:read" {
		t.Fatalf("displayName=%q", got.DisplayName)
	}
}
