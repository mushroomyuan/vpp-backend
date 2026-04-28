package decorator

import (
	"context"
)

type CommandHandler[C, R any] interface {
	Handle(ctx context.Context, cmd C) (R, error)
}

func ApplyCommandDecorators[C, R any](handler CommandHandler[C, R], metricsClient MetricsClient) CommandHandler[C, R] {
	return CommandLoggingDecorator[C, R]{
		base: CommandMetricsDecorator[C, R]{
			base:   handler,
			client: metricsClient,
		},
	}
}
