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
	calls   []RunDecisionCycle
	err     error
	started chan struct{}
	once    sync.Once
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{started: make(chan struct{})}
}

func (h *recordingHandler) Handle(_ context.Context, cmd RunDecisionCycle) (*RunDecisionCycleResult, error) {
	h.mu.Lock()
	h.calls = append(h.calls, cmd)
	h.mu.Unlock()
	h.once.Do(func() { close(h.started) })
	if h.err != nil {
		return &RunDecisionCycleResult{}, h.err
	}
	return &RunDecisionCycleResult{}, nil
}

func (h *recordingHandler) nCalls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

func (h *recordingHandler) tenants() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.calls))
	for i, c := range h.calls {
		out[i] = c.TenantID
	}
	return out
}

func TestDecisionLoop_TicksHandlerThenStopsOnCancel(t *testing.T) {
	h := newRecordingHandler()
	loop := NewDecisionLoop(h, 20*time.Millisecond, []string{"tenant-1"})

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
	if got := h.tenants()[0]; got != "tenant-1" {
		t.Errorf("tenant = %q, want tenant-1", got)
	}
}

func TestDecisionLoop_HandlerErrorDoesNotStopLoop(t *testing.T) {
	h := newRecordingHandler()
	h.err = errors.New("telemetry blip")
	loop := NewDecisionLoop(h, 15*time.Millisecond, []string{"tenant-1"})

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

func TestDecisionLoop_EvaluatesEachTenantPerTick(t *testing.T) {
	var ticks atomic.Int64
	h := handlerFunc(func(_ context.Context, cmd RunDecisionCycle) (*RunDecisionCycleResult, error) {
		if cmd.TenantID == "b" {
			ticks.Add(1)
		}
		return &RunDecisionCycleResult{}, nil
	})
	loop := NewDecisionLoop(h, 15*time.Millisecond, []string{"a", "b"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = loop.Run(ctx) }()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ticks.Load() >= 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("expected both tenants to be evaluated on a tick")
}

func TestDecisionLoop_EmptyTenantsStillExitsCleanly(t *testing.T) {
	h := newRecordingHandler()
	loop := NewDecisionLoop(h, 10*time.Millisecond, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after cancel")
	}
	if h.nCalls() != 0 {
		t.Errorf("empty tenant list must not call the handler, got %d", h.nCalls())
	}
}

func TestNewDecisionLoop_PanicsWithoutHandler(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewDecisionLoop(nil, time.Second, nil)
}

func TestNewDecisionLoop_ZeroIntervalFallsBackToDefault(t *testing.T) {
	h := newRecordingHandler()
	loop := NewDecisionLoop(h, 0, []string{"t"})
	if loop.interval != 60*time.Second {
		t.Errorf("interval = %s, want 60s default", loop.interval)
	}
}

// handlerFunc adapts a function to RunDecisionCycleHandler.
type handlerFunc func(context.Context, RunDecisionCycle) (*RunDecisionCycleResult, error)

func (f handlerFunc) Handle(ctx context.Context, cmd RunDecisionCycle) (*RunDecisionCycleResult, error) {
	return f(ctx, cmd)
}
