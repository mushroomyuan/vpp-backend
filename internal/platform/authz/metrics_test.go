package authz

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetrics_DecisionAndSync(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("resource")
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}

	c, err := NewCheckerWithMetrics(Config{
		HealthyAfter: 1 * time.Hour,
		StaleAfter:   2 * time.Hour,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ReplacePolicies([]PolicyRule{{"viewer", "resource:*", "read"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	src := &staticSource{perms: []RemotePermission{{
		Roles:     []string{"default/viewer"},
		Resources: []string{"resource:*"},
		Actions:   []string{"read"},
		Model:     "default/vpp-rbac",
		IsEnabled: true,
		State:     "Approved",
		Effect:    "Allow",
	}}}
	s := NewSyncerWithMetrics(src, c, Config{
		Owner:       "default",
		ModelFilter: "default/vpp-rbac",
	}, m)
	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	ok, degraded, err := c.Allow(context.Background(), middleware.Identity{Roles: []string{"viewer"}}, "resource:sites", "read")
	if err != nil || !ok || degraded {
		t.Fatalf("ok=%v degraded=%v err=%v", ok, degraded, err)
	}
	ok, _, err = c.Allow(context.Background(), middleware.Identity{Roles: []string{"viewer"}}, "resource:sites", "write")
	if err != nil || ok {
		t.Fatalf("write should deny ok=%v err=%v", ok, err)
	}

	body := scrape(t, reg)
	for _, want := range []string{
		`authz_policy_sync_last_success_timestamp{service="resource"}`,
		`authz_policy_sync_successes_total{service="resource"}`,
		`authz_decision_total{result="allow",service="resource"}`,
		`authz_decision_total{result="deny",service="resource"}`,
		`authz_policy_tier{service="resource",tier="healthy"} 1`,
		`authz_policy_loaded{service="resource"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q\n%s", want, body)
		}
	}
}

func TestMetrics_SyncFailure(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("resource")
	_ = reg.Register(m.Collector())
	c, err := NewCheckerWithMetrics(Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSyncerWithMetrics(&staticSource{err: context.DeadlineExceeded}, c, Config{Owner: "default"}, m)
	if err := s.SyncOnce(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	body := scrape(t, reg)
	if !strings.Contains(body, `authz_policy_sync_failures_total{service="resource"} 1`) {
		t.Fatalf("body=%s", body)
	}
}

func TestDecisionResult(t *testing.T) {
	if DecisionResult(true, false) != DecisionAllow {
		t.Fatal()
	}
	if DecisionResult(true, true) != DecisionDegradedAllow {
		t.Fatal()
	}
	if DecisionResult(false, true) != DecisionDegradedDeny {
		t.Fatal()
	}
	if DecisionResult(false, false) != DecisionDeny {
		t.Fatal()
	}
}

func scrape(t *testing.T, reg *prometheus.Registry) string {
	t.Helper()
	srv := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
