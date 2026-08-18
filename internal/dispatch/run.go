package dispatch

import (
	"context"

	"github.com/sirupsen/logrus"

	gatewaygrpc "github.com/mushroomyuan/vpp-backend/dispatch/adapter/outbound/gateway_grpc"
	"github.com/mushroomyuan/vpp-backend/dispatch/config"
	"github.com/mushroomyuan/vpp-backend/platform/discovery"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

// Run is the single entry point from configuration to a running server.
func Run(
	appCfg *config.Config,
	dbCfg platformpostgres.Config,
	gatewayCfg gatewaygrpc.Config,
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
		logrus.Info("consul-addr empty, skip Consul registration")
	}

	srv, err := createServer(appCfg, dbCfg, gatewayCfg)
	if err != nil {
		return err
	}

	return srv.PrepareRun().Run()
}
