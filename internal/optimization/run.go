package optimization

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/optimization/config"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

// Run is the single entry point from configuration to a running process:
// logging, optional tracing, then the composition root (decision loop +
// metrics + /healthz). There is no inbound business gRPC/HTTP — v1 is a
// pure internal decision loop (design plan §3).
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
