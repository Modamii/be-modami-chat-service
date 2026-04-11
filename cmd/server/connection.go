package main

import (
	"context"
	"os"

	"github.com/gocql/gocql"
	"github.com/redis/go-redis/v9"

	config "be-modami-chat-service/config"
	chatmessaging "be-modami-chat-service/internal/adapter/messaging"
	repository "be-modami-chat-service/internal/adapter/repository"
	"be-modami-chat-service/pkg/centrifugo"
	pkgkafka "be-modami-chat-service/pkg/kafka"
	"be-modami-chat-service/pkg/utils"

	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
)

// Connections holds all live infrastructure clients.
type Connections struct {
	ScyllaSession       *gocql.Session
	RedisClient         *redis.Client
	KafkaService        *pkgkafka.KafkaService
	KafkaProducer       *chatmessaging.Producer
	CentrifugoPublisher *centrifugo.Publisher
	IDGen               *utils.ObjectIDGenerator
}

// NewConnections initialises every infrastructure dependency. Fatal on failure.
func NewConnections(ctx context.Context, cfg *config.Config) *Connections {
	// ── ScyllaDB ────────────────────────────────────────────────────────────

	// Phase 1: keyspace-less session used only to run schema DDL.
	initCluster := gocql.NewCluster(cfg.ScyllaDB.Hosts...)
	initCluster.Consistency = gocql.LocalQuorum
	initCluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
		gocql.DCAwareRoundRobinPolicy(cfg.ScyllaDB.Datacenter),
	)
	initSession, err := initCluster.CreateSession()
	if err != nil {
		logger.Error(ctx, "failed to connect to scylladb (init session)", err)
		os.Exit(1)
	}
	if err := repository.EnsureSchema(initSession, cfg.ScyllaDB.Datacenter, cfg.ScyllaDB.ReplicationFactor); err != nil {
		logger.Error(ctx, "failed to ensure scylladb schema", err)
		os.Exit(1)
	}
	initSession.Close()
	logger.Info(ctx, "scylladb schema ready")

	// Phase 2: production session bound to the chat keyspace.
	cluster := gocql.NewCluster(cfg.ScyllaDB.Hosts...)
	cluster.Keyspace = cfg.ScyllaDB.Keyspace
	cluster.Consistency = gocql.LocalQuorum
	cluster.NumConns = cfg.ScyllaDB.NumConns
	cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
		gocql.DCAwareRoundRobinPolicy(cfg.ScyllaDB.Datacenter),
	)
	session, err := cluster.CreateSession()
	if err != nil {
		logger.Error(ctx, "failed to connect to scylladb", err)
		os.Exit(1)
	}
	logger.Info(ctx, "connected to scylladb", logging.Any("hosts", cfg.ScyllaDB.Hosts))

	// ── Redis ────────────────────────────────────────────────────────────────
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr(),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.Database,
		PoolSize:     cfg.Redis.PoolSize,
		WriteTimeout: cfg.Redis.WriteTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		DialTimeout:  cfg.Redis.DialTimeout,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error(ctx, "failed to connect to redis", err)
		os.Exit(1)
	}
	logger.Info(ctx, "connected to redis")

	// ── Kafka ────────────────────────────────────────────────────────────────
	kafkaService, err := pkgkafka.NewKafkaService(&pkgkafka.KafkaConfig{
		Brokers:         cfg.Kafka.Brokers(),
		ClientID:        cfg.Kafka.ClientID,
		ConsumerGroupID: cfg.Kafka.ConsumerGroup,
	}, cfg.Kafka.Environment)
	if err != nil {
		logger.Error(ctx, "failed to create kafka service", err)
		os.Exit(1)
	}
	logger.Info(ctx, "kafka service created")

	kafkaProducer := chatmessaging.NewProducer(kafkaService)

	// ── Centrifugo ───────────────────────────────────────────────────────────
	centrifugoClient := centrifugo.NewClient(cfg.Centrifugo.APIURL, cfg.Centrifugo.APIKey)
	centrifugoPublisher := centrifugo.NewPublisher(centrifugoClient)

	idGen := &utils.ObjectIDGenerator{}

	return &Connections{
		ScyllaSession:       session,
		RedisClient:         redisClient,
		KafkaService:        kafkaService,
		KafkaProducer:       kafkaProducer,
		CentrifugoPublisher: centrifugoPublisher,
		IDGen:               idGen,
	}
}

// Close gracefully releases all infrastructure connections.
func (c *Connections) Close() {
	c.ScyllaSession.Close()
	c.RedisClient.Close()
	c.KafkaService.Close()
}
