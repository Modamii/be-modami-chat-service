package main

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"be-modami-chat-service/pkg/utils"
	"be-modami-chat-service/internal/gateway/centrifugo"
	kafkagw "be-modami-chat-service/internal/gateway/kafka"
	"be-modami-chat-service/configs"
)

type Connections struct {
	MongoClient         *mongo.Client
	MongoDB             *mongo.Database
	RedisClient         *redis.Client
	KafkaProducer       *kafkagw.Producer
	CentrifugoPublisher *centrifugo.Publisher
	IDGen               *utils.ObjectIDGenerator
}

func NewConnections(ctx context.Context, cfg *config.Config) *Connections {
	// MongoDB
	mongoClient, err := mongo.Connect(options.Client().
		ApplyURI(cfg.MongoDB.URI).
		SetMaxPoolSize(uint64(cfg.MongoDB.MaxPool)))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to mongodb")
	}

	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatal().Err(err).Msg("failed to ping mongodb")
	}
	log.Info().Msg("connected to mongodb")

	db := mongoClient.Database(cfg.MongoDB.Database)

	// Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	log.Info().Msg("connected to redis")

	// Kafka producer
	idGen := &utils.ObjectIDGenerator{}
	kafkaProducer, err := kafkagw.NewProducer(cfg.Kafka.Brokers, idGen.NewID)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create kafka producer")
	}
	log.Info().Msg("kafka producer created")

	// Centrifugo
	centrifugoClient := centrifugo.NewClient(cfg.Centrifugo.APIURL, cfg.Centrifugo.APIKey)
	centrifugoPublisher := centrifugo.NewPublisher(centrifugoClient)

	return &Connections{
		MongoClient:         mongoClient,
		MongoDB:             db,
		RedisClient:         redisClient,
		KafkaProducer:       kafkaProducer,
		CentrifugoPublisher: centrifugoPublisher,
		IDGen:               idGen,
	}
}

func (c *Connections) Close() {
	if err := c.MongoClient.Disconnect(context.Background()); err != nil {
		log.Error().Err(err).Msg("mongodb disconnect error")
	}
	c.RedisClient.Close()
	c.KafkaProducer.Close()
}
