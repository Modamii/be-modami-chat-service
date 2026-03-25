package kafka

import (
	"context"
	"fmt"

	"be-modami-chat-service/internal/domain"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Topic constants for the chat service.
const (
	TopicMessagesInbound = "chat.messages.inbound"
	TopicMessagesSent    = "chat.messages.sent"
	TopicMessagesUpdated = "chat.messages.updated"
	TopicMessagesDeleted = "chat.messages.deleted"
	TopicReadReceipts    = "chat.read_receipts"
	TopicTyping          = "chat.typing"
	TopicReactions       = "chat.reactions"
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
	_, encoded, err := newEnvelope(eventType, payload, p.idFunc)
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
	return p.publish(ctx, TopicMessagesSent, msg.ConversationID, "message.sent", msg)
}

func (p *Producer) PublishMessageUpdated(ctx context.Context, msg *domain.Message) error {
	return p.publish(ctx, TopicMessagesUpdated, msg.ConversationID, "message.updated", msg)
}

func (p *Producer) PublishMessageDeleted(ctx context.Context, conversationID, messageID, senderID string) error {
	payload := map[string]string{
		"conversation_id": conversationID,
		"message_id":      messageID,
		"sender_id":       senderID,
	}
	return p.publish(ctx, TopicMessagesDeleted, conversationID, "message.deleted", payload)
}

func (p *Producer) PublishReadReceipt(ctx context.Context, conversationID, userID, messageID string) error {
	payload := map[string]string{
		"conversation_id": conversationID,
		"user_id":         userID,
		"message_id":      messageID,
	}
	return p.publish(ctx, TopicReadReceipts, conversationID, "read_receipt", payload)
}

func (p *Producer) PublishTypingIndicator(ctx context.Context, conversationID, userID string, typing bool) error {
	payload := map[string]interface{}{
		"conversation_id": conversationID,
		"user_id":         userID,
		"typing":          typing,
	}
	return p.publish(ctx, TopicTyping, conversationID, "typing", payload)
}

func (p *Producer) PublishReactionAdded(ctx context.Context, conversationID, messageID, emoji, userID string) error {
	payload := map[string]string{
		"conversation_id": conversationID,
		"message_id":      messageID,
		"emoji":           emoji,
		"user_id":         userID,
	}
	return p.publish(ctx, TopicReactions, conversationID, "reaction.added", payload)
}

func (p *Producer) PublishReactionRemoved(ctx context.Context, conversationID, messageID, emoji, userID string) error {
	payload := map[string]string{
		"conversation_id": conversationID,
		"message_id":      messageID,
		"emoji":           emoji,
		"user_id":         userID,
	}
	return p.publish(ctx, TopicReactions, conversationID, "reaction.removed", payload)
}
