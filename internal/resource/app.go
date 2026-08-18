package resource

import (
	"fmt"
	"strings"

	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mushroomyuan/vpp-backend/resource/config"
	"github.com/mushroomyuan/vpp-backend/resource/options"
)

// App wraps a cobra.Command and provides a single Run() entry point, following
// the thin-main pattern: cmd/main.go only calls resource.NewApp(...).Run().
type App struct {
	cmd *cobra.Command
}

// Run executes the root cobra command, which handles flag parsing, config
// loading, and delegating to the server startup logic.
func (a *App) Run() {
	if err := a.cmd.Execute(); err != nil {
		logrus.Fatalf("vpp-resource: %v", err)
	}
}

// NewApp constructs the root App. basename is the binary name shown in usage
// output (e.g. "vpp-resource").
func NewApp(basename string) *App {
	opts := options.NewOptions()

	cmd := &cobra.Command{
		Use:   basename,
		Short: "VPP Resource Service",
		Long: `VPP Resource Service manages Sites, Resources, Control Units (CUs),
Measurement Points, and asynchronous import jobs over gRPC and HTTP (grpc-gateway).`,
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

	// dbCfg is assembled here in the composition root and passed straight
	// through to the wiring layer.  The application config (config.Config)
	// intentionally knows nothing about database details.
	dbCfg := dbConfigFromOptions(opts.Database)
	redisCfg := redisConfigFromOptions(opts.Redis)

	return Run(appCfg, dbCfg, redisCfg)
}

// dbConfigFromOptions maps the external database options (filled by viper) to
// the shared platform Config type. The composition root is the only place
// allowed to see both the options shape and the infra type.
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

// loadViperConfig sets up viper to read from a YAML config file. If the
// --config flag points to a file, that file is used; otherwise viper searches
// for "resource.yaml" in common working directories.
func loadViperConfig() {
	if cfgFile := viper.GetString("config-file"); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("resource")
		viper.SetConfigType("yaml")
		// Support running from repo root, internal/resource module root,
		// and IDE-launched working directories.
		viper.AddConfigPath("./config")
		viper.AddConfigPath("../../config")
		viper.AddConfigPath("../../../config")
		viper.AddConfigPath(".")
	}

	viper.AutomaticEnv()
	viper.AllowEmptyEnv(true)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	if err := viper.ReadInConfig(); err != nil {
		logrus.Warnf("no config file loaded, using defaults and environment variables: %v", err)
	} else {
		logrus.Infof("using config file: %s", viper.ConfigFileUsed())
	}
}
