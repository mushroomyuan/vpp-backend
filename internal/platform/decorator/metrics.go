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

// WithMetrics records request count, latency, and in-flight gauges.
// kind should be "command" or "query".
func WithMetrics[C, R any](kind string, client MetricsClient) Middleware[C, R] {
	return func(next Handler[C, R]) Handler[C, R] {
		return handlerFunc[C, R](func(ctx context.Context, in C) (result R, err error) {
			start := time.Now()
			action := strings.ToLower(generateActionName(in))

			defer client.TrackInFlight(kind, action)()
			defer func() {
				status := "success"
				if err != nil {
					status = "failure"
				}
				client.Count(kind, action, status)
				client.Observe(kind, action, time.Since(start))
			}()

			return next.Handle(ctx, in)
		})
	}
}
