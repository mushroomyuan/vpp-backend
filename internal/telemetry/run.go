package telemetry

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/discovery"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
	kafka "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/kafka"
	"github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/timescaledb"
	"github.com/mushroomyuan/vpp-backend/telemetry/config"
)

// Run is the single entry point from configuration to a running server.
// It initialises optional cross-cutting concerns (tracing, service discovery),
// then delegates to createServer → PrepareRun → Run.
func Run(
	appCfg *config.Config,
	tsCfg timescaledb.Config,
	redisCfg platformredis.Config,
	kafkaCfg kafka.Config,
) error {
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
		logrus.Warn("telemetry.consul-addr not configured, service discovery is disabled")
	}

	srv, err := createServer(appCfg, tsCfg, redisCfg, kafkaCfg)
	if err != nil {
		return err
	}

	return srv.PrepareRun().Run()
}
