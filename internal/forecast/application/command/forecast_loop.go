package command

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

const defaultCycleInterval = 15 * time.Minute

// ForecastLoop is Forecast's primary process: an independent goroutine
// driven by time.NewTicker, mirroring optimization's DecisionLoop. Each
// tick runs one RunForecastCycle covering every enabled target. A
// per-cycle (joined) error is logged and the loop continues — one bad
// target or a transient Telemetry blip must not stop the rest, and must
// not kill the process.
//
// Unlike DecisionLoop, cycle-interval is not a correctness constraint
// (no side-effecting commands); the default 15m matches Telemetry's
// 15-minute aggregate view (design plan §4).
type ForecastLoop struct {
	handler  RunForecastCycleHandler
	interval time.Duration
}

// NewForecastLoop builds a loop. interval <= 0 falls back to 15m.
func NewForecastLoop(handler RunForecastCycleHandler, interval time.Duration) *ForecastLoop {
	if handler == nil {
		panic("NewForecastLoop: handler is required")
	}
	if interval <= 0 {
		interval = defaultCycleInterval
	}
	return &ForecastLoop{handler: handler, interval: interval}
}

// Run blocks until ctx is cancelled, running one cycle on each tick. It
// waits for the first tick (same as DecisionLoop / TimeoutScanner)
// rather than firing immediately on start.
func (l *ForecastLoop) Run(ctx context.Context) error {
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

func (l *ForecastLoop) tick(ctx context.Context) {
	_, err := l.handler.Handle(ctx, RunForecastCycle{Now: time.Now().UTC()})
	if err != nil {
		logging.Errorf(ctx, logrus.Fields{
			"component": "ForecastLoop",
			"error":     err.Error(),
		}, "forecast cycle failed")
	}
}
