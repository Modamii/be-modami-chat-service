package port

import (
	"context"

	"be-modami-chat-service/internal/domain"
)

// EventPublisher publishes domain events to the message broker.
type EventPublisher interface {
	PublishMessageSent(ctx context.Context, msg *domain.Message) error
	PublishMessageUpdated(ctx context.Context, msg *domain.Message) error
	PublishMessageDeleted(ctx context.Context, conversationID, messageID, senderID string) error
	PublishReadReceipt(ctx context.Context, conversationID, userID, messageID string) error
	PublishReactionAdded(ctx context.Context, conversationID, messageID, emoji, userID string) error
	PublishReactionRemoved(ctx context.Context, conversationID, messageID, emoji, userID string) error
}

// RealtimePublisher publishes events to connected clients via Centrifugo.
type RealtimePublisher interface {
	PublishToConversation(ctx context.Context, conversationID string, eventType string, data interface{}) error
	PublishToUser(ctx context.Context, userID string, eventType string, data interface{}) error
}
