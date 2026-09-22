// Package service holds Optimization's decision-making logic: the rule
// engine (Evaluator) that turns current state into Targets, and Allocate,
// which turns a Target into dispatch-ready CommandSpecs.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// Evaluator runs the v1 rule set against current Telemetry state and
// produces one PointTarget per rule instance that both breaches its
// threshold and is outside its cooldown window.
//
// v1 has exactly one rule *kind* (SOCThresholdRule) with potentially many
// *instances* (one per battery CU). This is why Evaluate iterates
// rules.SOCThresholds directly instead of dispatching through a
// map[RuleID]ruleHandler registry the way alarm's Evaluator does — alarm
// has multiple distinct rule kinds (dispatch task failures vs SOE
// changes) that need routing; Optimization v1 does not yet. If a second
// rule kind is added, revisit whether a registry earns its keep at that
// point (see internal/alarm/DECISION_DESIGN.md for the precedent and its
// stated trigger for introducing that indirection).
type Evaluator struct {
	rules     model.Rules
	telemetry port.TelemetryPort
	cooldown  *cooldownTracker
}

// NewEvaluator builds an Evaluator. defaultCooldown is used whenever a
// rule's own Cooldown is zero (see model.SOCThresholdRule.Cooldown's doc
// comment: zero means "use the default", not "no cooldown").
func NewEvaluator(rules model.Rules, telemetry port.TelemetryPort, defaultCooldown time.Duration) *Evaluator {
	if telemetry == nil {
		panic("NewEvaluator: telemetry is required")
	}
	return &Evaluator{
		rules:     rules,
		telemetry: telemetry,
		cooldown:  newCooldownTracker(defaultCooldown),
	}
}

// Evaluate checks every enabled rule for tenantID at time now, returning a
// Target for each rule that fired. now is passed in (rather than read via
// time.Now() internally) so cooldown behavior is deterministic in tests
// and so DecisionLoop can pass the tick time it already has.
//
// A per-rule read failure (e.g. Telemetry unreachable for one CU) does not
// abort evaluation of the remaining rules — errors are joined and returned
// alongside whatever targets were successfully computed, so one bad CU
// does not block decisions for every other CU in the same cycle.
func (e *Evaluator) Evaluate(ctx context.Context, tenantID string, now time.Time) ([]model.Target, error) {
	var targets []model.Target
	var errs []error

	for _, rule := range e.rules.SOCThresholds {
		if !rule.Enabled {
			continue
		}
		target, fired, err := e.evaluateSOCThreshold(ctx, tenantID, rule, now)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if fired {
			targets = append(targets, target)
		}
	}

	return targets, errors.Join(errs...)
}

func (e *Evaluator) evaluateSOCThreshold(
	ctx context.Context, tenantID string, rule model.SOCThresholdRule, now time.Time,
) (model.PointTarget, bool, error) {
	// Read-then-decide-then-check-cooldown, in that order — not "check
	// cooldown first" like the pre-review version. Cooldown is scoped per
	// direction (model.Direction; see cooldown.go's doc comment for why),
	// and direction is only known after reading the current SOC, so the
	// cooldown check necessarily moves after the read. The v1 optimization
	// of skipping Telemetry entirely while any cooldown is active is gone
	// as a result — that optimization was structurally incompatible with
	// the fix: the exact case it fixes (charge cooldown active, SOC has
	// since swung up to breach MaxSOC) is only detectable by reading
	// Telemetry during the old cooldown window, not by skipping the read.
	snap, err := e.telemetry.GetSnapshot(ctx, tenantID, rule.CUCode)
	if err != nil {
		return model.PointTarget{}, false, fmt.Errorf("soc_threshold[%s]: get snapshot: %w", rule.CUCode, err)
	}
	if snap.Stale {
		// Deciding on stale data is exactly the "反复决策同一份数据" failure
		// mode discussion §一 risk #5 warns about. Skip silently — this is
		// not an error, it's the anti-oscillation guard doing its job.
		return model.PointTarget{}, false, nil
	}

	soc, ok := snap.Metrics[rule.ReadPointKey]
	if !ok {
		return model.PointTarget{}, false, fmt.Errorf(
			"soc_threshold[%s]: metric %q not present in snapshot", rule.CUCode, rule.ReadPointKey,
		)
	}

	var (
		value     model.CommandValue
		direction model.Direction
	)
	switch {
	case soc <= rule.MinSOC:
		value = model.FloatCommandValue(rule.ChargePowerKW)
		direction = model.DirectionCharge
	case soc >= rule.MaxSOC:
		value = model.FloatCommandValue(rule.DischargePowerKW)
		direction = model.DirectionDischarge
	default:
		return model.PointTarget{}, false, nil
	}

	if e.cooldown.active(rule.CUCode, model.RuleSOCThreshold, direction, now) {
		logging.Infof(ctx, logrus.Fields{
			"component":        "DecisionLoop",
			"tenant_id":        tenantID,
			"rule_id":          string(model.RuleSOCThreshold),
			"cu_code":          rule.CUCode,
			"direction":        string(direction),
			"cooldown_skipped": true,
		}, "rule suppressed by cooldown")
		return model.PointTarget{}, false, nil
	}

	e.cooldown.record(rule.CUCode, model.RuleSOCThreshold, direction, now, rule.Cooldown)

	return model.PointTarget{
		Tenant:   tenantID,
		Src:      model.SourceInternalRule,
		Rule:     model.RuleSOCThreshold,
		CUCode:   rule.CUCode,
		PointKey: rule.WritePointKey,
		Value:    value,
	}, true, nil
}
