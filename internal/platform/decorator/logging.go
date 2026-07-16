package decorator

import (
	"context"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/sirupsen/logrus"
)

// WithLogging logs success/failure around the next handler.
// kind should be "command" or "query".
//
// Fields: kind, action, duration_ms; on failure also error.
// command_id is included when the input has a non-empty CommandID string field.
// Full command/query bodies are intentionally not logged.
func WithLogging[C, R any](kind string) Middleware[C, R] {
	return func(next Handler[C, R]) Handler[C, R] {
		return handlerFunc[C, R](func(ctx context.Context, in C) (result R, err error) {
			start := time.Now()
			action := strings.ToLower(generateActionName(in))
			fields := logrus.Fields{
				"kind":   kind,
				"action": action,
			}
			if id := extractStringField(in, "CommandID"); id != "" {
				fields["command_id"] = id
			}

			defer func() {
				fields["duration_ms"] = time.Since(start).Milliseconds()
				if err == nil {
					logging.Infof(ctx, fields, "%s execute successfully!", titleKind(kind))
				} else {
					fields["error"] = err.Error()
					logging.Errorf(ctx, fields, "Fail to execute %s", kind)
				}
			}()

			return next.Handle(ctx, in)
		})
	}
}

func titleKind(kind string) string {
	switch kind {
	case "command":
		return "Command"
	case "query":
		return "Query"
	default:
		return kind
	}
}
