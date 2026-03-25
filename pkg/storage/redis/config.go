package redis

import (
	"context"
	"time"

	"gitlab.com/lifegoeson-libs/pkg-logging/logger"

	"github.com/redis/go-redis/v9"
)

type RedisConfig struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func NewRedisClient(config RedisConfig) (*redis.Client, error) {
	if config.PoolSize == 0 {
		config.PoolSize = 10
	}
	if config.MinIdleConns == 0 {
		config.MinIdleConns = 5
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = 5 * time.Second
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 3 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 3 * time.Second
	}
	opts := &redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		MaxRetries:   config.MaxRetries,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	l := logger.FromContext(ctx)
	if err := client.Ping(ctx).Err(); err != nil {
		l.Error("Failed to connect to Redis", err)
		return nil, err
	}
	l.Info("Successfully connected to Redis")
	return client, nil
}

func CloseRedis(ctx context.Context, client *redis.Client) error {
	l := logger.FromContext(ctx)
	if err := client.Close(); err != nil {
		l.Error("Failed to close Redis connection", err)
		return err
	}
	l.Info("Redis connection closed")
	return nil
}
