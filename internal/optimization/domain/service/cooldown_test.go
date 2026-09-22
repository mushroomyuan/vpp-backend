package service

import (
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
)

func TestCooldownTracker_ActiveAfterRecord(t *testing.T) {
	c := newCooldownTracker(time.Minute)
	now := time.Now()

	if c.active("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now) {
		t.Fatal("expected not active before any record")
	}

	c.record("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now, 0) // 0 -> use default (1m)

	if !c.active("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now.Add(30*time.Second)) {
		t.Error("expected active within the default cooldown window")
	}
	if c.active("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now.Add(90*time.Second)) {
		t.Error("expected not active after the default cooldown window elapses")
	}
}

func TestCooldownTracker_PerRuleCooldownOverridesDefault(t *testing.T) {
	c := newCooldownTracker(time.Hour) // large default
	now := time.Now()

	c.record("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now, 5*time.Second) // rule-specific, shorter

	if c.active("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now.Add(10*time.Second)) {
		t.Error("expected rule-specific cooldown (5s) to override the tracker default (1h)")
	}
}

func TestCooldownTracker_IndependentPerCU(t *testing.T) {
	c := newCooldownTracker(time.Minute)
	now := time.Now()

	c.record("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now, 0)

	if c.active("cu-2", model.RuleSOCThreshold, model.DirectionCharge, now) {
		t.Error("cooldown for cu-1 must not suppress cu-2")
	}
}

// TestCooldownTracker_IndependentPerDirection pins the 2026-09 review fix
// (internal/optimization/review.md #1): a charge trigger's cooldown must
// not suppress a later, legitimate discharge trigger on the same
// (CUCode, RuleID) — that would conflate "don't repeat the same decision"
// with "don't do anything to this CU for a while", which is not what
// anti-oscillation is supposed to mean.
func TestCooldownTracker_IndependentPerDirection(t *testing.T) {
	c := newCooldownTracker(time.Minute)
	now := time.Now()

	c.record("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now, 0)

	if c.active("cu-1", model.RuleSOCThreshold, model.DirectionDischarge, now) {
		t.Error("a charge cooldown must not suppress a discharge trigger on the same CU/rule")
	}
	// The charge direction itself must still be suppressed.
	if !c.active("cu-1", model.RuleSOCThreshold, model.DirectionCharge, now) {
		t.Error("expected the charge direction itself to remain suppressed")
	}
}
