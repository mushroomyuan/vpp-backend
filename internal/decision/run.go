package decision

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/decision/config"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

// Run starts logging, optional tracing, then the composition root.
// The decision loop reads enabled policies and keeps plans in memory.
func Run(appCfg *config.Config) error {
	logging.Init(logging.Config{ServiceName: appCfg.ServiceName})

	if appCfg.TelemetryEndpoint != "" {
		shutdown, err := platformtelemetry.InitTracing(context.Background(), platformtelemetry.Config{
			Endpoint:    appCfg.TelemetryEndpoint,
			ServiceName: appCfg.ServiceName,
			Insecure:    appCfg.TelemetryInsecure,
		})
		if err != nil {
			return err
		}
		defer func() {
			if err := shutdown(context.Background()); err != nil {
				logrus.WithError(err).Warn("tracing shutdown error")
			}
		}()
	} else {
		logrus.Warn("tracing.endpoint not configured, tracing is disabled")
	}

	srv, err := createServer(appCfg)
	if err != nil {
		return err
	}

	return srv.PrepareRun().Run()
}
