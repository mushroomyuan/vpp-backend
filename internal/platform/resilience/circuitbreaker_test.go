package resilience

import (
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

func TestNewBreaker_DisabledReturnsNil(t *testing.T) {
	t.Parallel()

	cb := NewBreaker[any](BreakerConfig{Enabled: false})
	if cb != nil {
		t.Fatalf("expected nil breaker when disabled, got %v", cb)
	}
}

func TestNewBreaker_TripsOnConsecutiveFailures(t *testing.T) {
	t.Parallel()

	cb := NewBreaker[any](BreakerConfig{
		Enabled:             true,
		Name:                "test-dep",
		ConsecutiveFailures: 2,
		OpenTimeout:         time.Minute,
	})
	if cb == nil {
		t.Fatal("expected non-nil breaker when enabled")
	}

	failing := func() (any, error) {
		return nil, errors.New("boom")
	}

	// First two failures should still execute the request and propagate its error.
	for i := 0; i < 2; i++ {
		_, err := cb.Execute(failing)
		if err == nil || errors.Is(err, gobreaker.ErrOpenState) {
			t.Fatalf("call %d: expected underlying error, got %v", i, err)
		}
	}

	// Third call: breaker should now be open and short-circuit without
	// running the request.
	called := false
	_, err := cb.Execute(func() (any, error) {
		called = true
		return nil, nil
	})
	if called {
		t.Fatal("expected request NOT to be executed once breaker is open")
	}
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("got err=%v, want gobreaker.ErrOpenState", err)
	}
}

func TestNewBreaker_TripsOnFailureRatio(t *testing.T) {
	t.Parallel()

	cb := NewBreaker[any](BreakerConfig{
		Enabled:      true,
		Name:         "test-dep-ratio",
		MinRequests:  4,
		FailureRatio: 0.5,
		OpenTimeout:  time.Minute,
	})
	if cb == nil {
		t.Fatal("expected non-nil breaker when enabled")
	}

	ok := func() (any, error) { return nil, nil }
	bad := func() (any, error) { return nil, errors.New("boom") }

	// 2 successes, 2 failures = 50% failure ratio over MinRequests=4 samples.
	_, _ = cb.Execute(ok)
	_, _ = cb.Execute(bad)
	_, _ = cb.Execute(ok)
	_, _ = cb.Execute(bad)

	_, err := cb.Execute(ok)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("got err=%v, want gobreaker.ErrOpenState after failure ratio trip", err)
	}
}

func TestUnwrapBreakerErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		err     error
		wantErr error
	}{
		{"open state wrapped", gobreaker.ErrOpenState, ErrDependencyUnavailable},
		{"too many requests wrapped", gobreaker.ErrTooManyRequests, ErrDependencyUnavailable},
		{"other error passed through", errors.New("some other error"), nil},
		{"nil passed through", nil, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unwrapBreakerErr(tc.err)
			if tc.wantErr == nil {
				if got != tc.err {
					t.Fatalf("got %v, want passthrough %v", got, tc.err)
				}
				return
			}
			if !errors.Is(got, tc.wantErr) {
				t.Fatalf("got %v, want error wrapping %v", got, tc.wantErr)
			}
		})
	}
}
