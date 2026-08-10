package options

import "fmt"

// Options holds all configurable parameters for the gateway service.
type Options struct {
	Gateway       GatewayOptions       `mapstructure:"gateway"`
	Tracing       TracingOptions       `mapstructure:"tracing"`
	Database      DatabaseOptions      `mapstructure:"database"`
	TelemetryGRPC TelemetryGRPCOptions `mapstructure:"telemetry-grpc"`
	Simulator     SimulatorOptions     `mapstructure:"simulator"`
	Kafka         KafkaOptions         `mapstructure:"kafka"`
}

type GatewayOptions struct {
	GRPCAddr    string `mapstructure:"grpc-addr"`
	HTTPAddr    string `mapstructure:"http-addr"`
	MetricsAddr string `mapstructure:"metrics-addr"`
	ServiceName string `mapstructure:"service-name"`
	ConsulAddr  string `mapstructure:"consul-addr"`

	// Auth configures HTTP identity middleware for mappings (C10b).
	Auth AuthOptions `mapstructure:"auth"`
}

// AuthOptions configures Gateway HTTP identity middleware (APISIX X-Userinfo / Casdoor).
type AuthOptions struct {
	// TrustProxyHeaders requires valid X-Userinfo when true (mappings via APISIX OIDC).
	// When false, auth is bypassed for local direct :8083 debugging.
	TrustProxyHeaders bool `mapstructure:"trust-proxy-headers"`
	// Authz configures local Casbin PDP + Casdoor policy sync (C10b).
	Authz AuthzOptions `mapstructure:"authz"`
}

// AuthzOptions is the gateway-service wiring for platform/authz.
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

type TelemetryGRPCOptions struct {
	Addr string `mapstructure:"addr"`
}

// SimulatorOptions configures the outbound HTTP client to vpp-simulator.
// Addr empty → commands for external_system=simulator fall through to ems_log.
type SimulatorOptions struct {
	Addr string `mapstructure:"addr"` // e.g. http://127.0.0.1:8084
}

// KafkaOptions configures the resource event consumer and command event producer.
// Brokers empty → consumer/publisher degrade to no-op.
type KafkaOptions struct {
	Brokers      []string `mapstructure:"brokers"`
	Topic        string   `mapstructure:"topic"`         // resource lifecycle consume; default: vpp.resource.events
	GroupID      string   `mapstructure:"group-id"`      // default: vpp-gateway-resource-events
	CommandTopic string   `mapstructure:"command-topic"` // command completed produce; default: vpp.command.events
}

func NewOptions() *Options {
	return &Options{
		Gateway: GatewayOptions{
			GRPCAddr:    ":5005",
			HTTPAddr:    ":8083",
			MetricsAddr: ":9104",
			ServiceName: "vpp-gateway",
		},
		Database: DatabaseOptions{
			Driver: "postgres",
			Host:   "127.0.0.1",
			Port:   5432,
			User:   "postgres",
			DBName: "gateway",
			Params: map[string]string{
				"sslmode":  "disable",
				"TimeZone": "Asia/Shanghai",
			},
			MaxOpenConns:           20,
			MaxIdleConns:           5,
			ConnMaxLifetimeSeconds: 1800,
			ConnMaxIdleTimeSeconds: 300,
		},
		TelemetryGRPC: TelemetryGRPCOptions{
			Addr: "127.0.0.1:5003",
		},
		Kafka: KafkaOptions{
			Topic:        "vpp.resource.events",
			GroupID:      "vpp-gateway-resource-events",
			CommandTopic: "vpp.command.events",
		},
	}
}

func (o *Options) Validate() []error {
	var errs []error
	if o.Gateway.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("gateway.grpc-addr must not be empty"))
	}
	if o.Gateway.HTTPAddr == "" {
		errs = append(errs, fmt.Errorf("gateway.http-addr must not be empty"))
	}
	if o.Gateway.ServiceName == "" {
		errs = append(errs, fmt.Errorf("gateway.service-name must not be empty"))
	}
	if o.TelemetryGRPC.Addr == "" {
		errs = append(errs, fmt.Errorf("telemetry-grpc.addr must not be empty"))
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
