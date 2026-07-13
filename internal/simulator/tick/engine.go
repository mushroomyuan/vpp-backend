package tick

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/simulator/runtime"
	"github.com/mushroomyuan/vpp-backend/simulator/telemetry"
)

// Engine periodically advances device state and publishes telemetry.
type Engine struct {
	interval  time.Duration
	manager   *runtime.Manager
	publisher *telemetry.Publisher
}

func NewEngine(interval time.Duration, mgr *runtime.Manager, pub *telemetry.Publisher) *Engine {
	return &Engine{
		interval:  interval,
		manager:   mgr,
		publisher: pub,
	}
}

// Run blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	logrus.Infof("tick engine started interval=%s devices=%d", e.interval, e.manager.Count())
	e.step(ctx, e.interval)

	for {
		select {
		case <-ctx.Done():
			logrus.Info("tick engine stopped")
			return nil
		case <-ticker.C:
			e.step(ctx, e.interval)
		}
	}
}

func (e *Engine) step(ctx context.Context, delta time.Duration) {
	e.manager.TickAll(delta)
	if e.publisher != nil {
		e.publisher.PublishAll(ctx)
	}
}
