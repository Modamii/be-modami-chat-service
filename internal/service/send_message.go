package service

import (
	"context"
	"fmt"

	"be-modami-chat-service/internal/domain"

	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
)

// SendMessageCommand holds input for sending a message.
type SendMessageCommand struct {
	ConversationID string
	SenderID       string
	Type           domain.MessageType
	Content        domain.MessageContent
	Media          *domain.MediaInfo
	ReplyTo        *domain.ReplyInfo
	ForwardedFrom  *domain.ForwardInfo
}

// SendMessage persists a message and publishes events.
func (s *ChatService) SendMessage(ctx context.Context, cmd SendMessageCommand) (*domain.Message, error) {
	// Rate limit check
	allowed, err := s.rateLimiter.AllowMessage(ctx, cmd.SenderID)
	if err != nil {
		return nil, fmt.Errorf("rate limit check: %w", err)
	}
	if !allowed {
		return nil, domain.ErrRateLimited
	}

	// Verify conversation exists and sender is a member
	conv, err := s.convRepo.FindByID(ctx, cmd.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("find conversation: %w", err)
	}
	if !conv.HasMember(cmd.SenderID) {
		return nil, domain.ErrNotChatMember
	}

	// Check if only admins can send
	if conv.Settings.OnlyAdminsCanSend && !conv.IsAdmin(cmd.SenderID) {
		return nil, domain.ErrForbidden
	}

	// Build message
	msg := &domain.Message{
		ID:             s.idGen.NewID(),
		ConversationID: cmd.ConversationID,
		SenderID:       cmd.SenderID,
		Type:           cmd.Type,
		Content:        cmd.Content,
		Media:          cmd.Media,
		ReplyTo:        cmd.ReplyTo,
		ForwardedFrom:  cmd.ForwardedFrom,
		Reactions:      []domain.Reaction{},
	}

	if err := msg.Validate(); err != nil {
		return nil, err
	}

	// Persist before broadcast (critical pattern)
	if err := s.msgRepo.Create(ctx, msg); err != nil {
		return nil, fmt.Errorf("save message: %w", err)
	}

	// Update conversation's last message (denormalized)
	lastMsg := &domain.LastMessage{
		MessageID:      msg.ID,
		SenderID:       msg.SenderID,
		ContentPreview: truncate(msg.Content.Text, 100),
		Type:           msg.Type,
		SentAt:         msg.CreatedAt,
	}
	if err := s.convRepo.UpdateLastMessage(ctx, cmd.ConversationID, lastMsg); err != nil {
		logger.Error(ctx, "failed to update last message", err, logging.String("conv_id", cmd.ConversationID))
	}

	// Publish event to Kafka
	if err := s.publisher.PublishMessageSent(ctx, msg); err != nil {
		logger.Error(ctx, "failed to publish message event", err, logging.String("msg_id", msg.ID))
	}

	// Push new message to all conversation subscribers via Centrifugo
	if err := s.realtime.PublishToConversation(ctx, cmd.ConversationID, "message.new", msg); err != nil {
		logger.Error(ctx, "failed to publish realtime message", err, logging.String("msg_id", msg.ID))
	}

	// Increment unread counts for other participants
	for _, pid := range conv.ParticipantIDs() {
		if pid == cmd.SenderID {
			continue
		}
		if err := s.cache.IncrementUnread(ctx, pid, cmd.ConversationID); err != nil {
			logger.Error(ctx, "failed to increment unread", err, logging.String("user_id", pid))
		}
	}

	return msg, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
