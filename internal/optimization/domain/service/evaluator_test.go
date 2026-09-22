package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
)

// fakeTelemetry is an in-memory port.TelemetryPort for unit tests.
type fakeTelemetry struct {
	snapshots map[string]port.Snapshot // keyed by cuCode
	errs      map[string]error         // keyed by cuCode
}

func newFakeTelemetry() *fakeTelemetry {
	return &fakeTelemetry{
		snapshots: make(map[string]port.Snapshot),
		errs:      make(map[string]error),
	}
}

func (f *fakeTelemetry) GetSnapshot(_ context.Context, _, cuCode string) (port.Snapshot, error) {
	if err, ok := f.errs[cuCode]; ok {
		return port.Snapshot{}, err
	}
	return f.snapshots[cuCode], nil
}

func lowSOCRule(cuCode string) model.SOCThresholdRule {
	return model.SOCThresholdRule{
		Enabled:          true,
		CUCode:           cuCode,
		ReadPointKey:     "soc",
		WritePointKey:    "active_power_setpoint_kw",
		MinSOC:           20,
		MaxSOC:           90,
		ChargePowerKW:    50,
		DischargePowerKW: -50,
	}
}

func TestEvaluator_FiresChargeBelowMinSOC(t *testing.T) {
	tel := newFakeTelemetry()
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 15}}

	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{lowSOCRule("cu-1")}}
	e := NewEvaluator(rules, tel, time.Minute)

	targets, err := e.Evaluate(context.Background(), "tenant-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	pt, ok := targets[0].(model.PointTarget)
	if !ok {
		t.Fatalf("expected PointTarget, got %T", targets[0])
	}
	if pt.CUCode != "cu-1" || pt.PointKey != "active_power_setpoint_kw" {
		t.Errorf("unexpected target: %+v", pt)
	}
	if pt.Value.FloatValue == nil || *pt.Value.FloatValue != 50 {
		t.Errorf("expected charge power 50, got %+v", pt.Value)
	}
	if pt.Source() != model.SourceInternalRule {
		t.Errorf("expected Source() = %q, got %q", model.SourceInternalRule, pt.Source())
	}
	if pt.RuleID() != string(model.RuleSOCThreshold) {
		t.Errorf("expected RuleID() = %q, got %q", model.RuleSOCThreshold, pt.RuleID())
	}
}

func TestEvaluator_FiresDischargeAboveMaxSOC(t *testing.T) {
	tel := newFakeTelemetry()
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 95}}

	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{lowSOCRule("cu-1")}}
	e := NewEvaluator(rules, tel, time.Minute)

	targets, err := e.Evaluate(context.Background(), "tenant-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	pt := targets[0].(model.PointTarget)
	if pt.Value.FloatValue == nil || *pt.Value.FloatValue != -50 {
		t.Errorf("expected discharge power -50, got %+v", pt.Value)
	}
}

func TestEvaluator_NoTriggerWithinRange(t *testing.T) {
	tel := newFakeTelemetry()
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 50}}

	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{lowSOCRule("cu-1")}}
	e := NewEvaluator(rules, tel, time.Minute)

	targets, err := e.Evaluate(context.Background(), "tenant-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected no targets, got %d", len(targets))
	}
}

func TestEvaluator_SkipsStaleSnapshotWithoutError(t *testing.T) {
	tel := newFakeTelemetry()
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 5}, Stale: true}

	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{lowSOCRule("cu-1")}}
	e := NewEvaluator(rules, tel, time.Minute)

	targets, err := e.Evaluate(context.Background(), "tenant-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected stale snapshot to be skipped, got %d targets", len(targets))
	}
}

func TestEvaluator_SkipsDisabledRule(t *testing.T) {
	tel := newFakeTelemetry()
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 5}}

	rule := lowSOCRule("cu-1")
	rule.Enabled = false
	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{rule}}
	e := NewEvaluator(rules, tel, time.Minute)

	targets, err := e.Evaluate(context.Background(), "tenant-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected disabled rule to produce no targets, got %d", len(targets))
	}
}

func TestEvaluator_CooldownSuppressesRepeatFire(t *testing.T) {
	tel := newFakeTelemetry()
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 5}}

	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{lowSOCRule("cu-1")}}
	e := NewEvaluator(rules, tel, time.Minute)

	now := time.Now()
	targets, err := e.Evaluate(context.Background(), "tenant-1", now)
	if err != nil || len(targets) != 1 {
		t.Fatalf("expected first cycle to fire once, got targets=%d err=%v", len(targets), err)
	}

	// Same stale-but-still-breaching reading, next tick within cooldown.
	targets, err = e.Evaluate(context.Background(), "tenant-1", now.Add(10*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected cooldown to suppress the second cycle, got %d targets", len(targets))
	}

	// Past the cooldown window: fires again.
	targets, err = e.Evaluate(context.Background(), "tenant-1", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected rule to fire again after cooldown elapses, got %d targets", len(targets))
	}
}

// TestEvaluator_CooldownDoesNotSuppressOppositeDirection pins the 2026-09
// review fix (internal/optimization/review.md #1): a charge trigger's
// cooldown must not swallow a later, legitimate discharge trigger on the
// same CU/rule within the same cooldown window — that is a different
// decision, not a repeat of the same one, and anti-oscillation was never
// supposed to suppress it.
func TestEvaluator_CooldownDoesNotSuppressOppositeDirection(t *testing.T) {
	tel := newFakeTelemetry()
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 5}} // breach low

	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{lowSOCRule("cu-1")}}
	e := NewEvaluator(rules, tel, time.Minute)

	now := time.Now()
	targets, err := e.Evaluate(context.Background(), "tenant-1", now)
	if err != nil || len(targets) != 1 {
		t.Fatalf("expected first cycle to fire a charge, got targets=%d err=%v", len(targets), err)
	}
	if v := targets[0].(model.PointTarget).Value.FloatValue; v == nil || *v != 50 {
		t.Fatalf("expected the first target to be a charge (50), got %+v", targets[0])
	}

	// Still within the charge cooldown window, but SOC has since swung to
	// breach MaxSOC — a discharge is a different, legitimate decision and
	// must fire despite the still-active charge cooldown.
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 95}}
	targets, err = e.Evaluate(context.Background(), "tenant-1", now.Add(10*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected the discharge to fire despite the active charge cooldown, got %d targets", len(targets))
	}
	if v := targets[0].(model.PointTarget).Value.FloatValue; v == nil || *v != -50 {
		t.Fatalf("expected a discharge target (-50), got %+v", targets[0])
	}

	// The charge direction itself is still suppressed by its own cooldown.
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{"soc": 5}}
	targets, err = e.Evaluate(context.Background(), "tenant-1", now.Add(15*time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected the charge direction to remain suppressed by its own cooldown, got %d targets", len(targets))
	}
}

func TestEvaluator_MissingMetricYieldsErrorButNotPanic(t *testing.T) {
	tel := newFakeTelemetry()
	tel.snapshots["cu-1"] = port.Snapshot{CUCode: "cu-1", Metrics: map[string]float64{}} // no "soc" key

	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{lowSOCRule("cu-1")}}
	e := NewEvaluator(rules, tel, time.Minute)

	targets, err := e.Evaluate(context.Background(), "tenant-1", time.Now())
	if err == nil {
		t.Fatal("expected an error for a missing metric")
	}
	if len(targets) != 0 {
		t.Fatalf("expected no targets, got %d", len(targets))
	}
}

func TestEvaluator_OneRuleFailureDoesNotBlockOthers(t *testing.T) {
	tel := newFakeTelemetry()
	tel.errs["cu-broken"] = errors.New("telemetry unavailable")
	tel.snapshots["cu-ok"] = port.Snapshot{CUCode: "cu-ok", Metrics: map[string]float64{"soc": 5}}

	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{
		lowSOCRule("cu-broken"),
		lowSOCRule("cu-ok"),
	}}
	e := NewEvaluator(rules, tel, time.Minute)

	targets, err := e.Evaluate(context.Background(), "tenant-1", time.Now())
	if err == nil {
		t.Fatal("expected the cu-broken failure to surface as an error")
	}
	if len(targets) != 1 {
		t.Fatalf("expected cu-ok to still fire despite cu-broken failing, got %d targets", len(targets))
	}
	if targets[0].(model.PointTarget).CUCode != "cu-ok" {
		t.Errorf("expected the successful target to be for cu-ok, got %+v", targets[0])
	}
}

func TestNewEvaluator_PanicsWithoutTelemetry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected NewEvaluator to panic when telemetry is nil")
		}
	}()
	NewEvaluator(model.Rules{}, nil, time.Minute)
}
