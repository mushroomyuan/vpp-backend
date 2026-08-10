package authz

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/identity"
)

func TestChecker_HealthyAllowDeny(t *testing.T) {
	c, err := NewCheckerWithMetrics(Config{
		HealthyAfter: 5 * time.Minute,
		StaleAfter:   30 * time.Minute,
	}, nil)
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
		principal := identity.Principal{Roles: []string{tc.role}}
		decision, err := c.Allow(context.Background(), principal, tc.resource, tc.action)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if decision.Degraded {
			t.Fatalf("%+v: unexpectedly degraded", tc)
		}
		if decision.Allowed != tc.want {
			t.Fatalf("%+v: got %v", tc, decision.Allowed)
		}
	}
}

func TestChecker_StaleStillUsesCache(t *testing.T) {
	c, err := NewCheckerWithMetrics(Config{
		HealthyAfter: 1 * time.Minute,
		StaleAfter:   10 * time.Minute,
	}, nil)
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
	decision, err := c.Allow(context.Background(), identity.Principal{Roles: []string{"viewer"}}, "resource:sites", "read")
	if err != nil || !decision.Allowed || !decision.Degraded {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestChecker_DenyWritesWhenStale(t *testing.T) {
	c, err := NewCheckerWithMetrics(Config{
		HealthyAfter:        1 * time.Minute,
		StaleAfter:          10 * time.Minute,
		DenyWritesWhenStale: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return synced.Add(2 * time.Minute) }
	rules := []PolicyRule{
		{"operator", "dispatch:tasks", "read"},
		{"operator", "dispatch:tasks", "submit"},
	}
	if err := c.ReplacePolicies(rules, synced); err != nil {
		t.Fatal(err)
	}
	principal := identity.Principal{Roles: []string{"operator"}}
	decision, err := c.Allow(context.Background(), principal, "dispatch:tasks", "read")
	if err != nil || !decision.Allowed || !decision.Degraded {
		t.Fatalf("read: decision=%+v err=%v", decision, err)
	}
	decision, err = c.Allow(context.Background(), principal, "dispatch:tasks", "submit")
	if err != nil || decision.Allowed || !decision.Degraded {
		t.Fatalf("submit: decision=%+v err=%v", decision, err)
	}
}

func TestChecker_InvalidFailClosed(t *testing.T) {
	c, err := NewCheckerWithMetrics(Config{
		HealthyAfter:         1 * time.Minute,
		StaleAfter:           2 * time.Minute,
		AllowReadWhenInvalid: false,
	}, nil)
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
	decision, err := c.Allow(context.Background(), identity.Principal{Roles: []string{"viewer"}}, "resource:sites", "read")
	if err != nil || decision.Allowed || !decision.Degraded {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
	decision, err = c.Allow(context.Background(), identity.Principal{Roles: []string{"admin"}}, "resource:sites", "write")
	if err != nil || decision.Allowed || !decision.Degraded {
		t.Fatalf("write decision=%+v err=%v", decision, err)
	}
}

func TestChecker_InvalidAllowReadWhenConfigured(t *testing.T) {
	c, err := NewCheckerWithMetrics(Config{
		HealthyAfter:         1 * time.Minute,
		StaleAfter:           2 * time.Minute,
		AllowReadWhenInvalid: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return synced.Add(5 * time.Minute) }
	if err := c.ReplacePolicies(c3Policies(), synced); err != nil {
		t.Fatal(err)
	}
	decision, err := c.Allow(context.Background(), identity.Principal{Roles: []string{"viewer"}}, "resource:sites", "read")
	if err != nil || !decision.Allowed || !decision.Degraded {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
	decision, err = c.Allow(context.Background(), identity.Principal{Roles: []string{"operator"}}, "resource:sites", "write")
	if err != nil || decision.Allowed {
		t.Fatalf("write must fail-closed, decision=%+v err=%v", decision, err)
	}
}

func TestChecker_ColdStartSafetyNet(t *testing.T) {
	c, err := NewCheckerWithMetrics(Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Tier() != TierInvalid || c.HasPolicies() {
		t.Fatalf("tier=%s has=%v", c.Tier(), c.HasPolicies())
	}
	decision, err := c.Allow(context.Background(), identity.Principal{Roles: []string{"admin"}}, "resource:sites", "write")
	if err != nil || !decision.Allowed || !decision.Degraded {
		t.Fatalf("admin safety net: decision=%+v err=%v", decision, err)
	}
	decision, err = c.Allow(context.Background(), identity.Principal{Roles: []string{"operator"}}, "resource:sites", "read")
	if err != nil || decision.Allowed || !decision.Degraded {
		t.Fatalf("operator denied: decision=%+v err=%v", decision, err)
	}
}

func TestChecker_SnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	c1, err := NewCheckerWithMetrics(Config{SnapshotPath: path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	if err := c1.ReplacePolicies(c3Policies(), synced); err != nil {
		t.Fatal(err)
	}

	c2, err := NewCheckerWithMetrics(Config{
		SnapshotPath: path,
		HealthyAfter: 5 * time.Minute,
		StaleAfter:   30 * time.Minute,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c2.now = func() time.Time { return synced.Add(1 * time.Minute) }
	if c2.Tier() != TierHealthy {
		t.Fatalf("tier=%s last=%v", c2.Tier(), c2.LastSuccess())
	}
	decision, err := c2.Allow(context.Background(), identity.Principal{Roles: []string{"viewer"}}, "resource:cus", "read")
	if err != nil || !decision.Allowed || decision.Degraded {
		t.Fatalf("decision=%+v err=%v", decision, err)
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
