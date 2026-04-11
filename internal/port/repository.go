package port

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

// IDGenerator generates unique IDs.
type IDGenerator interface {
	NewID() string
}
