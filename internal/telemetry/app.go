package telemetry

import (
	"fmt"
	"strings"

	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	kafkapub "github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/kafka_pub"
	"github.com/mushroomyuan/vpp-backend/telemetry/adapter/outbound/timescaledb"
	"github.com/mushroomyuan/vpp-backend/telemetry/config"
	"github.com/mushroomyuan/vpp-backend/telemetry/options"
)

// App wraps a cobra.Command and provides a single Run() entry point, following
// the thin-main pattern: cmd/main.go only calls telemetry.NewApp(...).Run().
type App struct {
	cmd *cobra.Command
}

// Run executes the root cobra command, which handles flag parsing, config
// loading, and delegating to the server startup logic.
func (a *App) Run() {
	if err := a.cmd.Execute(); err != nil {
		logrus.Fatalf("vpp-telemetry: %v", err)
	}
}

// NewApp constructs the root App. basename is the binary name shown in usage
// output (e.g. "vpp-telemetry").
func NewApp(basename string) *App {
	opts := options.NewOptions()

	cmd := &cobra.Command{
		Use:   basename,
		Short: "VPP Telemetry Service",
		Long: `VPP Telemetry Service ingests, stores, and queries time-series
telemetry data from Control Units (CUs) over gRPC.`,
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

// runApp loads configuration and launches the server. It is the boundary
// between cobra's flag layer and the internal server wiring.
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
	tsCfg := tsConfigFromOptions(opts.TimescaleDB)
	redisCfg := redisConfigFromOptions(opts.Redis)
	kafkaCfg := kafkaConfigFromOptions(opts.Kafka)

	return Run(appCfg, tsCfg, redisCfg, kafkaCfg)
}

func tsConfigFromOptions(o options.TimescaleDBOptions) timescaledb.Config {
	return timescaledb.Config{
		Host:     o.Host,
		Port:     o.Port,
		User:     o.User,
		Password: o.Password,
		DBName:   o.DBName,
		SSLMode:  o.SSLMode,
		DSN:      o.DSN,
		MaxConns: o.MaxConns,
		MinConns: o.MinConns,
	}
}

func redisConfigFromOptions(o options.RedisOptions) platformredis.Config {
	return platformredis.Config{
		Addr:                o.Addr,
		Password:            o.Password,
		DB:                  o.DB,
		PoolSize:            o.PoolSize,
		MinIdleConns:        o.MinIdleConns,
		DialTimeoutSeconds:  o.DialTimeoutSeconds,
		ReadTimeoutSeconds:  o.ReadTimeoutSeconds,
		WriteTimeoutSeconds: o.WriteTimeoutSeconds,
		PingTimeoutSeconds:  o.PingTimeoutSeconds,
	}
}

func kafkaConfigFromOptions(o options.KafkaOptions) kafkapub.Config {
	return kafkapub.Config{
		Brokers: o.Brokers,
		Topic:   o.Topic,
	}
}

// loadViperConfig sets up viper to read from a YAML config file. If the
// --config flag points to a file, that file is used; otherwise viper searches
// for "telemetry.yaml" in common working directories.
func loadViperConfig() {
	if cfgFile := viper.GetString("config-file"); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("telemetry")
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
