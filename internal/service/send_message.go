package service

import (
	"context"
	"fmt"

	"be-modami-chat-service/internal/domain"

	"github.com/rs/zerolog/log"
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
		log.Error().Err(err).Str("conv_id", cmd.ConversationID).Msg("failed to update last message")
	}

	// Publish event to Kafka (async delivery via consumer)
	if err := s.publisher.PublishMessageSent(ctx, msg); err != nil {
		log.Error().Err(err).Str("msg_id", msg.ID).Msg("failed to publish message event")
	}

	// Increment unread counts for other participants
	for _, pid := range conv.ParticipantIDs() {
		if pid == cmd.SenderID {
			continue
		}
		if err := s.cache.IncrementUnread(ctx, pid, cmd.ConversationID); err != nil {
			log.Error().Err(err).Str("user_id", pid).Msg("failed to increment unread")
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
