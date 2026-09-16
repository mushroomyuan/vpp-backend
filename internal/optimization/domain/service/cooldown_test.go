package service

import (
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
)

func TestCooldownTracker_ActiveAfterRecord(t *testing.T) {
	c := newCooldownTracker(time.Minute)
	now := time.Now()

	if c.active("cu-1", model.RuleSOCThreshold, now) {
		t.Fatal("expected not active before any record")
	}

	c.record("cu-1", model.RuleSOCThreshold, now, 0) // 0 -> use default (1m)

	if !c.active("cu-1", model.RuleSOCThreshold, now.Add(30*time.Second)) {
		t.Error("expected active within the default cooldown window")
	}
	if c.active("cu-1", model.RuleSOCThreshold, now.Add(90*time.Second)) {
		t.Error("expected not active after the default cooldown window elapses")
	}
}

func TestCooldownTracker_PerRuleCooldownOverridesDefault(t *testing.T) {
	c := newCooldownTracker(time.Hour) // large default
	now := time.Now()

	c.record("cu-1", model.RuleSOCThreshold, now, 5*time.Second) // rule-specific, shorter

	if c.active("cu-1", model.RuleSOCThreshold, now.Add(10*time.Second)) {
		t.Error("expected rule-specific cooldown (5s) to override the tracker default (1h)")
	}
}

func TestCooldownTracker_IndependentPerCU(t *testing.T) {
	c := newCooldownTracker(time.Minute)
	now := time.Now()

	c.record("cu-1", model.RuleSOCThreshold, now, 0)

	if c.active("cu-2", model.RuleSOCThreshold, now) {
		t.Error("cooldown for cu-1 must not suppress cu-2")
	}
}
