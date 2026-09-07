package decorator

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// fakeMetrics is a minimal MetricsClient test double that records Count calls.
type fakeMetrics struct {
	counts []fakeCount
}

type fakeCount struct {
	kind, action, status string
}

func (f *fakeMetrics) Count(kind, action, status string) {
	f.counts = append(f.counts, fakeCount{kind, action, status})
}
func (f *fakeMetrics) CountN(kind, action, status string, n float64) {}
func (f *fakeMetrics) Observe(kind, action string, d time.Duration)  {}
func (f *fakeMetrics) TrackInFlight(kind, action string) func()      { return func() {} }

type rlCmd struct{}

func TestWithRateLimit_Allows(t *testing.T) {
	t.Parallel()

	called := false
	inner := handlerFunc[rlCmd, string](func(ctx context.Context, in rlCmd) (string, error) {
		called = true
		return "ok", nil
	})

	limiter := rate.NewLimiter(rate.Inf, 1) // never blocks
	metrics := &fakeMetrics{}
	h := WithRateLimit[rlCmd, string](limiter, metrics)(inner)

	got, err := h.Handle(context.Background(), rlCmd{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("got %q, want %q", got, "ok")
	}
	if !called {
		t.Fatal("expected inner handler to be called")
	}
	if len(metrics.counts) != 0 {
		t.Fatalf("expected no metrics recorded, got %v", metrics.counts)
	}
}

func TestWithRateLimit_Rejects(t *testing.T) {
	t.Parallel()

	called := false
	inner := handlerFunc[rlCmd, string](func(ctx context.Context, in rlCmd) (string, error) {
		called = true
		return "ok", nil
	})

	limiter := rate.NewLimiter(rate.Limit(0), 0) // always empty bucket
	metrics := &fakeMetrics{}
	h := WithRateLimit[rlCmd, string](limiter, metrics)(inner)

	_, err := h.Handle(context.Background(), rlCmd{})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got err=%v, want ErrRateLimited", err)
	}
	if called {
		t.Fatal("expected inner handler NOT to be called")
	}
	if len(metrics.counts) != 1 {
		t.Fatalf("expected exactly one metrics count, got %v", metrics.counts)
	}
	got := metrics.counts[0]
	if got.kind != "ratelimit" || got.action != "rlcmd" || got.status != "rejected" {
		t.Fatalf("unexpected count recorded: %+v", got)
	}
}

func TestWithRateLimit_NilLimiterDisabled(t *testing.T) {
	t.Parallel()

	called := false
	inner := handlerFunc[rlCmd, string](func(ctx context.Context, in rlCmd) (string, error) {
		called = true
		return "ok", nil
	})

	h := WithRateLimit[rlCmd, string](nil, &fakeMetrics{})(inner)

	if _, err := h.Handle(context.Background(), rlCmd{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected inner handler to be called when limiter is nil")
	}
}

func TestWithRateLimit_NilMetricsClientDoesNotPanic(t *testing.T) {
	t.Parallel()

	inner := handlerFunc[rlCmd, string](func(ctx context.Context, in rlCmd) (string, error) {
		return "ok", nil
	})

	limiter := rate.NewLimiter(rate.Limit(0), 0)
	h := WithRateLimit[rlCmd, string](limiter, nil)(inner)

	if _, err := h.Handle(context.Background(), rlCmd{}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got err=%v, want ErrRateLimited", err)
	}
}
