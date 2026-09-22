package command

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recordingHandler struct {
	mu      sync.Mutex
	calls   []RunForecastCycle
	err     error
	started chan struct{}
	once    sync.Once
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{started: make(chan struct{})}
}

func (h *recordingHandler) Handle(_ context.Context, cmd RunForecastCycle) (*RunForecastCycleResult, error) {
	h.mu.Lock()
	h.calls = append(h.calls, cmd)
	h.mu.Unlock()
	h.once.Do(func() { close(h.started) })
	if h.err != nil {
		return &RunForecastCycleResult{}, h.err
	}
	return &RunForecastCycleResult{}, nil
}

func (h *recordingHandler) nCalls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

func TestForecastLoop_TicksHandlerThenStopsOnCancel(t *testing.T) {
	h := newRecordingHandler()
	loop := NewForecastLoop(h, 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()

	select {
	case <-h.started:
	case <-time.After(500 * time.Millisecond):
		cancel()
		t.Fatal("timed out waiting for the first tick")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after cancel")
	}

	if h.nCalls() < 1 {
		t.Fatal("expected at least one cycle")
	}
}

func TestForecastLoop_HandlerErrorDoesNotStopLoop(t *testing.T) {
	h := newRecordingHandler()
	h.err = errors.New("telemetry blip")
	loop := NewForecastLoop(h, 15*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if h.nCalls() >= 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected the loop to keep ticking after a handler error, got %d calls", h.nCalls())
}

func TestForecastLoop_CancelBeforeFirstTick(t *testing.T) {
	var ticks atomic.Int64
	h := loopHandlerFunc(func(context.Context, RunForecastCycle) (*RunForecastCycleResult, error) {
		ticks.Add(1)
		return &RunForecastCycleResult{}, nil
	})
	loop := NewForecastLoop(h, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after cancel")
	}
	if ticks.Load() != 0 {
		t.Fatal("must wait for the first tick, not fire on start")
	}
}

func TestNewForecastLoop_PanicsWithoutHandler(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewForecastLoop(nil, time.Second)
}

func TestNewForecastLoop_ZeroIntervalFallsBackToDefault(t *testing.T) {
	h := newRecordingHandler()
	loop := NewForecastLoop(h, 0)
	if loop.interval != 15*time.Minute {
		t.Errorf("interval = %s, want 15m default", loop.interval)
	}
}

type loopHandlerFunc func(context.Context, RunForecastCycle) (*RunForecastCycleResult, error)

func (f loopHandlerFunc) Handle(ctx context.Context, cmd RunForecastCycle) (*RunForecastCycleResult, error) {
	return f(ctx, cmd)
}
