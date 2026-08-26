package options

import "fmt"

// Options holds all configurable parameters for the alarm service.
type Options struct {
	Alarm    AlarmOptions    `mapstructure:"alarm"`
	Tracing  TracingOptions  `mapstructure:"tracing"`
	Database DatabaseOptions `mapstructure:"database"`
	Kafka    KafkaOptions    `mapstructure:"kafka"`
}

type AlarmOptions struct {
	HTTPAddr    string `mapstructure:"http-addr"`
	MetricsAddr string `mapstructure:"metrics-addr"`
	ServiceName string `mapstructure:"service-name"`

	Auth  AuthOptions  `mapstructure:"auth"`
	Rules RulesOptions `mapstructure:"rules"`
}

type AuthOptions struct {
	// TrustProxyHeaders requires valid X-Userinfo when true (Path C via APISIX).
	// When false, auth is bypassed for local direct :8087 debugging.
	TrustProxyHeaders bool         `mapstructure:"trust-proxy-headers"`
	Authz             AuthzOptions `mapstructure:"authz"`
}

type AuthzOptions struct {
	Disabled bool `mapstructure:"disabled"`

	CasdoorURL      string `mapstructure:"casdoor-url"`
	CasdoorOrg      string `mapstructure:"casdoor-organization"`
	CasdoorApp      string `mapstructure:"casdoor-application"`
	CasdoorUsername string `mapstructure:"casdoor-username"`
	CasdoorPassword string `mapstructure:"casdoor-password"`

	Owner        string `mapstructure:"owner"`
	ModelFilter  string `mapstructure:"model-filter"`
	SnapshotPath string `mapstructure:"snapshot-path"`

	SyncInterval         string `mapstructure:"sync-interval"`
	HealthyAfter         string `mapstructure:"healthy-after"`
	StaleAfter           string `mapstructure:"stale-after"`
	AllowReadWhenInvalid bool   `mapstructure:"allow-read-when-invalid"`

	DisableRegisterCatalog bool `mapstructure:"disable-register-catalog"`
}

type RulesOptions struct {
	DispatchTaskFailed RuleOptions    `mapstructure:"dispatch-task-failed"`
	SOEDiscreteChange  SOERuleOptions `mapstructure:"soe-discrete-change"`
}

type RuleOptions struct {
	Enabled  bool   `mapstructure:"enabled"`
	Severity string `mapstructure:"severity"`
}

type SOERuleOptions struct {
	Enabled     bool     `mapstructure:"enabled"`
	Severity    string   `mapstructure:"severity"`
	MetricNames []string `mapstructure:"metric-names"`
}

type TracingOptions struct {
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
}

type DatabaseOptions struct {
	Driver   string            `mapstructure:"driver"`
	Host     string            `mapstructure:"host"`
	Port     int               `mapstructure:"port"`
	User     string            `mapstructure:"user"`
	Password string            `mapstructure:"password"`
	DBName   string            `mapstructure:"dbname"`
	Params   map[string]string `mapstructure:"params"`
	DSN      string            `mapstructure:"dsn"`

	MaxOpenConns           int `mapstructure:"max-open-conns"`
	MaxIdleConns           int `mapstructure:"max-idle-conns"`
	ConnMaxLifetimeSeconds int `mapstructure:"conn-max-lifetime-seconds"`
	ConnMaxIdleTimeSeconds int `mapstructure:"conn-max-idle-time-seconds"`
}

// KafkaOptions: brokers empty → consumers are no-op.
type KafkaOptions struct {
	Brokers         []string `mapstructure:"brokers"`
	DispatchTopic   string   `mapstructure:"dispatch-topic"`
	SOETopic        string   `mapstructure:"soe-topic"`
	DispatchGroupID string   `mapstructure:"dispatch-group-id"`
	SOEGroupID      string   `mapstructure:"soe-group-id"`
}

func NewOptions() *Options {
	return &Options{
		Alarm: AlarmOptions{
			HTTPAddr:    ":8087",
			MetricsAddr: ":9107",
			ServiceName: "alarm",
			Rules: RulesOptions{
				DispatchTaskFailed: RuleOptions{Enabled: true, Severity: "critical"},
				SOEDiscreteChange:  SOERuleOptions{Enabled: true, Severity: "warning"},
			},
		},
		Database: DatabaseOptions{
			Driver: "postgres",
			Host:   "127.0.0.1",
			Port:   5432,
			User:   "postgres",
			DBName: "alarm",
			Params: map[string]string{
				"sslmode":  "disable",
				"TimeZone": "Asia/Shanghai",
			},
			MaxOpenConns:           20,
			MaxIdleConns:           5,
			ConnMaxLifetimeSeconds: 1800,
			ConnMaxIdleTimeSeconds: 300,
		},
		Kafka: KafkaOptions{
			DispatchTopic:   "vpp.dispatch.events",
			SOETopic:        "vpp.soe.events",
			DispatchGroupID: "vpp-alarm-dispatch-events",
			SOEGroupID:      "vpp-alarm-soe-events",
		},
	}
}

func (o *Options) Validate() []error {
	var errs []error
	if o.Alarm.HTTPAddr == "" {
		errs = append(errs, fmt.Errorf("alarm.http-addr must not be empty"))
	}
	if o.Alarm.ServiceName == "" {
		errs = append(errs, fmt.Errorf("alarm.service-name must not be empty"))
	}
	if o.Database.Driver == "" {
		errs = append(errs, fmt.Errorf("database.driver must not be empty"))
	}
	if o.Database.DSN == "" {
		if o.Database.Host == "" {
			errs = append(errs, fmt.Errorf("database.host must not be empty (or set database.dsn)"))
		}
		if o.Database.User == "" {
			errs = append(errs, fmt.Errorf("database.user must not be empty (or set database.dsn)"))
		}
		if o.Database.DBName == "" {
			errs = append(errs, fmt.Errorf("database.dbname must not be empty (or set database.dsn)"))
		}
	}
	return errs
}
