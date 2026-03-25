package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"be-modami-chat-service/internal/domain"

	"github.com/rs/zerolog/log"
)

// ChatService orchestrates chat business logic.
type ChatService struct {
	msgRepo     MessageRepository
	convRepo    ConversationRepository
	publisher   EventPublisher
	realtime    RealtimePublisher
	cache       CacheStore
	presence    PresenceStore
	rateLimiter RateLimiter
	idGen       IDGenerator
}

// NewChatService creates a ChatService with injected dependencies.
func NewChatService(
	msgRepo MessageRepository,
	convRepo ConversationRepository,
	publisher EventPublisher,
	realtime RealtimePublisher,
	cache CacheStore,
	presence PresenceStore,
	rateLimiter RateLimiter,
	idGen IDGenerator,
) *ChatService {
	return &ChatService{
		msgRepo:     msgRepo,
		convRepo:    convRepo,
		publisher:   publisher,
		realtime:    realtime,
		cache:       cache,
		presence:    presence,
		rateLimiter: rateLimiter,
		idGen:       idGen,
	}
}

// GetMessages returns paginated messages for a conversation.
func (s *ChatService) GetMessages(ctx context.Context, conversationID, userID string, page domain.PageRequest) (*domain.PageResponse[*domain.Message], error) {
	conv, err := s.convRepo.FindByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("find conversation: %w", err)
	}
	if !conv.HasMember(userID) {
		return nil, domain.ErrNotChatMember
	}

	page.Limit = domain.NormalizeLimit(page.Limit)
	return s.msgRepo.FindByConversation(ctx, conversationID, page)
}

// GetConversations returns a user's conversation list sorted by last activity.
func (s *ChatService) GetConversations(ctx context.Context, userID string, page domain.PageRequest) (*domain.PageResponse[*domain.Conversation], error) {
	page.Limit = domain.NormalizeLimit(page.Limit)
	return s.convRepo.FindByUser(ctx, userID, page)
}

// GetConversation returns a single conversation if the user is a member.
func (s *ChatService) GetConversation(ctx context.Context, conversationID, userID string) (*domain.Conversation, error) {
	conv, err := s.convRepo.FindByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("find conversation: %w", err)
	}
	if !conv.HasMember(userID) {
		return nil, domain.ErrNotChatMember
	}
	return conv, nil
}

// MarkAsRead updates the read cursor for a user in a conversation.
func (s *ChatService) MarkAsRead(ctx context.Context, conversationID, userID, messageID string) error {
	conv, err := s.convRepo.FindByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("find conversation: %w", err)
	}
	if !conv.HasMember(userID) {
		return domain.ErrNotChatMember
	}

	// Verify the message exists in this conversation
	msg, err := s.msgRepo.FindByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("find message: %w", err)
	}
	if msg.ConversationID != conversationID {
		return domain.ErrNotFound
	}

	now := time.Now()
	if err := s.convRepo.UpdateReadCursor(ctx, conversationID, userID, messageID, now); err != nil {
		return fmt.Errorf("update read cursor: %w", err)
	}

	// Clear unread count in cache
	if err := s.cache.ClearUnread(ctx, userID, conversationID); err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("failed to clear unread cache")
	}

	// Publish read receipt event
	if err := s.publisher.PublishReadReceipt(ctx, conversationID, userID, messageID); err != nil {
		log.Error().Err(err).Msg("failed to publish read receipt")
	}

	// Push read receipt to conversation members via Centrifugo
	if err := s.realtime.PublishToConversation(ctx, conversationID, "read_receipt", map[string]string{
		"user_id":    userID,
		"message_id": messageID,
	}); err != nil {
		log.Error().Err(err).Msg("failed to publish realtime read receipt")
	}

	return nil
}

// EditMessage updates a message's content.
func (s *ChatService) EditMessage(ctx context.Context, messageID, userID, newText string) (*domain.Message, error) {
	msg, err := s.msgRepo.FindByID(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("find message: %w", err)
	}
	if msg.SenderID != userID {
		return nil, domain.ErrForbidden
	}
	if msg.Deleted {
		return nil, domain.ErrMessageDeleted
	}
	if msg.Type != domain.MessageTypeText {
		return nil, domain.ErrInvalidInput("only text messages can be edited")
	}
	if len(newText) > domain.MaxMessageContentLength {
		return nil, domain.ErrInvalidInput("message content exceeds maximum length")
	}

	msg.Content.Text = newText
	msg.Edited = true

	if err := s.msgRepo.Update(ctx, msg); err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}

	if err := s.publisher.PublishMessageUpdated(ctx, msg); err != nil {
		log.Error().Err(err).Str("msg_id", msg.ID).Msg("failed to publish message update")
	}

	return msg, nil
}

// DeleteMessage soft-deletes a message.
func (s *ChatService) DeleteMessage(ctx context.Context, messageID, userID string) error {
	msg, err := s.msgRepo.FindByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("find message: %w", err)
	}
	if msg.SenderID != userID {
		return domain.ErrForbidden
	}

	msg.Deleted = true
	msg.Content = domain.MessageContent{}
	msg.Media = nil

	if err := s.msgRepo.Update(ctx, msg); err != nil {
		return fmt.Errorf("delete message: %w", err)
	}

	if err := s.publisher.PublishMessageDeleted(ctx, msg.ConversationID, msg.ID, userID); err != nil {
		log.Error().Err(err).Str("msg_id", msg.ID).Msg("failed to publish message deletion")
	}

	return nil
}

// AddReaction adds a reaction to a message.
func (s *ChatService) AddReaction(ctx context.Context, messageID, userID, emoji string) error {
	msg, err := s.msgRepo.FindByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("find message: %w", err)
	}

	// Verify user is a member of the conversation
	conv, err := s.convRepo.FindByID(ctx, msg.ConversationID)
	if err != nil {
		return fmt.Errorf("find conversation: %w", err)
	}
	if !conv.HasMember(userID) {
		return domain.ErrNotChatMember
	}

	if err := s.msgRepo.AddReaction(ctx, messageID, emoji, userID); err != nil {
		if errors.Is(err, domain.ErrReactionLimitReached) {
			return err
		}
		return fmt.Errorf("add reaction: %w", err)
	}

	if err := s.publisher.PublishReactionAdded(ctx, msg.ConversationID, messageID, emoji, userID); err != nil {
		log.Error().Err(err).Msg("failed to publish reaction event")
	}

	return nil
}

// RemoveReaction removes a reaction from a message.
func (s *ChatService) RemoveReaction(ctx context.Context, messageID, userID, emoji string) error {
	msg, err := s.msgRepo.FindByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("find message: %w", err)
	}

	if err := s.msgRepo.RemoveReaction(ctx, messageID, emoji, userID); err != nil {
		return fmt.Errorf("remove reaction: %w", err)
	}

	if err := s.publisher.PublishReactionRemoved(ctx, msg.ConversationID, messageID, emoji, userID); err != nil {
		log.Error().Err(err).Msg("failed to publish reaction removal")
	}

	return nil
}

// HandleTyping processes a typing indicator.
func (s *ChatService) HandleTyping(ctx context.Context, conversationID, userID string, typing bool) error {
	conv, err := s.convRepo.FindByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("find conversation: %w", err)
	}
	if !conv.HasMember(userID) {
		return domain.ErrNotChatMember
	}

	if typing {
		if err := s.presence.SetTyping(ctx, conversationID, userID); err != nil {
			return fmt.Errorf("set typing: %w", err)
		}
	} else {
		if err := s.presence.ClearTyping(ctx, conversationID, userID); err != nil {
			return fmt.Errorf("clear typing: %w", err)
		}
	}

	// Publish typing indicator via Centrifugo (ephemeral, no persistence)
	return s.realtime.PublishToConversation(ctx, conversationID, "typing", map[string]interface{}{
		"user_id": userID,
		"typing":  typing,
	})
}

// GetUnreadCounts returns unread message counts for all conversations.
func (s *ChatService) GetUnreadCounts(ctx context.Context, userID string) (map[string]int64, error) {
	return s.cache.GetUnreadCounts(ctx, userID)
}
