package resilience

import (
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
)

// ErrDependencyUnavailable wraps gobreaker's open-state errors so call sites
// don't need to import gobreaker directly. Callers can errors.Is() against
// this instead of gobreaker.ErrOpenState.
var ErrDependencyUnavailable = errors.New("resilience: dependency unavailable (circuit open)")

// BreakerConfig configures one CircuitBreaker instance for one outbound
// dependency (e.g. "dispatch->gateway"). Enabled=false (the default) means
// NewBreaker returns nil, and interceptors/transports treat nil as "no breaker".
type BreakerConfig struct {
	Enabled bool
	Name    string // dependency label, used in logs and gobreaker.Settings.Name

	// Trip conditions (OR'd together): either is sufficient to open the breaker.
	ConsecutiveFailures uint32  // 0 disables this condition
	MinRequests         uint32  // sample-based: only consider FailureRatio once Requests >= this
	FailureRatio        float64 // e.g. 0.5

	OpenTimeout time.Duration // how long the breaker stays Open before probing (half-open)

	// IsSuccessful classifies an error as "not a dependency failure" (e.g. a
	// gRPC NotFound is the dependency working correctly). nil = only err==nil
	// counts as success (gobreaker default).
	IsSuccessful func(err error) bool
}

// NewBreaker builds a *gobreaker.CircuitBreaker[T] from cfg, or returns nil
// (disabled) when cfg.Enabled is false. T is the type parameter gobreaker
// uses for Execute's return value; gRPC/HTTP wrappers pick a concrete T.
func NewBreaker[T any](cfg BreakerConfig) *gobreaker.CircuitBreaker[T] {
	if !cfg.Enabled {
		return nil
	}
	return gobreaker.NewCircuitBreaker[T](gobreaker.Settings{
		Name:    cfg.Name,
		Timeout: cfg.OpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if cfg.ConsecutiveFailures > 0 && counts.ConsecutiveFailures >= cfg.ConsecutiveFailures {
				return true
			}
			if cfg.MinRequests > 0 && counts.Requests >= cfg.MinRequests {
				return float64(counts.TotalFailures)/float64(counts.Requests) >= cfg.FailureRatio
			}
			return false
		},
		IsSuccessful: cfg.IsSuccessful,
		OnStateChange: func(name string, from, to gobreaker.State) {
			logrus.WithFields(logrus.Fields{
				"component":  "resilience.CircuitBreaker",
				"dependency": name,
				"from":       from.String(),
				"to":         to.String(),
			}).Warn("circuit breaker state changed")
		},
	})
}

// unwrapBreakerErr turns gobreaker's sentinel errors into ErrDependencyUnavailable.
func unwrapBreakerErr(err error) error {
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return fmt.Errorf("%w: %s", ErrDependencyUnavailable, err.Error())
	}
	return err
}
