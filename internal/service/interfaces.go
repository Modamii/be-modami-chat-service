package service

import (
	"context"
	"time"

	"be-modami-chat-service/internal/domain"
)

// MessageRepository defines persistence operations for messages.
type MessageRepository interface {
	Create(ctx context.Context, msg *domain.Message) error
	FindByID(ctx context.Context, id string) (*domain.Message, error)
	FindByConversation(ctx context.Context, conversationID string, page domain.PageRequest) (*domain.PageResponse[*domain.Message], error)
	Update(ctx context.Context, msg *domain.Message) error
	AddReaction(ctx context.Context, messageID, emoji, userID string) error
	RemoveReaction(ctx context.Context, messageID, emoji, userID string) error
}

// ConversationRepository defines persistence operations for conversations.
type ConversationRepository interface {
	Create(ctx context.Context, conv *domain.Conversation) error
	FindByID(ctx context.Context, id string) (*domain.Conversation, error)
	FindByUser(ctx context.Context, userID string, page domain.PageRequest) (*domain.PageResponse[*domain.Conversation], error)
	FindDirectChat(ctx context.Context, userID1, userID2 string) (*domain.Conversation, error)
	Update(ctx context.Context, conv *domain.Conversation) error
	UpdateLastMessage(ctx context.Context, conversationID string, lastMsg *domain.LastMessage) error
	UpdateReadCursor(ctx context.Context, conversationID, userID, messageID string, readAt time.Time) error
	AddParticipant(ctx context.Context, conversationID string, participant domain.Participant) error
	RemoveParticipant(ctx context.Context, conversationID, userID string) error
}

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

// CacheStore manages ephemeral cached data.
type CacheStore interface {
	IncrementUnread(ctx context.Context, userID, conversationID string) error
	ClearUnread(ctx context.Context, userID, conversationID string) error
	GetUnreadCounts(ctx context.Context, userID string) (map[string]int64, error)
}

// PresenceStore manages user presence and typing indicators.
type PresenceStore interface {
	SetOnline(ctx context.Context, userID string) error
	SetOffline(ctx context.Context, userID string) error
	IsOnline(ctx context.Context, userID string) (bool, error)
	GetOnlineUsers(ctx context.Context, userIDs []string) ([]string, error)
	SetTyping(ctx context.Context, conversationID, userID string) error
	ClearTyping(ctx context.Context, conversationID, userID string) error
}

// RateLimiter checks and enforces rate limits.
type RateLimiter interface {
	AllowMessage(ctx context.Context, userID string) (bool, error)
}

// IDGenerator generates unique IDs.
type IDGenerator interface {
	NewID() string
}
