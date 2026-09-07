package decorator

// ApplyCommandDecorators wraps a command handler as:
//
//	Logging → [RateLimit] → Metrics → Tracing → handler
//
// RateLimit is only inserted when a non-nil limiter is supplied via
// WithRateLimiter (see options.go); by default it is absent, so calling this
// without opts is identical to the previous Logging → Metrics → Tracing
// chain.
func ApplyCommandDecorators[C, R any](handler CommandHandler[C, R], metricsClient MetricsClient, opts ...Option[C, R]) CommandHandler[C, R] {
	cfg := &decoratorConfig[C, R]{}
	for _, opt := range opts {
		opt(cfg)
	}

	mws := []Middleware[C, R]{WithLogging[C, R]("command")}
	if cfg.rateLimiter != nil {
		mws = append(mws, WithRateLimit[C, R](cfg.rateLimiter, metricsClient))
	}
	mws = append(mws,
		WithMetrics[C, R]("command", metricsClient),
		WithTracing[C, R]("command"),
	)
	return Chain(handler, mws...)
}

// ApplyQueryDecorators wraps a query handler as:
//
//	Logging → [RateLimit] → Metrics → Tracing → handler
//
// See ApplyCommandDecorators for the RateLimit insertion rule.
func ApplyQueryDecorators[H, R any](handler QueryHandler[H, R], metricsClient MetricsClient, opts ...Option[H, R]) QueryHandler[H, R] {
	cfg := &decoratorConfig[H, R]{}
	for _, opt := range opts {
		opt(cfg)
	}

	mws := []Middleware[H, R]{WithLogging[H, R]("query")}
	if cfg.rateLimiter != nil {
		mws = append(mws, WithRateLimit[H, R](cfg.rateLimiter, metricsClient))
	}
	mws = append(mws,
		WithMetrics[H, R]("query", metricsClient),
		WithTracing[H, R]("query"),
	)
	return Chain(handler, mws...)
}
