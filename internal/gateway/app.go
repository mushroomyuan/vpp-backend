package gateway

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	telemetrygrpc "github.com/mushroomyuan/vpp-backend/gateway/adapter/outbound/telemetry_grpc"
	"github.com/mushroomyuan/vpp-backend/gateway/config"
	"github.com/mushroomyuan/vpp-backend/gateway/options"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

// App wraps a cobra.Command and provides a single Run() entry point.
type App struct {
	cmd *cobra.Command
}

func (a *App) Run() {
	if err := a.cmd.Execute(); err != nil {
		logrus.Fatalf("vpp-gateway: %v", err)
	}
}

func NewApp(basename string) *App {
	opts := options.NewOptions()

	cmd := &cobra.Command{
		Use:   basename,
		Short: "VPP Gateway Service",
		Long: `VPP Gateway Service bridges external EMS/IoT systems and internal
platform services (telemetry, dispatch) over HTTP and gRPC.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApp(opts)
		},
	}

	cmd.PersistentFlags().StringP("config", "c", "", "path to config file (YAML)")
	_ = viper.BindPFlag("config-file", cmd.PersistentFlags().Lookup("config"))

	return &App{cmd: cmd}
}

func runApp(opts *options.Options) error {
	loadViperConfig()

	if err := viper.Unmarshal(opts); err != nil {
		return fmt.Errorf("unmarshal options: %w", err)
	}

	if errs := opts.Validate(); len(errs) != 0 {
		for _, e := range errs {
			logrus.Error(e)
		}
		return fmt.Errorf("invalid configuration, see errors above")
	}

	appCfg := config.CreateFromOptions(opts)
	dbCfg := dbConfigFromOptions(opts.Database)
	telemetryCfg := telemetryConfigFromOptions(opts.TelemetryGRPC)

	return Run(appCfg, dbCfg, telemetryCfg)
}

func dbConfigFromOptions(o options.DatabaseOptions) platformpostgres.Config {
	return platformpostgres.Config{
		Driver:                 o.Driver,
		Host:                   o.Host,
		Port:                   o.Port,
		User:                   o.User,
		Password:               o.Password,
		DBName:                 o.DBName,
		Params:                 o.Params,
		DSN:                    o.DSN,
		MaxOpenConns:           o.MaxOpenConns,
		MaxIdleConns:           o.MaxIdleConns,
		ConnMaxLifetimeSeconds: o.ConnMaxLifetimeSeconds,
		ConnMaxIdleTimeSeconds: o.ConnMaxIdleTimeSeconds,
	}
}

func telemetryConfigFromOptions(o options.TelemetryGRPCOptions) telemetrygrpc.Config {
	return telemetrygrpc.Config{Addr: o.Addr}
}

func loadViperConfig() {
	if cfgFile := viper.GetString("config-file"); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("gateway")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./config")
		viper.AddConfigPath("../../config")
		viper.AddConfigPath("../../../config")
		viper.AddConfigPath(".")
	}

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	if err := viper.ReadInConfig(); err != nil {
		logrus.Warnf("no config file loaded, using defaults and environment variables: %v", err)
	} else {
		logrus.Infof("using config file: %s", viper.ConfigFileUsed())
	}
}
