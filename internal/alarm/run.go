package alarm

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/alarm/config"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

// Run is the single entry point from configuration to a running server.
func Run(appCfg *config.Config, dbCfg platformpostgres.Config) error {
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

	srv, err := createServer(appCfg, dbCfg)
	if err != nil {
		return err
	}

	return srv.PrepareRun().Run()
}
