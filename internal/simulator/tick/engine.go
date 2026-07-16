package tick

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	plattelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/simulator/runtime"
	"github.com/mushroomyuan/vpp-backend/simulator/telemetry"
)

// Engine periodically advances device state and publishes telemetry.
type Engine struct {
	interval     time.Duration
	manager      *runtime.Manager
	publisher    *telemetry.Publisher
	sampleEvery  uint64 // create spans every Nth tick; 0/1 = every tick
	tickSequence uint64
}

func NewEngine(interval time.Duration, mgr *runtime.Manager, pub *telemetry.Publisher, sampleEvery int) *Engine {
	if sampleEvery < 1 {
		sampleEvery = 1
	}
	return &Engine{
		interval:    interval,
		manager:     mgr,
		publisher:   pub,
		sampleEvery: uint64(sampleEvery),
	}
}

// Run blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	logging.Infof(ctx, logrus.Fields{
		"component":          "tick",
		"interval":           e.interval.String(),
		"devices":            e.manager.Count(),
		"trace_sample_every": e.sampleEvery,
	}, "tick engine started")
	e.step(ctx, e.interval)

	for {
		select {
		case <-ctx.Done():
			logging.Infof(ctx, logrus.Fields{"component": "tick"}, "tick engine stopped")
			return nil
		case <-ticker.C:
			e.step(ctx, e.interval)
		}
	}
}

func (e *Engine) step(ctx context.Context, delta time.Duration) {
	seq := atomic.AddUint64(&e.tickSequence, 1)
	sample := e.sampleEvery <= 1 || seq%e.sampleEvery == 0

	if !sample {
		e.manager.TickAll(delta)
		if e.publisher != nil {
			e.publisher.PublishAll(ctx)
		}
		return
	}

	ctx, span := plattelemetry.Start(ctx, "simulator.tick")
	defer span.End()
	span.SetAttributes(
		attribute.Int64("simulator.tick.seq", int64(seq)),
		attribute.Int64("simulator.tick.interval_ms", delta.Milliseconds()),
		attribute.Int("simulator.device.count", e.manager.Count()),
		attribute.Bool("simulator.publish.enabled", e.publisher != nil),
	)

	e.manager.TickAll(delta)
	if e.publisher != nil {
		pctx, pspan := plattelemetry.Start(ctx, "simulator.publish")
		e.publisher.PublishAll(pctx)
		pspan.End()
	}
}
