package decorator

import "context"

// Handler is the shared CQRS handler shape for both commands and queries.
type Handler[C, R any] interface {
	Handle(ctx context.Context, in C) (R, error)
}

// CommandHandler is an alias kept for existing call sites.
type CommandHandler[C, R any] = Handler[C, R]

// QueryHandler is an alias kept for existing call sites.
type QueryHandler[C, R any] = Handler[C, R]

// Middleware wraps a Handler (onion style), same idea as HTTP middleware.
type Middleware[C, R any] func(next Handler[C, R]) Handler[C, R]

type handlerFunc[C, R any] func(ctx context.Context, in C) (R, error)

func (f handlerFunc[C, R]) Handle(ctx context.Context, in C) (R, error) {
	return f(ctx, in)
}

// Chain applies middlewares so that mws[0] is outermost.
// Example: Chain(h, Logging, Metrics, Tracing) → Logging(Metrics(Tracing(h))).
func Chain[C, R any](h Handler[C, R], mws ...Middleware[C, R]) Handler[C, R] {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
