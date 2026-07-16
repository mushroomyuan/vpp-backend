package resource

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/discovery"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/config"
)

// Run is the single entry point from configuration to a running server.
// It initialises optional cross-cutting concerns (tracing), then delegates to
// createServer → PrepareRun → Run.
//
// appCfg carries application-level settings (addresses, worker, telemetry).
// dbCfg is the driver-agnostic database configuration; it is an infrastructure
// concern assembled in the composition root and passed straight through without
// touching any application-layer types.
func Run(appCfg *config.Config, dbCfg platformpostgres.Config, redisCfg platformredis.Config) error {
	logging.Init(logging.Config{ServiceName: appCfg.ServiceName})

	if appCfg.TelemetryEndpoint != "" {
		shutdown, err := telemetry.InitTracing(context.Background(), telemetry.Config{
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
		logrus.Warn("telemetry.endpoint not configured, tracing is disabled")
	}

	if appCfg.ConsulAddr != "" {
		deregister, err := discovery.RegistryToConsul(context.Background(), appCfg.ServiceName, discovery.Config{
			ConsulAddr: appCfg.ConsulAddr,
			GRPCAddr:   appCfg.GRPCAddr,
		})
		if err != nil {
			return err
		}
		defer func() {
			if err := deregister(); err != nil {
				logrus.WithError(err).Warn("consul deregister error")
			}
		}()
	} else {
		logrus.Warn("discovery.consul-addr not configured, service discovery is disabled")
	}

	srv, err := createServer(appCfg, dbCfg, redisCfg)
	if err != nil {
		return err
	}

	return srv.PrepareRun().Run()
}
