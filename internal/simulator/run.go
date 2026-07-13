package simulator

import (
	"context"

	"github.com/sirupsen/logrus"

	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/simulator/config"
)

// Run is the single entry point from configuration to a running server.
// Consul registration is skipped in v1 (HTTP-only service; platform discovery expects gRPC).
func Run(appCfg *config.Config) error {
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
