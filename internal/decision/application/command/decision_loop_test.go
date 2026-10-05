package command

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDecisionLoop_TicksThenStops(t *testing.T) {
	h := &recordingCycle{started: make(chan struct{})}
	loop := NewDecisionLoop(h, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := loop.Run(ctx); err != nil {
			t.Errorf("Run: %v", err)
		}
	}()
	select {
	case <-h.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first tick")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop did not stop")
	}
	if h.n() == 0 {
		t.Fatal("expected at least one cycle")
	}
}

func TestDecisionLoop_HandlerErrorDoesNotStop(t *testing.T) {
	h := &recordingCycle{err: errors.New("boom")}
	loop := NewDecisionLoop(h, 15*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()
	deadline := time.After(time.Second)
	for h.n() < 2 {
		select {
		case <-deadline:
			t.Fatalf("calls = %d, want at least 2", h.n())
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
}

func TestNewDecisionLoop_PanicsWithoutHandler(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewDecisionLoop(nil, time.Second)
}

type recordingCycle struct {
	mu      sync.Mutex
	calls   int
	err     error
	started chan struct{}
	once    sync.Once
}

func (h *recordingCycle) Handle(context.Context, time.Time) (CycleResult, error) {
	h.mu.Lock()
	h.calls++
	h.mu.Unlock()
	h.once.Do(func() {
		if h.started != nil {
			close(h.started)
		}
	})
	if h.err != nil {
		return CycleResult{}, h.err
	}
	return CycleResult{}, nil
}

func (h *recordingCycle) n() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}
