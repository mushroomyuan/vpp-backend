package decorator

import (
	"context"
	"strings"
	"time"
)

type MetricsClient interface {
	Count(kind, action, status string)
	CountN(kind, action, status string, n float64)
	Observe(kind, action string, d time.Duration)
	TrackInFlight(kind, action string) func()
}

type QueryMetricsDecorator[C, R any] struct {
	base   QueryHandler[C, R]
	client MetricsClient
}

func NewQueryMetricsDecorator[C, R any](base QueryHandler[C, R], client MetricsClient) QueryMetricsDecorator[C, R] {
	return QueryMetricsDecorator[C, R]{base: base, client: client}
}

func (q QueryMetricsDecorator[C, R]) Handle(ctx context.Context, cmd C) (R, error) {
	return handleWithMetrics(ctx, cmd, "query", q.client, q.base.Handle)
}

type CommandMetricsDecorator[C, R any] struct {
	base   CommandHandler[C, R]
	client MetricsClient
}

func NewCommandMetricsDecorator[C, R any](base CommandHandler[C, R], client MetricsClient) CommandMetricsDecorator[C, R] {
	return CommandMetricsDecorator[C, R]{base: base, client: client}
}

func (q CommandMetricsDecorator[C, R]) Handle(ctx context.Context, cmd C) (R, error) {
	return handleWithMetrics(ctx, cmd, "command", q.client, q.base.Handle)
}

func handleWithMetrics[C, R any](
	ctx context.Context,
	cmd C,
	kind string,
	client MetricsClient,
	fn func(context.Context, C) (R, error),
) (result R, err error) {
	start := time.Now()
	action := strings.ToLower(generateActionName(cmd))

	defer client.TrackInFlight(kind, action)()

	defer func() {
		status := "success"
		if err != nil {
			status = "failure"
		}
		client.Count(kind, action, status)
		client.Observe(kind, action, time.Since(start))
	}()

	return fn(ctx, cmd)
}
