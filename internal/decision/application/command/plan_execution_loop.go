package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	appport "github.com/mushroomyuan/vpp-backend/decision/application/port"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// ExecutionDependencies are the ports for the outbox worker.
// Resource is the live catalog, not the scope cache: a stale cache must not authorize a command.
type ExecutionDependencies struct {
	Plans      plan.Repository
	Resource   port.ResourcePort
	Dispatch   appport.DispatchPort
	Lease      time.Duration
	RetryAfter time.Duration
	Interval   time.Duration
	WorkerID   string
}

// PlanExecutionLoop claims due outbox rows and submits one Dispatch task per step.
// It does not elect a leader. A crashed worker's lease expires and another claim
// retries the same idempotency key.
type PlanExecutionLoop struct {
	plans      plan.Repository
	resource   port.ResourcePort
	dispatch   appport.DispatchPort
	lease      time.Duration
	retryAfter time.Duration
	interval   time.Duration
	workerID   string
}

// NewPlanExecutionLoop wires the worker. An unsaved plan is not visible to it.
func NewPlanExecutionLoop(deps ExecutionDependencies) *PlanExecutionLoop {
	if deps.Plans == nil || deps.Resource == nil || deps.Dispatch == nil {
		panic("NewPlanExecutionLoop: plans, resource, and dispatch are required")
	}
	if deps.Lease <= 0 || deps.RetryAfter <= 0 || deps.Interval <= 0 || deps.WorkerID == "" {
		panic("NewPlanExecutionLoop: lease, retry, interval, and worker id are required")
	}
	return &PlanExecutionLoop{
		plans:      deps.Plans,
		resource:   deps.Resource,
		dispatch:   deps.Dispatch,
		lease:      deps.Lease,
		retryAfter: deps.RetryAfter,
		interval:   deps.Interval,
		workerID:   deps.WorkerID,
	}
}

// Run claims due work until ctx is cancelled. The first pass does not wait for the ticker,
// so a lease left by a crashed process is recovered at startup.
func (l *PlanExecutionLoop) Run(ctx context.Context) error {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		l.tick(ctx, time.Now())
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return nil
		}
	}
}

// Tick drains every step that is due at now.
func (l *PlanExecutionLoop) Tick(ctx context.Context, now time.Time) error {
	if now.IsZero() {
		return fmt.Errorf("plan execution: now is required")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		claim, err := l.plans.ClaimDue(ctx, l.workerID, now, l.lease)
		if errors.Is(err, plan.ErrNoneDue) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := l.execute(ctx, claim, now); err != nil {
			logging.Errorf(ctx, logrus.Fields{
				"component": "PlanExecutionLoop",
				"plan_id":   claim.PlanID,
				"step_id":   claim.StepID,
				"attempt":   claim.Attempt,
				"error":     err.Error(),
			}, "plan step outcome was not recorded")
		}
	}
}

func (l *PlanExecutionLoop) tick(ctx context.Context, now time.Time) {
	if err := l.Tick(ctx, now); err != nil && !errors.Is(err, context.Canceled) {
		logging.Errorf(ctx, logrus.Fields{
			"component": "PlanExecutionLoop",
			"error":     err.Error(),
		}, "plan execution pass failed")
	}
}

func (l *PlanExecutionLoop) execute(ctx context.Context, claim plan.Claim, now time.Time) error {
	query, err := policy.SOCScopeQueryFor(claim.TenantID, claim.Scope)
	if err != nil {
		return l.plans.MarkFailed(ctx, claim, err.Error(), now)
	}
	resolved, err := l.resource.ResolveScope(ctx, query)
	if err != nil {
		return l.plans.MarkUncertain(ctx, claim, err.Error(), now, now.Add(l.retryAfter))
	}
	if !catalogMatches(claim, resolved) {
		return l.plans.MarkStale(ctx, claim, "resource revision or binding changed", now)
	}
	task, err := taskFromClaim(claim)
	if err != nil {
		return l.plans.MarkFailed(ctx, claim, err.Error(), now)
	}
	result, err := l.dispatch.SubmitTask(ctx, task)
	if err != nil {
		if uncertainDispatch(err) {
			return l.plans.MarkUncertain(ctx, claim, err.Error(), now, now.Add(l.retryAfter))
		}
		return l.plans.MarkFailed(ctx, claim, err.Error(), now)
	}
	if result.TaskID == "" {
		return l.plans.MarkUncertain(ctx, claim, "dispatch returned an empty task id", now, now.Add(l.retryAfter))
	}
	return l.plans.MarkSubmitted(ctx, claim, result.TaskID, now)
}

func taskFromClaim(claim plan.Claim) (appport.Task, error) {
	if claim.IdempotencyKey == "" || len(claim.Commands) == 0 {
		return appport.Task{}, fmt.Errorf("plan execution: step %s is missing its key or commands", claim.StepID)
	}
	commands := make([]appport.Command, len(claim.Commands))
	for i, cmd := range claim.Commands {
		commands[i] = appport.Command{
			CUCode:   cmd.CUCode,
			MetricID: cmd.MetricID,
			Value:    cmd.Value,
		}
	}
	return appport.Task{
		TenantID:       claim.TenantID,
		Name:           claim.IdempotencyKey,
		IdempotencyKey: claim.IdempotencyKey,
		Commands:       commands,
	}, nil
}

func catalogMatches(claim plan.Claim, resolved port.ResolvedScope) bool {
	if !resolved.PrecheckOK || strings.TrimSpace(resolved.ResourceRevision) != claim.ResourceRevision {
		return false
	}
	members := make(map[string]port.ResolvedCU, len(resolved.Members))
	for _, member := range resolved.Members {
		members[member.CUID] = member
	}
	for _, cmd := range claim.Commands {
		member, ok := members[cmd.CUCode]
		if !ok || !bindingMatches(cmd, member) {
			return false
		}
	}
	return true
}

func bindingMatches(cmd plan.PlannedCommand, member port.ResolvedCU) bool {
	for _, binding := range member.Bindings {
		if binding.MetricID != cmd.MetricID {
			continue
		}
		if !binding.Enabled || binding.Revision != cmd.BindingRevision {
			return false
		}
		switch binding.AccessMode {
		case port.BindingAccessWrite, port.BindingAccessReadWrite:
			return true
		default:
			return false
		}
	}
	return false
}

// uncertainDispatch is true when the caller cannot tell whether Dispatch accepted the task.
// Those attempts retry with the same idempotency key. A definite client error does not.
func uncertainDispatch(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
			codes.PermissionDenied, codes.FailedPrecondition, codes.Unimplemented,
			codes.Unauthenticated:
			return false
		default:
			return true
		}
	}
	return true
}
