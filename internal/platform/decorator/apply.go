package decorator

// ApplyCommandDecorators wraps a command handler as:
//
//	Logging → Metrics → Tracing → handler
func ApplyCommandDecorators[C, R any](handler CommandHandler[C, R], metricsClient MetricsClient) CommandHandler[C, R] {
	return Chain(handler,
		WithLogging[C, R]("command"),
		WithMetrics[C, R]("command", metricsClient),
		WithTracing[C, R]("command"),
	)
}

// ApplyQueryDecorators wraps a query handler as:
//
//	Logging → Metrics → Tracing → handler
func ApplyQueryDecorators[H, R any](handler QueryHandler[H, R], metricsClient MetricsClient) QueryHandler[H, R] {
	return Chain(handler,
		WithLogging[H, R]("query"),
		WithMetrics[H, R]("query", metricsClient),
		WithTracing[H, R]("query"),
	)
}
