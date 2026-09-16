package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	appport "github.com/mushroomyuan/vpp-backend/optimization/application/port"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// evaluator is the Evaluate half of one decision cycle. *service.Evaluator
// satisfies it; tests inject a stub so Allocate/Submit failure paths do not
// have to go through Telemetry.
type evaluator interface {
	Evaluate(ctx context.Context, tenantID string, now time.Time) ([]model.Target, error)
}

// RunDecisionCycle is one pass of "pull state → evaluate rules → Allocate →
// SubmitTask" for a single tenant. DecisionLoop issues one of these per
// tenant per tick.
type RunDecisionCycle struct {
	TenantID string
	// Now is the tick time. Zero means "use time.Now()" — the loop always
	// fills this in so cooldown stays deterministic across a multi-tenant
	// tick; tests pass a frozen clock the same way Evaluator tests do.
	Now time.Time
}

// RunDecisionCycleResult is what one cycle produced. Errors from individual
// targets are joined onto the handler's error return rather than stuffed
// here, matching Evaluator's "partial success + joined error" contract.
type RunDecisionCycleResult struct {
	TargetsFired   int
	TasksSubmitted int
	TaskIDs        []string
}

// RunDecisionCycleHandler is the decorated command handler for one cycle.
type RunDecisionCycleHandler = decorator.CommandHandler[RunDecisionCycle, *RunDecisionCycleResult]

type runDecisionCycleHandler struct {
	evaluator evaluator
	resource  port.ResourcePort
	dispatch  appport.DispatchPort
	observer  port.Observer
}

// NewRunDecisionCycleHandler builds the use-case handler. metricsClient may
// be nil only if the caller is a test that also wraps via the unexported
// constructor; production always passes the process-wide metrics.Client.
func NewRunDecisionCycleHandler(
	evaluator *service.Evaluator,
	resource port.ResourcePort,
	dispatch appport.DispatchPort,
	metricsClient decorator.MetricsClient,
	observer port.Observer,
) RunDecisionCycleHandler {
	if evaluator == nil {
		panic("NewRunDecisionCycleHandler: evaluator is required")
	}
	return newRunDecisionCycleHandler(evaluator, resource, dispatch, metricsClient, observer)
}

func newRunDecisionCycleHandler(
	evaluator evaluator,
	resource port.ResourcePort,
	dispatch appport.DispatchPort,
	metricsClient decorator.MetricsClient,
	observer port.Observer,
) RunDecisionCycleHandler {
	if evaluator == nil {
		panic("NewRunDecisionCycleHandler: evaluator is required")
	}
	if resource == nil {
		panic("NewRunDecisionCycleHandler: resource is required")
	}
	if dispatch == nil {
		panic("NewRunDecisionCycleHandler: dispatch is required")
	}
	if metricsClient == nil {
		metricsClient = nopMetrics{}
	}
	return decorator.ApplyCommandDecorators[RunDecisionCycle, *RunDecisionCycleResult](
		runDecisionCycleHandler{
			evaluator: evaluator,
			resource:  resource,
			dispatch:  dispatch,
			observer:  observer,
		},
		metricsClient,
	)
}

func (h runDecisionCycleHandler) Handle(ctx context.Context, cmd RunDecisionCycle) (out *RunDecisionCycleResult, err error) {
	start := time.Now()
	defer func() {
		if h.observer != nil {
			h.observer.ObserveCycle(time.Since(start), err)
		}
	}()

	if cmd.TenantID == "" {
		return nil, fmt.Errorf("optimization: RunDecisionCycle requires tenant_id")
	}
	now := cmd.Now
	if now.IsZero() {
		now = time.Now()
	}

	targets, evalErr := h.evaluator.Evaluate(ctx, cmd.TenantID, now)

	out = &RunDecisionCycleResult{TargetsFired: len(targets)}
	var errs []error
	if evalErr != nil {
		errs = append(errs, evalErr)
	}

	for _, target := range targets {
		fields := targetLogFields(target)
		if h.observer != nil {
			h.observer.ObserveRulesFired(ruleIDOf(target), 1)
		}
		specs, allocErr := service.Allocate(ctx, h.resource, target)
		if allocErr != nil {
			logging.Errorf(ctx, fields, "Allocate failed: %v", allocErr)
			errs = append(errs, fmt.Errorf("allocate %s: %w", target.Source(), allocErr))
			continue
		}
		if len(specs) == 0 {
			logging.Warnf(ctx, fields, "Allocate produced no commands; skipping SubmitTask")
			continue
		}

		name := taskName(target)
		res, submitErr := h.dispatch.SubmitTask(ctx, target.TenantID(), name, specs)
		if submitErr != nil {
			fields["task_name"] = name
			logging.Errorf(ctx, fields, "SubmitTask failed: %v", submitErr)
			if h.observer != nil {
				h.observer.ObserveSubmit(false)
			}
			errs = append(errs, fmt.Errorf("submit %s: %w", name, submitErr))
			continue
		}
		if h.observer != nil {
			h.observer.ObserveSubmit(true)
		}
		fields["task_id"] = res.TaskID
		fields["task_status"] = res.Status
		fields["command_count"] = len(specs)
		logging.Infof(ctx, fields, "submitted automatic dispatch task")
		out.TasksSubmitted++
		if res.TaskID != "" {
			out.TaskIDs = append(out.TaskIDs, res.TaskID)
		}
	}

	err = errors.Join(errs...)
	logging.InfoWithCost(ctx, logrus.Fields{
		"component":       "DecisionLoop",
		"tenant_id":       cmd.TenantID,
		"targets_fired":   out.TargetsFired,
		"tasks_submitted": out.TasksSubmitted,
	}, start, "decision cycle completed")
	return out, err
}

func taskName(t model.Target) string {
	switch v := t.(type) {
	case model.PointTarget:
		return fmt.Sprintf("opt:%s:%s", v.Source(), v.CUCode)
	case model.AggregateTarget:
		return fmt.Sprintf("opt:%s:aggregate", v.Source())
	default:
		return "opt:unknown"
	}
}

func ruleIDOf(t model.Target) string {
	if pt, ok := t.(model.PointTarget); ok && pt.Source() == model.SourceInternalRule {
		return string(model.RuleSOCThreshold)
	}
	return t.Source()
}

// nopMetrics is used when the caller did not supply a MetricsClient
// (unit tests). Production always passes platform/metrics.Client.
type nopMetrics struct{}

func (nopMetrics) Count(string, string, string)           {}
func (nopMetrics) CountN(string, string, string, float64) {}
func (nopMetrics) Observe(string, string, time.Duration)  {}
func (nopMetrics) TrackInFlight(string, string) func()    { return func() {} }

func targetLogFields(t model.Target) logrus.Fields {
	fields := logrus.Fields{
		"component": "DecisionLoop",
		"tenant_id": t.TenantID(),
		"source":    t.Source(),
	}
	if pt, ok := t.(model.PointTarget); ok {
		fields["cu_code"] = pt.CUCode
		fields["point_key"] = pt.PointKey
		if pt.Source() == model.SourceInternalRule {
			fields["rule_id"] = string(model.RuleSOCThreshold)
		}
	}
	return fields
}
