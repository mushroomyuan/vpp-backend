package simulator

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mushroomyuan/vpp-backend/simulator/config"
	"github.com/mushroomyuan/vpp-backend/simulator/options"
)

// App wraps a cobra.Command and provides a single Run() entry point.
type App struct {
	cmd *cobra.Command
}

func (a *App) Run() {
	if err := a.cmd.Execute(); err != nil {
		logrus.Fatalf("vpp-simulator: %v", err)
	}
}

func NewApp(basename string) *App {
	opts := options.NewOptions()

	cmd := &cobra.Command{
		Use:   basename,
		Short: "VPP Simulator Service",
		Long: `VPP Simulator is a virtual device runtime that loads CUs from Resource,
emits telemetry via Gateway, and accepts control commands for closed-loop testing.`,
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
	return Run(appCfg)
}

func loadViperConfig() {
	if cfgFile := viper.GetString("config-file"); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("simulator")
		viper.SetConfigType("yaml")
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
