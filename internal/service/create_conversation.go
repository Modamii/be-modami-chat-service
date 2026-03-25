package service

import (
	"context"
	"fmt"

	"be-modami-chat-service/internal/domain"
)

// CreateDirectChatCommand holds input for creating a 1-1 conversation.
type CreateDirectChatCommand struct {
	UserID1 string
	UserID2 string
}

// CreateGroupCommand holds input for creating a group conversation.
type CreateGroupCommand struct {
	Name      string
	CreatedBy string
	MemberIDs []string
}

// CreateDirectChat creates or returns an existing direct conversation.
func (s *ChatService) CreateDirectChat(ctx context.Context, cmd CreateDirectChatCommand) (*domain.Conversation, error) {
	if cmd.UserID1 == cmd.UserID2 {
		return nil, domain.ErrInvalidInput("cannot create conversation with yourself")
	}

	// Check if direct chat already exists
	existing, err := s.convRepo.FindDirectChat(ctx, cmd.UserID1, cmd.UserID2)
	if err == nil && existing != nil {
		return existing, nil
	}

	conv := domain.NewDirectConversation(s.idGen.NewID(), cmd.UserID1, cmd.UserID2)
	if err := s.convRepo.Create(ctx, conv); err != nil {
		return nil, fmt.Errorf("create direct chat: %w", err)
	}

	return conv, nil
}

// CreateGroup creates a new group conversation.
func (s *ChatService) CreateGroup(ctx context.Context, cmd CreateGroupCommand) (*domain.Conversation, error) {
	conv, err := domain.NewGroupConversation(s.idGen.NewID(), cmd.Name, cmd.CreatedBy, cmd.MemberIDs)
	if err != nil {
		return nil, err
	}

	if err := s.convRepo.Create(ctx, conv); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}

	return conv, nil
}

// AddParticipant adds a user to a group conversation.
func (s *ChatService) AddParticipant(ctx context.Context, conversationID, requesterID, userID string) error {
	conv, err := s.convRepo.FindByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("find conversation: %w", err)
	}
	if !conv.IsAdmin(requesterID) {
		return domain.ErrForbidden
	}
	if err := conv.AddParticipant(userID); err != nil {
		return err
	}
	participant := domain.Participant{
		UserID: userID,
		Role:   domain.RoleMember,
	}
	return s.convRepo.AddParticipant(ctx, conversationID, participant)
}

// RemoveParticipant removes a user from a group conversation.
func (s *ChatService) RemoveParticipant(ctx context.Context, conversationID, requesterID, userID string) error {
	conv, err := s.convRepo.FindByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("find conversation: %w", err)
	}
	// Admins can remove others, any member can remove themselves
	if requesterID != userID && !conv.IsAdmin(requesterID) {
		return domain.ErrForbidden
	}
	return s.convRepo.RemoveParticipant(ctx, conversationID, userID)
}
