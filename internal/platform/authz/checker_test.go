package authz

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/middleware"
)

func TestChecker_HealthyAllowDeny(t *testing.T) {
	c, err := NewChecker(Config{
		HealthyAfter: 5 * time.Minute,
		StaleAfter:   30 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	if err := c.ReplacePolicies(c3Policies(), now); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		role, resource, action string
		want                   bool
	}{
		{"viewer", "resource:sites", "read", true},
		{"viewer", "resource:sites", "write", false},
		{"operator", "resource:sites", "write", true},
		{"operator", "resource:sites", "delete", false},
		{"admin", "resource:sites", "delete", true},
		{"admin", "resource:tree", "change-lifecycle", true},
	}
	for _, tc := range cases {
		id := middleware.Identity{Roles: []string{tc.role}}
		ok, degraded, err := c.Allow(context.Background(), id, tc.resource, tc.action)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if degraded {
			t.Fatalf("%+v: unexpectedly degraded", tc)
		}
		if ok != tc.want {
			t.Fatalf("%+v: got %v", tc, ok)
		}
	}
}

func TestChecker_StaleStillUsesCache(t *testing.T) {
	c, err := NewChecker(Config{
		HealthyAfter: 1 * time.Minute,
		StaleAfter:   10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return synced.Add(2 * time.Minute) }
	if err := c.ReplacePolicies(c3Policies(), synced); err != nil {
		t.Fatal(err)
	}
	if c.Tier() != TierStale {
		t.Fatalf("tier=%s", c.Tier())
	}
	ok, degraded, err := c.Allow(context.Background(), middleware.Identity{Roles: []string{"viewer"}}, "resource:sites", "read")
	if err != nil || !ok || !degraded {
		t.Fatalf("ok=%v degraded=%v err=%v", ok, degraded, err)
	}
}

func TestChecker_InvalidFailClosed(t *testing.T) {
	c, err := NewChecker(Config{
		HealthyAfter:         1 * time.Minute,
		StaleAfter:           2 * time.Minute,
		AllowReadWhenInvalid: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return synced.Add(5 * time.Minute) }
	if err := c.ReplacePolicies(c3Policies(), synced); err != nil {
		t.Fatal(err)
	}
	if c.Tier() != TierInvalid {
		t.Fatalf("tier=%s", c.Tier())
	}
	// Even viewer read is denied when AllowReadWhenInvalid=false.
	ok, degraded, err := c.Allow(context.Background(), middleware.Identity{Roles: []string{"viewer"}}, "resource:sites", "read")
	if err != nil || ok || !degraded {
		t.Fatalf("ok=%v degraded=%v err=%v", ok, degraded, err)
	}
	ok, degraded, err = c.Allow(context.Background(), middleware.Identity{Roles: []string{"admin"}}, "resource:sites", "write")
	if err != nil || ok || !degraded {
		t.Fatalf("write ok=%v degraded=%v err=%v", ok, degraded, err)
	}
}

func TestChecker_InvalidAllowReadWhenConfigured(t *testing.T) {
	c, err := NewChecker(Config{
		HealthyAfter:         1 * time.Minute,
		StaleAfter:           2 * time.Minute,
		AllowReadWhenInvalid: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return synced.Add(5 * time.Minute) }
	if err := c.ReplacePolicies(c3Policies(), synced); err != nil {
		t.Fatal(err)
	}
	ok, degraded, err := c.Allow(context.Background(), middleware.Identity{Roles: []string{"viewer"}}, "resource:sites", "read")
	if err != nil || !ok || !degraded {
		t.Fatalf("ok=%v degraded=%v err=%v", ok, degraded, err)
	}
	ok, _, err = c.Allow(context.Background(), middleware.Identity{Roles: []string{"operator"}}, "resource:sites", "write")
	if err != nil || ok {
		t.Fatalf("write must fail-closed, ok=%v err=%v", ok, err)
	}
}

func TestChecker_ColdStartSafetyNet(t *testing.T) {
	c, err := NewChecker(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Tier() != TierInvalid || c.HasPolicies() {
		t.Fatalf("tier=%s has=%v", c.Tier(), c.HasPolicies())
	}
	ok, degraded, err := c.Allow(context.Background(), middleware.Identity{Roles: []string{"admin"}}, "resource:sites", "write")
	if err != nil || !ok || !degraded {
		t.Fatalf("admin safety net: ok=%v degraded=%v err=%v", ok, degraded, err)
	}
	ok, degraded, err = c.Allow(context.Background(), middleware.Identity{Roles: []string{"operator"}}, "resource:sites", "read")
	if err != nil || ok || !degraded {
		t.Fatalf("operator denied: ok=%v degraded=%v err=%v", ok, degraded, err)
	}
}

func TestChecker_SnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	c1, err := NewChecker(Config{SnapshotPath: path})
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	if err := c1.ReplacePolicies(c3Policies(), synced); err != nil {
		t.Fatal(err)
	}

	c2, err := NewChecker(Config{
		SnapshotPath: path,
		HealthyAfter: 5 * time.Minute,
		StaleAfter:   30 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	c2.now = func() time.Time { return synced.Add(1 * time.Minute) }
	if c2.Tier() != TierHealthy {
		t.Fatalf("tier=%s last=%v", c2.Tier(), c2.LastSuccess())
	}
	ok, degraded, err := c2.Allow(context.Background(), middleware.Identity{Roles: []string{"viewer"}}, "resource:cus", "read")
	if err != nil || !ok || degraded {
		t.Fatalf("ok=%v degraded=%v err=%v", ok, degraded, err)
	}
}

func c3Policies() []PolicyRule {
	return []PolicyRule{
		{"viewer", "resource:*", "read"},
		{"operator", "resource:*", "read"},
		{"admin", "resource:*", "read"},
		{"operator", "resource:*", "write"},
		{"admin", "resource:*", "write"},
		{"admin", "resource:*", "delete"},
		{"admin", "resource:*", "change-lifecycle"},
	}
}
