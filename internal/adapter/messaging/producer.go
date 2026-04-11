package messaging

import (
	"context"

	"be-modami-chat-service/internal/domain"
	pkgkafka "be-modami-chat-service/pkg/kafka"
	"be-modami-chat-service/pkg/kafka/events"
)

// Producer implements port.EventPublisher using the standard KafkaService.
type Producer struct {
	svc *pkgkafka.KafkaService
}

// NewProducer creates a Producer backed by the provided KafkaService.
func NewProducer(svc *pkgkafka.KafkaService) *Producer {
	return &Producer{svc: svc}
}

func (p *Producer) publish(ctx context.Context, topic, key string, payload events.EventPayload) error {
	event := events.NewKafkaEventBase(payload, key, nil)
	event.EventType = payload.GetType()
	return p.svc.EmitToFullTopic(ctx, topic, &pkgkafka.ProducerMessage{
		Key:   key,
		Value: event,
	})
}

func (p *Producer) PublishMessageSent(ctx context.Context, msg *domain.Message) error {
	return p.publish(ctx, events.TopicMessagesSent, msg.ConversationID, &events.MessageSentEvent{
		BaseEventPayload: events.BaseEventPayload{Type: events.EventTypeMessageSent},
		ConversationID:   msg.ConversationID,
		MessageID:        msg.ID,
		SenderID:         msg.SenderID,
		MessageType:      string(msg.Type),
		Text:             msg.Content.Text,
	})
}

func (p *Producer) PublishMessageUpdated(ctx context.Context, msg *domain.Message) error {
	return p.publish(ctx, events.TopicMessagesUpdated, msg.ConversationID, &events.MessageUpdatedEvent{
		BaseEventPayload: events.BaseEventPayload{Type: events.EventTypeMessageUpdated},
		ConversationID:   msg.ConversationID,
		MessageID:        msg.ID,
		SenderID:         msg.SenderID,
		Text:             msg.Content.Text,
	})
}

func (p *Producer) PublishMessageDeleted(ctx context.Context, conversationID, messageID, senderID string) error {
	return p.publish(ctx, events.TopicMessagesDeleted, conversationID, &events.MessageDeletedEvent{
		BaseEventPayload: events.BaseEventPayload{Type: events.EventTypeMessageDeleted},
		ConversationID:   conversationID,
		MessageID:        messageID,
		SenderID:         senderID,
	})
}

func (p *Producer) PublishReadReceipt(ctx context.Context, conversationID, userID, messageID string) error {
	return p.publish(ctx, events.TopicReadReceipts, conversationID, &events.ReadReceiptEvent{
		BaseEventPayload: events.BaseEventPayload{Type: events.EventTypeReadReceipt},
		ConversationID:   conversationID,
		UserID:           userID,
		MessageID:        messageID,
	})
}

func (p *Producer) PublishReactionAdded(ctx context.Context, conversationID, messageID, emoji, userID string) error {
	return p.publish(ctx, events.TopicReactions, conversationID, &events.ReactionEvent{
		BaseEventPayload: events.BaseEventPayload{Type: events.EventTypeReactionAdded},
		Action:           "added",
		ConversationID:   conversationID,
		MessageID:        messageID,
		Emoji:            emoji,
		UserID:           userID,
	})
}

func (p *Producer) PublishReactionRemoved(ctx context.Context, conversationID, messageID, emoji, userID string) error {
	return p.publish(ctx, events.TopicReactions, conversationID, &events.ReactionEvent{
		BaseEventPayload: events.BaseEventPayload{Type: events.EventTypeReactionRemoved},
		Action:           "removed",
		ConversationID:   conversationID,
		MessageID:        messageID,
		Emoji:            emoji,
		UserID:           userID,
	})
}
