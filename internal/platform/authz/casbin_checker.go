package authz

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/mushroomyuan/vpp-backend/platform/middleware"
)

//go:embed model.conf
var modelText string

// Checker is a local Casbin PermissionChecker with sync-state / fail tiers.
type Checker struct {
	cfg Config

	mu          sync.RWMutex
	enforcer    *casbin.Enforcer
	lastSuccess time.Time
	hasPolicies bool
	now         func() time.Time // test hook
}

// NewChecker builds an empty enforcer and optionally loads a disk snapshot.
func NewChecker(cfg Config) (*Checker, error) {
	cfg = cfg.withDefaults()
	m, err := model.NewModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("authz model: %w", err)
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("authz enforcer: %w", err)
	}
	c := &Checker{
		cfg:      cfg,
		enforcer: e,
		now:      time.Now,
	}
	if snap, err := loadSnapshot(cfg.SnapshotPath); err != nil {
		return nil, err
	} else if snap != nil {
		if err := c.replacePoliciesLocked(snap.Policies, snap.SyncedAt); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Allow implements PermissionChecker.
func (c *Checker) Allow(_ context.Context, id middleware.Identity, resource, action string) (bool, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tier := c.tierLocked()
	degraded := tier != TierHealthy

	switch tier {
	case TierHealthy, TierStale:
		ok, err := c.enforceLocked(id, resource, action)
		return ok, degraded, err

	default: // TierInvalid
		if !c.hasPolicies {
			// True cold start / empty cache: safety net only.
			return id.HasRole(c.cfg.SafetyNetRole), true, nil
		}
		if action == "read" && c.cfg.AllowReadWhenInvalid {
			ok, err := c.enforceLocked(id, resource, action)
			return ok, true, err
		}
		// Fail-closed for writes / destructive actions (and reads by default).
		return false, true, nil
	}
}

// Tier reports the current sync health band.
func (c *Checker) Tier() Tier {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tierLocked()
}

// Staleness is time since last successful sync; negative if never synced.
func (c *Checker) Staleness() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastSuccess.IsZero() {
		return -1
	}
	return c.now().Sub(c.lastSuccess)
}

// LastSuccess returns the last successful sync time (zero if never).
func (c *Checker) LastSuccess() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSuccess
}

// HasPolicies reports whether any p-rules are loaded (snapshot or sync).
func (c *Checker) HasPolicies() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hasPolicies
}

// ReplacePolicies atomically swaps Casbin p-rules and updates sync metadata.
// Callers (Syncer) should pass syncedAt = time of successful fetch.
func (c *Checker) ReplacePolicies(rules []PolicyRule, syncedAt time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.replacePoliciesLocked(rules, syncedAt); err != nil {
		return err
	}
	if c.cfg.SnapshotPath != "" {
		if err := saveSnapshot(c.cfg.SnapshotPath, Snapshot{
			SyncedAt: syncedAt,
			Policies: rules,
		}); err != nil {
			return fmt.Errorf("authz save snapshot: %w", err)
		}
	}
	return nil
}

func (c *Checker) replacePoliciesLocked(rules []PolicyRule, syncedAt time.Time) error {
	c.enforcer.ClearPolicy()
	for _, r := range rules {
		if _, err := c.enforcer.AddPolicy(r[0], r[1], r[2]); err != nil {
			return err
		}
	}
	c.hasPolicies = len(rules) > 0
	c.lastSuccess = syncedAt
	return nil
}

func (c *Checker) tierLocked() Tier {
	if c.lastSuccess.IsZero() {
		return TierInvalid
	}
	stale := c.now().Sub(c.lastSuccess)
	if stale < c.cfg.HealthyAfter {
		return TierHealthy
	}
	if stale < c.cfg.StaleAfter {
		return TierStale
	}
	return TierInvalid
}

func (c *Checker) enforceLocked(id middleware.Identity, resource, action string) (bool, error) {
	if resource == "" || action == "" {
		return false, nil
	}
	for _, role := range id.Roles {
		ok, err := c.enforcer.Enforce(role, resource, action)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}
