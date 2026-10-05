package command

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// DecisionCycler runs one decision pass. The loop does not choose tenants;
// the cycler reads enabled policies itself.
type DecisionCycler interface {
	Handle(ctx context.Context, now time.Time) (CycleResult, error)
}

// DecisionLoop ticks the decision cycler. A failed pass is logged and the loop continues.
// Saved plans are submitted by PlanExecutionLoop, not by this ticker.
type DecisionLoop struct {
	handler  DecisionCycler
	interval time.Duration
}

// NewDecisionLoop builds the loop. interval <= 0 falls back to 60s.
func NewDecisionLoop(handler DecisionCycler, interval time.Duration) *DecisionLoop {
	if handler == nil {
		panic("NewDecisionLoop: handler is required")
	}
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &DecisionLoop{handler: handler, interval: interval}
}

// Run blocks until ctx is cancelled. It waits for the first tick.
func (l *DecisionLoop) Run(ctx context.Context) error {
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
	result, err := l.handler.Handle(ctx, now)
	if err != nil {
		logging.Errorf(ctx, logrus.Fields{
			"component": "DecisionLoop",
			"plans":     len(result.Plans),
			"error":     err.Error(),
		}, "decision cycle failed")
	}
}
