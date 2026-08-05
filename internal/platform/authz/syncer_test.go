package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/middleware"
)

type staticSource struct {
	perms []RemotePermission
	err   error
	n     atomic.Int32
}

func (s *staticSource) FetchPermissions(context.Context, string) ([]RemotePermission, error) {
	s.n.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return s.perms, nil
}

func TestSyncer_SyncOnceLoadsPolicies(t *testing.T) {
	c, err := NewChecker(Config{
		HealthyAfter: 5 * time.Minute,
		StaleAfter:   30 * time.Minute,
		ModelFilter:  "default/vpp-rbac",
		Owner:        "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	src := &staticSource{perms: []RemotePermission{{
		Roles:     []string{"default/viewer"},
		Resources: []string{"resource:*"},
		Actions:   []string{"read"},
		Model:     "default/vpp-rbac",
		Effect:    "Allow",
		IsEnabled: true,
		State:     "Approved",
	}}}
	s := NewSyncer(src, c, c.cfg)
	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	ok, _, err := c.Allow(context.Background(), middleware.Identity{Roles: []string{"viewer"}}, "resource:sites", "read")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if src.n.Load() != 1 {
		t.Fatalf("fetches=%d", src.n.Load())
	}
}

func TestCasdoorClient_FetchPermissions(t *testing.T) {
	var logins atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		logins.Add(1)
		http.SetCookie(w, &http.Cookie{Name: "casdoor_session_id", Value: "test"})
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/get-permissions", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("casdoor_session_id"); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("owner") != "default" {
			t.Errorf("owner=%q", r.URL.Query().Get("owner"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"data": []RemotePermission{{
				Name:      "vpp-resource-read",
				Roles:     []string{"default/admin"},
				Resources: []string{"resource:*"},
				Actions:   []string{"read"},
				Model:     "default/vpp-rbac",
				IsEnabled: true,
				State:     "Approved",
			}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := NewCasdoorClient(CasdoorClientConfig{
		BaseURL:  srv.URL,
		Username: "admin",
		Password: "123",
	})
	if err != nil {
		t.Fatal(err)
	}
	perms, err := client.FetchPermissions(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(perms) != 1 || perms[0].Name != "vpp-resource-read" {
		t.Fatalf("%+v", perms)
	}
	if logins.Load() != 1 {
		t.Fatalf("logins=%d", logins.Load())
	}
}
