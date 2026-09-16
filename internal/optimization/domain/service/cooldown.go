package service

import (
	"sync"
	"time"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
)

// cooldownTracker suppresses repeat triggers for the same (CUCode, RuleID)
// pair within a cooldown window. The decision loop already runs slower
// than Telemetry's collection cycle (discussion §一 risk #5), but that
// alone does not stop a reading sitting right at a threshold boundary
// from firing every single cycle — cooldown is the second half of the
// anti-oscillation story.
//
// v1 keeps this in an in-memory map, not Redis: Optimization runs as a
// single instance, same as the rest of this codebase's Postgres/Kafka/Redis
// (a documented SPOF, see discussion §一 risk #1). Losing cooldown state on
// restart is an acceptable, self-healing failure mode — worst case is one
// extra decision right after a restart, not a correctness bug.
type cooldownTracker struct {
	mu              sync.Mutex
	defaultCooldown time.Duration
	// suppressedUntil maps "<ruleID>:<cuCode>" -> the time before which
	// that rule/CU pair must not fire again.
	suppressedUntil map[string]time.Time
}

func newCooldownTracker(defaultCooldown time.Duration) *cooldownTracker {
	if defaultCooldown <= 0 {
		// Config (a later step) is expected to wire this to 2x the decision
		// loop's interval, per the Optimization design plan §4. This
		// fallback only guards against a zero value slipping through
		// unconfigured — it is not itself a considered default.
		defaultCooldown = 2 * time.Minute
	}
	return &cooldownTracker{
		defaultCooldown: defaultCooldown,
		suppressedUntil: make(map[string]time.Time),
	}
}

func cooldownKey(cuCode string, ruleID model.RuleID) string {
	return string(ruleID) + ":" + cuCode
}

// active reports whether (cuCode, ruleID) is still within its cooldown
// window at time now.
func (c *cooldownTracker) active(cuCode string, ruleID model.RuleID, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	until, ok := c.suppressedUntil[cooldownKey(cuCode, ruleID)]
	return ok && now.Before(until)
}

// record marks (cuCode, ruleID) as having just fired at now, suppressing
// further triggers until now+cooldown. A ruleCooldown <= 0 falls back to
// the tracker's default.
func (c *cooldownTracker) record(cuCode string, ruleID model.RuleID, now time.Time, ruleCooldown time.Duration) {
	cooldown := ruleCooldown
	if cooldown <= 0 {
		cooldown = c.defaultCooldown
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.suppressedUntil[cooldownKey(cuCode, ruleID)] = now.Add(cooldown)
}
