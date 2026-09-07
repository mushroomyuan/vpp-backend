package decorator

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/time/rate"
)

type applyCmd struct{}

func TestApplyCommandDecorators_NoOptsUnchanged(t *testing.T) {
	t.Parallel()

	inner := handlerFunc[applyCmd, string](func(ctx context.Context, in applyCmd) (string, error) {
		return "ok", nil
	})

	h := ApplyCommandDecorators[applyCmd, string](inner, &fakeMetrics{})

	got, err := h.Handle(context.Background(), applyCmd{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("got %q, want %q", got, "ok")
	}
}

func TestApplyCommandDecorators_WithRateLimiterRejects(t *testing.T) {
	t.Parallel()

	called := false
	inner := handlerFunc[applyCmd, string](func(ctx context.Context, in applyCmd) (string, error) {
		called = true
		return "ok", nil
	})

	limiter := rate.NewLimiter(rate.Limit(0), 0) // always-empty bucket
	metrics := &fakeMetrics{}
	h := ApplyCommandDecorators[applyCmd, string](inner, metrics, WithRateLimiter[applyCmd, string](limiter))

	_, err := h.Handle(context.Background(), applyCmd{})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got err=%v, want ErrRateLimited", err)
	}
	if called {
		t.Fatal("expected inner handler NOT to be called once rate limited")
	}
}

func TestApplyCommandDecorators_WithNilRateLimiterOptionUnchanged(t *testing.T) {
	t.Parallel()

	called := false
	inner := handlerFunc[applyCmd, string](func(ctx context.Context, in applyCmd) (string, error) {
		called = true
		return "ok", nil
	})

	h := ApplyCommandDecorators[applyCmd, string](inner, &fakeMetrics{}, WithRateLimiter[applyCmd, string](nil))

	if _, err := h.Handle(context.Background(), applyCmd{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected inner handler to be called when limiter option is nil")
	}
}

type applyQuery struct{}

func TestApplyQueryDecorators_WithRateLimiterRejects(t *testing.T) {
	t.Parallel()

	called := false
	inner := handlerFunc[applyQuery, string](func(ctx context.Context, in applyQuery) (string, error) {
		called = true
		return "ok", nil
	})

	limiter := rate.NewLimiter(rate.Limit(0), 0)
	h := ApplyQueryDecorators[applyQuery, string](inner, &fakeMetrics{}, WithRateLimiter[applyQuery, string](limiter))

	_, err := h.Handle(context.Background(), applyQuery{})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got err=%v, want ErrRateLimited", err)
	}
	if called {
		t.Fatal("expected inner handler NOT to be called once rate limited")
	}
}
