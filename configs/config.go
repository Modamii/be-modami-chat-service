package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the chat service.
type Config struct {
	App        AppConfig        `mapstructure:"app"`
	Server     ServerConfig     `mapstructure:"server"`
	MongoDB    MongoDBConfig    `mapstructure:"mongodb"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Kafka      KafkaConfig      `mapstructure:"kafka"`
	Centrifugo CentrifugoConfig `mapstructure:"centrifugo"`
	JWT        JWTConfig        `mapstructure:"jwt"`
	CORS       CORSConfig       `mapstructure:"cors"`
	Log        LogConfig        `mapstructure:"log"`
}

type AppConfig struct {
	Name        string `mapstructure:"name"`
	Environment string `mapstructure:"environment"`
}

type ServerConfig struct {
	Port         int `mapstructure:"port"`
	Host         string `mapstructure:"host"`
	ReadTimeout  int `mapstructure:"read_timeout"`
	WriteTimeout int `mapstructure:"write_timeout"`
}

type MongoDBConfig struct {
	URI      string `mapstructure:"uri"`
	Database string `mapstructure:"database"`
	MaxPool  int    `mapstructure:"max_pool"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type KafkaConfig struct {
	Brokers       []string `mapstructure:"brokers"`
	ConsumerGroup string   `mapstructure:"consumer_group"`
}

type CentrifugoConfig struct {
	APIURL string `mapstructure:"api_url"`
	APIKey string `mapstructure:"api_key"`
}

type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	Expiration int    `mapstructure:"expiration"` // minutes
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	MaxAge         int      `mapstructure:"max_age"` // seconds
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Pretty bool   `mapstructure:"pretty"`
}

// TokenExpiration returns the JWT expiration as a time.Duration.
func (c *JWTConfig) TokenExpiration() time.Duration {
	return time.Duration(c.Expiration) * time.Minute
}

// setDefaults registers default values so the service can start with minimal config.
func setDefaults() {
	// App
	viper.SetDefault("app.name", "chat-service")
	viper.SetDefault("app.environment", "local")

	// Server
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.read_timeout", 15)
	viper.SetDefault("server.write_timeout", 15)

	// MongoDB
	viper.SetDefault("mongodb.uri", "mongodb://localhost:27017")
	viper.SetDefault("mongodb.database", "chat")
	viper.SetDefault("mongodb.max_pool", 100)

	// Redis
	viper.SetDefault("redis.addr", "localhost:6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	// Kafka
	viper.SetDefault("kafka.brokers", []string{"localhost:9092"})
	viper.SetDefault("kafka.consumer_group", "chat-message-processor")

	// Centrifugo
	viper.SetDefault("centrifugo.api_url", "http://localhost:8000/api")
	viper.SetDefault("centrifugo.api_key", "")

	// JWT
	viper.SetDefault("jwt.secret", "")
	viper.SetDefault("jwt.expiration", 15)

	// CORS
	viper.SetDefault("cors.allowed_origins", []string{"*"})
	viper.SetDefault("cors.max_age", 300)

	// Log
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.pretty", false)
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
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// validate checks that required fields are set and values are sane.
func (c *Config) validate() error {
	if c.MongoDB.URI == "" {
		return fmt.Errorf("mongodb.uri is required")
	}
	if c.MongoDB.Database == "" {
		return fmt.Errorf("mongodb.database is required")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka.brokers is required")
	}
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if c.JWT.Expiration <= 0 {
		return fmt.Errorf("jwt.expiration must be positive")
	}
	return nil
}
