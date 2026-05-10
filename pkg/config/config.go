package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Redis         RedisConfig
	JWT           JWTConfig
	GRPC          GRPCConfig
	Transport     TransportConfig
	Observability ObservabilityConfig
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	MaxConns int    `mapstructure:"max_conns"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret          string        `mapstructure:"secret"`
	AccessTokenTTL  time.Duration `mapstructure:"access_token_ttl"`
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`
}

type GRPCConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	CAFile   string `mapstructure:"ca_file"`
}

type TransportConfig struct {
	XrayBin        string `mapstructure:"xray_bin"`
	Hysteria2Bin   string `mapstructure:"hysteria2_bin"`
	WireGuardBin   string `mapstructure:"wireguard_bin"`
}

type ObservabilityConfig struct {
	PrometheusNamespace string `mapstructure:"prometheus_namespace"`
	LogLevel            string `mapstructure:"log_level"`
}

// AgentConfig holds node-agent specific settings.
type AgentConfig struct {
	NodeID               string        `mapstructure:"node_id"`
	ControlPlaneAddr     string        `mapstructure:"control_plane_addr"`
	HeartbeatInterval    time.Duration `mapstructure:"heartbeat_interval"`
	ConfigPollInterval   time.Duration `mapstructure:"config_poll_interval"`
	CertFile             string        `mapstructure:"cert_file"`
	KeyFile              string        `mapstructure:"key_file"`
	CAFile               string        `mapstructure:"ca_file"`
}

func Load(configName string) (*Config, error) {
	viper.SetConfigName(configName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("/etc/vpnplatform")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadAgent(configName string) (*AgentConfig, error) {
	viper.SetConfigName(configName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("/etc/vpnplatform")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	viper.SetDefault("agent.heartbeat_interval", "30s")
	viper.SetDefault("agent.config_poll_interval", "60s")
	viper.SetDefault("agent.control_plane_addr", "localhost:9090")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg AgentConfig
	if err := viper.UnmarshalKey("agent", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "release")

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_conns", 25)

	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)

	viper.SetDefault("jwt.access_token_ttl", "15m")
	viper.SetDefault("jwt.refresh_token_ttl", "168h")

	viper.SetDefault("grpc.host", "0.0.0.0")
	viper.SetDefault("grpc.port", 9090)

	viper.SetDefault("observability.log_level", "info")
	viper.SetDefault("observability.prometheus_namespace", "vpnplatform")
}
