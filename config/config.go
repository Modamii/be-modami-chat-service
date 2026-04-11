package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Config holds all configuration for the chat service.
type Config struct {
	App           AppConfig           `mapstructure:"app"`
	Server        ServerConfig        `mapstructure:"server"`
	ScyllaDB      ScyllaDBConfig      `mapstructure:"scylladb"`
	Redis         RedisConfig         `mapstructure:"redis"`
	Kafka         KafkaConfig         `mapstructure:"kafka"`
	Centrifugo    CentrifugoConfig    `mapstructure:"centrifugo"`
	Keycloak      KeycloakConfig      `mapstructure:"keycloak"`
	CORS          CORSConfig          `mapstructure:"cors"`
	Observability ObservabilityConfig `mapstructure:"observability"`
}

type AppConfig struct {
	Name        string `mapstructure:"name"`
	Version     string `mapstructure:"version"`
	Environment string `mapstructure:"environment"`
}

type ServerConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
}

type ScyllaDBConfig struct {
	Hosts             []string `mapstructure:"hosts"`
	Keyspace          string   `mapstructure:"keyspace"`
	NumConns          int      `mapstructure:"num_conns"`
	Datacenter        string   `mapstructure:"datacenter"`
	ReplicationFactor int      `mapstructure:"replication_factor"`
}

type RedisTLSConfig struct {
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

type RedisConfig struct {
	Host              string         `mapstructure:"host"`
	Port              int            `mapstructure:"port"`
	Database          int            `mapstructure:"database"`
	RateLimitDatabase int            `mapstructure:"rate_limit_database"`
	Password          string         `mapstructure:"pass"`
	UserName          string         `mapstructure:"user_name"`
	PoolSize          int            `mapstructure:"pool_size"`
	TTL               time.Duration  `mapstructure:"ttl"`
	WriteTimeout      time.Duration  `mapstructure:"write_timeout"`
	ReadTimeout       time.Duration  `mapstructure:"read_timeout"`
	DialTimeout       time.Duration  `mapstructure:"dial_timeout"`
	TLSConfig         RedisTLSConfig `mapstructure:"tls_config"`
}

// Addr returns "host:port" for use in redis.Options.
func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

type KafkaConfig struct {
	BrokerList           string `mapstructure:"broker_list"`
	Enable               bool   `mapstructure:"enable"`
	TLSEnable            bool   `mapstructure:"tls_enable"`
	Partition            int    `mapstructure:"partition"`
	Partitioner          string `mapstructure:"partitioner"`
	ConsumerGroup        string `mapstructure:"consumer_group"`
	ClientID             string `mapstructure:"client_id"`
	Environment          string `mapstructure:"env"`
	SASLProducerUsername string `mapstructure:"sasl_producer_username"`
	SASLProducerPassword string `mapstructure:"sasl_producer_password"`
	SASLConsumerUsername string `mapstructure:"sasl_consumer_username"`
	SASLConsumerPassword string `mapstructure:"sasl_consumer_password"`
}

// Brokers returns the broker list as a slice (splits on comma).
func (k *KafkaConfig) Brokers() []string {
	return strings.Split(k.BrokerList, ",")
}

type CentrifugoConfig struct {
	APIURL string `mapstructure:"api_url"`
	APIKey string `mapstructure:"api_key"`
}

type KeycloakConfig struct {
	JWKSUrl string `mapstructure:"jwks_url"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	MaxAge         int      `mapstructure:"max_age"`
}

type ObservabilityConfig struct {
	ServiceName    string `mapstructure:"service_name"`
	ServiceVersion string `mapstructure:"service_version"`
	Environment    string `mapstructure:"environment"`
	LogLevel       string `mapstructure:"log_level"`
	OTLPEndpoint   string `mapstructure:"otlp_endpoint"`
	OTLPInsecure   bool   `mapstructure:"otlp_insecure"`
}

// setDefaults registers sensible defaults so the service can start with minimal config.
func setDefaults() {
	viper.SetDefault("app.name", "chat-service")
	viper.SetDefault("app.version", "1.0.0")
	viper.SetDefault("app.environment", "local")

	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8086)
	viper.SetDefault("server.read_timeout", 15)
	viper.SetDefault("server.write_timeout", 15)

	viper.SetDefault("scylladb.hosts", []string{"localhost"})
	viper.SetDefault("scylladb.keyspace", "chat")
	viper.SetDefault("scylladb.num_conns", 2)
	viper.SetDefault("scylladb.datacenter", "datacenter1")
	viper.SetDefault("scylladb.replication_factor", 3)

	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.database", 0)
	viper.SetDefault("redis.pool_size", 10)

	viper.SetDefault("kafka.broker_list", "localhost:9092")
	viper.SetDefault("kafka.consumer_group", "chat-message-processor")
	viper.SetDefault("kafka.client_id", "chat-service")
	viper.SetDefault("kafka.env", "local")

	viper.SetDefault("centrifugo.api_url", "http://localhost:8000/api")

	viper.SetDefault("cors.allowed_origins", []string{"*"})
	viper.SetDefault("cors.max_age", 300)

	viper.SetDefault("observability.log_level", "info")
	viper.SetDefault("observability.service_name", "chat-service")
	viper.SetDefault("observability.environment", "local")
	viper.SetDefault("observability.otlp_insecure", true)
}

// Load reads configuration from file and environment variables.
// Environment variables override file values with prefix CHAT_.
// Example: CHAT_SERVER_PORT=9090 overrides server.port.
func Load(path string) (*Config, error) {
	setDefaults()

	viper.SetConfigFile(path)
	viper.SetEnvPrefix("CHAT")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	)); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// validate checks that required fields are set and values are sane.
func (c *Config) validate() error {
	if len(c.ScyllaDB.Hosts) == 0 {
		return fmt.Errorf("scylladb.hosts is required")
	}
	if c.ScyllaDB.Keyspace == "" {
		return fmt.Errorf("scylladb.keyspace is required")
	}
	if c.Redis.Host == "" {
		return fmt.Errorf("redis.host is required")
	}
	if c.Kafka.BrokerList == "" {
		return fmt.Errorf("kafka.broker_list is required")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	return nil
}
