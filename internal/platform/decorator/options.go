package decorator

import "golang.org/x/time/rate"

// Option configures optional decorators layered on top of the default
// Logging → Metrics → Tracing chain applied by ApplyCommandDecorators /
// ApplyQueryDecorators. The zero value (no options passed) preserves
// today's behavior exactly, so all existing call sites keep compiling and
// behaving the same without changes.
type Option[C, R any] func(*decoratorConfig[C, R])

type decoratorConfig[C, R any] struct {
	rateLimiter *rate.Limiter
}

// WithRateLimiter enables rate limiting for this handler by inserting
// WithRateLimit into the chain (right after Logging, before Metrics/
// Tracing/the handler). Pass nil, or omit this option entirely, to leave
// rate limiting disabled for the handler.
func WithRateLimiter[C, R any](limiter *rate.Limiter) Option[C, R] {
	return func(c *decoratorConfig[C, R]) { c.rateLimiter = limiter }
}
