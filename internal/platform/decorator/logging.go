package decorator

import (
	"context"
	"encoding/json"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/sirupsen/logrus"
)

// WithLogging logs success/failure around the next handler.
// kind should be "command" or "query".
func WithLogging[C, R any](kind string) Middleware[C, R] {
	return func(next Handler[C, R]) Handler[C, R] {
		return handlerFunc[C, R](func(ctx context.Context, in C) (result R, err error) {
			body, _ := json.Marshal(in)
			action := generateActionName(in)
			fields := logrus.Fields{
				kind:            action,
				kind + "_body":  string(body),
			}

			defer func() {
				if err == nil {
					logging.Infof(ctx, fields, "%s execute successfully!", titleKind(kind))
				} else {
					logging.Errorf(ctx, fields, "Fail to execute %s: %v", kind, err)
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
