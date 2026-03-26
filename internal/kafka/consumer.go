package kafka

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/twmb/franz-go/pkg/kgo"
)

// MessageHandler processes a single Kafka record.
type MessageHandler func(ctx context.Context, record *kgo.Record) error

// Consumer is a Kafka consumer for chat service topics.
type Consumer struct {
	client  *kgo.Client
	handler MessageHandler
}

// NewConsumer creates a new Kafka consumer for the given topics.
func NewConsumer(brokers []string, groupID string, topics []string, handler MessageHandler) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topics...),
	)
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer: %w", err)
	}
	return &Consumer{client: client, handler: handler}, nil
}

// Start begins consuming messages. Blocks until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if err := ctx.Err(); err != nil {
			return err
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()
			if err := c.handler(ctx, record); err != nil {
				log.Error().Err(err).
					Str("topic", record.Topic).
					Msg("consumer handler error")
			}
		}
	}
}

// Close shuts down the consumer.
func (c *Consumer) Close() {
	c.client.Close()
}
