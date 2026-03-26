package kafka

import (
	"context"
	"fmt"

	"be-modami-chat-service/internal/domain"
	"be-modami-chat-service/pkg/kafka/events"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Producer implements service.EventPublisher using Kafka via franz-go.
type Producer struct {
	client *kgo.Client
	idFunc func() string
}

// NewProducer creates a new Kafka producer.
func NewProducer(brokers []string, idFunc func() string) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create kafka client: %w", err)
	}
	return &Producer{client: client, idFunc: idFunc}, nil
}

// Close shuts down the producer.
func (p *Producer) Close() {
	p.client.Close()
}

func (p *Producer) publish(ctx context.Context, topic string, key string, eventType string, payload interface{}) error {
	_, encoded, err := events.NewEnvelope(eventType, payload, p.idFunc)
	if err != nil {
		return err
	}

	record := &kgo.Record{
		Topic: topic,
		Key:   []byte(key),
		Value: encoded,
	}

	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", topic, err)
	}
	return nil
}

func (p *Producer) PublishMessageSent(ctx context.Context, msg *domain.Message) error {
	event := events.MessageSentEvent{
		ConversationID: msg.ConversationID,
		MessageID:      msg.ID,
		SenderID:       msg.SenderID,
		Type:           string(msg.Type),
		Text:           msg.Content.Text,
	}
	return p.publish(ctx, events.TopicMessagesSent, msg.ConversationID, "message.sent", event)
}

func (p *Producer) PublishMessageUpdated(ctx context.Context, msg *domain.Message) error {
	event := events.MessageUpdatedEvent{
		ConversationID: msg.ConversationID,
		MessageID:      msg.ID,
		SenderID:       msg.SenderID,
		Text:           msg.Content.Text,
	}
	return p.publish(ctx, events.TopicMessagesUpdated, msg.ConversationID, "message.updated", event)
}

func (p *Producer) PublishMessageDeleted(ctx context.Context, conversationID, messageID, senderID string) error {
	event := events.MessageDeletedEvent{
		ConversationID: conversationID,
		MessageID:      messageID,
		SenderID:       senderID,
	}
	return p.publish(ctx, events.TopicMessagesDeleted, conversationID, "message.deleted", event)
}

func (p *Producer) PublishReadReceipt(ctx context.Context, conversationID, userID, messageID string) error {
	event := events.ReadReceiptEvent{
		ConversationID: conversationID,
		UserID:         userID,
		MessageID:      messageID,
	}
	return p.publish(ctx, events.TopicReadReceipts, conversationID, "read_receipt", event)
}

func (p *Producer) PublishReactionAdded(ctx context.Context, conversationID, messageID, emoji, userID string) error {
	event := events.ReactionEvent{
		ConversationID: conversationID,
		MessageID:      messageID,
		Emoji:          emoji,
		UserID:         userID,
	}
	return p.publish(ctx, events.TopicReactions, conversationID, "reaction.added", event)
}

func (p *Producer) PublishReactionRemoved(ctx context.Context, conversationID, messageID, emoji, userID string) error {
	event := events.ReactionEvent{
		ConversationID: conversationID,
		MessageID:      messageID,
		Emoji:          emoji,
		UserID:         userID,
	}
	return p.publish(ctx, events.TopicReactions, conversationID, "reaction.removed", event)
}
