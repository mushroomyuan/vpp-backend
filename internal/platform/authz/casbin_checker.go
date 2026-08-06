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
	"github.com/sirupsen/logrus"
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
	metrics     *Metrics
}

// NewCheckerWithMetrics builds an empty enforcer, optionally loads a disk snapshot,
// and attaches C8 observability when metrics is non-nil.
func NewCheckerWithMetrics(cfg Config, metrics *Metrics) (*Checker, error) {
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
		metrics:  metrics,
	}
	if snap, err := loadSnapshot(cfg.SnapshotPath); err != nil {
		return nil, err
	} else if snap != nil {
		if err := c.replacePoliciesLocked(snap.Policies, snap.SyncedAt); err != nil {
			return nil, err
		}
		c.refreshMetricsLocked()
	}
	return c, nil
}

// SetMetrics attaches or replaces the metrics sink (e.g. after construction).
func (c *Checker) SetMetrics(m *Metrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = m
	c.refreshMetricsLocked()
}

// Allow implements PermissionChecker.
func (c *Checker) Allow(_ context.Context, id middleware.Identity, resource, action string) (bool, bool, error) {
	c.mu.RLock()
	tier := c.tierLocked()
	degraded := tier != TierHealthy
	var (
		ok  bool
		err error
	)

	switch tier {
	case TierHealthy, TierStale:
		ok, err = c.enforceLocked(id, resource, action)
	default: // TierInvalid
		if !c.hasPolicies {
			ok = id.HasRole(c.cfg.SafetyNetRole)
		} else if action == "read" && c.cfg.AllowReadWhenInvalid {
			ok, err = c.enforceLocked(id, resource, action)
		} else {
			ok = false
		}
	}
	c.mu.RUnlock()

	if c.metrics != nil {
		prev, cur := c.metrics.RefreshFromChecker(c)
		if prev != cur && (cur == TierStale || cur == TierInvalid) {
			logrus.WithFields(logrus.Fields{
				"component":   "authz",
				"service":     c.metrics.service,
				"from_tier":   prev.String(),
				"to_tier":     cur.String(),
				"last_success": c.LastSuccess(),
				"staleness":   c.Staleness().String(),
				"has_policies": c.HasPolicies(),
			}).Error("authz policy sync tier degraded")
		}
		if err == nil {
			c.metrics.ObserveDecision(ok, degraded)
		}
	}
	return ok, degraded, err
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
	c.refreshMetricsLocked()
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

func (c *Checker) refreshMetricsLocked() {
	if c.metrics == nil {
		return
	}
	// Snapshot fields under lock then update gauges without calling Tier() (would re-lock).
	tier := c.tierLocked()
	var staleSec float64 = -1
	if !c.lastSuccess.IsZero() {
		staleSec = c.now().Sub(c.lastSuccess).Seconds()
		c.metrics.syncLastSuccess.Set(float64(c.lastSuccess.Unix()))
	}
	c.metrics.staleSeconds.Set(staleSec)
	if c.hasPolicies {
		c.metrics.hasPolicies.Set(1)
	} else {
		c.metrics.hasPolicies.Set(0)
	}
	c.metrics.setTierGauges(tier)
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
