package gateway

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/simulator"
	telemetrygrpc "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/telemetry_grpc"
	"github.com/mushroomyuan/vpp-backend/gateway/config"
	"github.com/mushroomyuan/vpp-backend/platform/discovery"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

// Run is the single entry point from configuration to a running server.
func Run(
	appCfg *config.Config,
	dbCfg platformpostgres.Config,
	telemetryCfg telemetrygrpc.Config,
	simulatorCfg simulator.Config,
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
		logrus.Warn("gateway.consul-addr not configured, service discovery is disabled")
	}

	srv, err := createServer(appCfg, dbCfg, telemetryCfg, simulatorCfg)
	if err != nil {
		return err
	}

	return srv.PrepareRun().Run()
}
