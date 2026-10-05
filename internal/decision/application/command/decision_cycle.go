package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/evaluation"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

var socMetrics = []contracts.MetricID{contracts.MetricEnergyStorageStateOfCharge}

// CycleObserver records one decision pass. Nil methods are not called.
type CycleObserver interface {
	ObserveCycle(d time.Duration, err error)
	ObservePlan(feasibility string)
	ObservePolicy(result string)
}

// CycleResult is the plans produced by one pass.
// Each plan is already saved. Dispatch still runs in PlanExecutionLoop.
type CycleResult struct {
	Plans []*plan.Plan
}

// CycleDependencies are the ports for one decision pass.
type CycleDependencies struct {
	Policies        policy.Repository
	Resolver        dctx.ScopeResolver
	Collector       dctx.StateCollector
	Evaluator       *evaluation.SOCEvaluator
	Submit          SubmitObjectiveHandler
	Observer        CycleObserver
	StaleAge        time.Duration
	Window          time.Duration
	DefaultCooldown time.Duration
}

// RunDecisionCycle reads enabled policies and plans those that fire.
// Submit plans and saves. A lost cooldown claim is a skip. Approval is not consulted yet.
type RunDecisionCycle struct {
	policies        policy.Repository
	resolver        dctx.ScopeResolver
	collector       dctx.StateCollector
	evaluator       *evaluation.SOCEvaluator
	submit          SubmitObjectiveHandler
	observer        CycleObserver
	staleAge        time.Duration
	window          time.Duration
	defaultCooldown time.Duration
}

// NewRunDecisionCycle wires one pass. Dispatch stays in PlanExecutionLoop.
func NewRunDecisionCycle(deps CycleDependencies) *RunDecisionCycle {
	if deps.Policies == nil || deps.Resolver == nil || deps.Collector == nil ||
		deps.Evaluator == nil || deps.Submit == nil {
		panic("NewRunDecisionCycle: policies, resolver, collector, evaluator, and submit handler are required")
	}
	if deps.Window <= 0 || deps.DefaultCooldown <= 0 {
		panic("NewRunDecisionCycle: window and default cooldown must be positive")
	}
	if deps.StaleAge < 0 {
		panic("NewRunDecisionCycle: stale age must not be negative")
	}
	return &RunDecisionCycle{
		policies:        deps.Policies,
		resolver:        deps.Resolver,
		collector:       deps.Collector,
		evaluator:       deps.Evaluator,
		submit:          deps.Submit,
		observer:        deps.Observer,
		staleAge:        deps.StaleAge,
		window:          deps.Window,
		defaultCooldown: deps.DefaultCooldown,
	}
}

// Handle lists enabled policies, one tenant at a time, and plans those that fire.
// A failure on one policy does not stop the others. Skips are not errors.
func (h *RunDecisionCycle) Handle(ctx context.Context, now time.Time) (result CycleResult, err error) {
	started := time.Now()
	defer func() {
		if h.observer != nil {
			h.observer.ObserveCycle(time.Since(started), err)
		}
	}()
	if now.IsZero() {
		return CycleResult{}, fmt.Errorf("decision cycle: now is required")
	}
	policies, err := h.policies.ListEnabled(ctx)
	if err != nil {
		return CycleResult{}, err
	}
	var errs []error
	for _, group := range groupByTenant(policies) {
		plans, groupErr := h.runTenant(ctx, group, now)
		result.Plans = append(result.Plans, plans...)
		if groupErr != nil {
			errs = append(errs, groupErr)
		}
	}
	err = errors.Join(errs...)
	logging.Infof(ctx, logrus.Fields{
		"component": "DecisionLoop",
		"plans":     len(result.Plans),
		"policies":  len(policies),
	}, "decision cycle finished")
	return result, err
}

func (h *RunDecisionCycle) runTenant(ctx context.Context, policies []*policy.Policy, now time.Time) ([]*plan.Plan, error) {
	type item struct {
		policy *policy.Policy
		scope  port.ResolvedScope
		skip   string
		err    error
		state  dctx.ScopeState
	}
	items := make([]item, len(policies))
	var scopes []port.ResolvedScope
	var indexes []int
	for i, current := range policies {
		items[i].policy = current
		query, err := policy.SOCScopeQuery(current)
		if err != nil {
			items[i].err = err
			continue
		}
		resolved, err := h.resolver.Resolve(ctx, query)
		if err != nil {
			if errors.Is(err, dctx.ErrScopeUnavailable) {
				items[i].skip = "scope_unavailable"
				continue
			}
			items[i].err = err
			continue
		}
		if !resolved.PrecheckOK {
			items[i].skip = "precheck"
			continue
		}
		items[i].scope = resolved
		scopes = append(scopes, resolved)
		indexes = append(indexes, i)
	}

	if len(scopes) > 0 {
		batch, err := h.collector.CollectBatch(ctx, policies[0].TenantID, scopes, socMetrics, h.staleAge, now)
		if err != nil || len(batch) != len(scopes) {
			if err == nil {
				err = fmt.Errorf("state collector: batch length %d, want %d", len(batch), len(scopes))
			}
			for _, idx := range indexes {
				items[idx].err = err
			}
		} else {
			for n, idx := range indexes {
				if batch[n].Err != nil {
					if errors.Is(batch[n].Err, dctx.ErrUnusableState) {
						items[idx].skip = evaluation.ReasonUnusableState
						continue
					}
					items[idx].err = batch[n].Err
					continue
				}
				items[idx].state = batch[n].State
			}
		}
	}

	var plans []*plan.Plan
	var errs []error
	for i := range items {
		current := &items[i]
		if current.err != nil {
			h.observePolicy("error")
			errs = append(errs, fmt.Errorf("policy %s: %w", current.policy.ID, current.err))
			logging.Errorf(ctx, logrus.Fields{
				"component": "DecisionLoop",
				"tenant_id": current.policy.TenantID,
				"policy_id": current.policy.ID,
				"error":     current.err.Error(),
			}, "policy evaluation failed")
			continue
		}
		if current.skip != "" {
			h.observePolicy(current.skip)
			h.logSkip(ctx, current.policy, current.skip)
			continue
		}
		dc, err := dctx.NewDecisionContext(current.policy.TenantID, current.policy.Scope, current.scope, current.state, now)
		if err != nil {
			h.observePolicy("error")
			errs = append(errs, fmt.Errorf("policy %s: %w", current.policy.ID, err))
			continue
		}
		outcome, err := h.evaluator.Evaluate(ctx, evaluation.Input{
			Policy: current.policy,
			DC:     dc,
			Now:    now,
			Window: h.window,
		})
		if err != nil {
			h.observePolicy("error")
			errs = append(errs, fmt.Errorf("policy %s: %w", current.policy.ID, err))
			continue
		}
		if outcome.Reason != evaluation.ReasonFired || outcome.Objective == nil {
			reason := outcome.Reason
			if reason == "" {
				reason = "skipped"
			}
			h.observePolicy(reason)
			h.logSkip(ctx, current.policy, reason)
			continue
		}
		planned, err := h.submit.Handle(ctx, SubmitObjective{
			Objective:     outcome.Objective,
			DC:            dc,
			Direction:     outcome.Direction,
			Now:           now,
			CooldownUntil: evaluation.CooldownUntil(current.policy, h.defaultCooldown, now),
		})
		if errors.Is(err, plan.ErrCooldownHeld) {
			h.observePolicy(evaluation.ReasonCooldown)
			h.logSkip(ctx, current.policy, evaluation.ReasonCooldown)
			continue
		}
		if err != nil {
			h.observePolicy("error")
			errs = append(errs, fmt.Errorf("policy %s: %w", current.policy.ID, err))
			continue
		}
		h.observePolicy(evaluation.ReasonFired)
		if h.observer != nil {
			h.observer.ObservePlan(string(planned.Feasibility))
		}
		logging.Infof(ctx, logrus.Fields{
			"component":   "DecisionLoop",
			"tenant_id":   current.policy.TenantID,
			"policy_id":   current.policy.ID,
			"direction":   string(outcome.Direction),
			"plan_id":     planned.ID,
			"feasibility": string(planned.Feasibility),
			"unmet_kw":    planned.UnmetPowerKW,
		}, "policy produced a plan")
		plans = append(plans, planned)
	}
	return plans, errors.Join(errs...)
}

func (h *RunDecisionCycle) logSkip(ctx context.Context, p *policy.Policy, reason string) {
	level := logging.Debugf
	if reason != evaluation.ReasonInBand {
		level = logging.Infof
	}
	level(ctx, logrus.Fields{
		"component": "DecisionLoop",
		"tenant_id": p.TenantID,
		"policy_id": p.ID,
		"reason":    reason,
	}, "policy skipped")
}

func (h *RunDecisionCycle) observePolicy(result string) {
	if h.observer != nil {
		h.observer.ObservePolicy(result)
	}
}

// groupByTenant collects one slice per tenant for a single telemetry batch.
// Policies from the same tenant stay together even when ListEnabled does not return them contiguously.
func groupByTenant(policies []*policy.Policy) map[string][]*policy.Policy {
	groups := make(map[string][]*policy.Policy)
	for _, current := range policies {
		if current == nil {
			continue
		}
		groups[current.TenantID] = append(groups[current.TenantID], current)
	}
	return groups
}
