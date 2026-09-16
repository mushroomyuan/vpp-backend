package command

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// DecisionLoop is Optimization's primary process: an independent goroutine
// driven by time.NewTicker, mirroring dispatch's TimeoutScanner (see
// internal/dispatch/application/command/scan_timeouts.go). Each tick runs
// one RunDecisionCycle per configured tenant. A per-cycle failure is
// logged and the loop continues — one bad tenant or a transient Telemetry
// blip must not stop decisions for everyone else, and must not kill the
// process.
//
// Cooldown / debounce lives on Evaluator (in-memory lastFiredAt keyed by
// (CUCode, RuleID)), not here. The loop's job is only to fire slower than
// Telemetry's collection cycle (v1 default 60s vs Simulator's 30s) and to
// hand the tick time into Evaluate so cooldown is deterministic.
type DecisionLoop struct {
	handler  RunDecisionCycleHandler
	interval time.Duration
	tenants  []string
}

// NewDecisionLoop builds a loop. interval <= 0 falls back to 60s (the
// design-plan default, and the smallest value that still sits strictly
// above Telemetry's 30s collection cycle). tenantIDs is copied; an empty
// list makes every tick a no-op (the process still serves /healthz and
// /metrics so it can come up before config/rules are filled in).
func NewDecisionLoop(handler RunDecisionCycleHandler, interval time.Duration, tenantIDs []string) *DecisionLoop {
	if handler == nil {
		panic("NewDecisionLoop: handler is required")
	}
	if interval <= 0 {
		interval = 60 * time.Second
	}
	tenants := make([]string, len(tenantIDs))
	copy(tenants, tenantIDs)
	return &DecisionLoop{
		handler:  handler,
		interval: interval,
		tenants:  tenants,
	}
}

// Run blocks until ctx is cancelled, running one cycle per tenant on each
// tick. It waits for the first tick (same as TimeoutScanner) rather than
// firing immediately on start.
func (l *DecisionLoop) Run(ctx context.Context) error {
	if len(l.tenants) == 0 {
		logging.Warnf(ctx, logrus.Fields{
			"component": "DecisionLoop",
			"interval":  l.interval.String(),
		}, "no tenant IDs configured; ticks will be no-ops")
	}

	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.tick(ctx)
		case <-ctx.Done():
			return nil
		}
	}
}

func (l *DecisionLoop) tick(ctx context.Context) {
	now := time.Now()
	for _, tenantID := range l.tenants {
		if tenantID == "" {
			continue
		}
		_, err := l.handler.Handle(ctx, RunDecisionCycle{TenantID: tenantID, Now: now})
		if err != nil {
			logging.Errorf(ctx, logrus.Fields{
				"component": "DecisionLoop",
				"tenant_id": tenantID,
				"error":     err.Error(),
			}, "decision cycle failed")
		}
	}
}
