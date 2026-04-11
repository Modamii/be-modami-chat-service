package domain

import (
	"sort"
	"time"
)

// ConversationType represents the type of conversation.
type ConversationType string

const (
	ConversationTypeDirect ConversationType = "direct"
	ConversationTypeGroup  ConversationType = "group"
)

// ParticipantRole defines the role of a participant in a conversation.
type ParticipantRole string

const (
	RoleMember ParticipantRole = "member"
	RoleAdmin  ParticipantRole = "admin"
)

// Conversation represents a chat conversation (1-1 or group).
type Conversation struct {
	ID           string           `json:"id"`
	Type         ConversationType `json:"type"`
	Name         string           `json:"name,omitempty"`
	AvatarURL    string           `json:"avatar_url,omitempty"`
	CreatedBy    string           `json:"created_by"`
	Participants []Participant    `json:"participants"`
	LastMessage  *LastMessage     `json:"last_message,omitempty"`
	Settings     ConversationSettings `json:"settings"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// Participant represents a user in a conversation.
type Participant struct {
	UserID                  string          `json:"user_id"`
	Role                    ParticipantRole `json:"role"`
	JoinedAt                time.Time       `json:"joined_at"`
	LastReadMessageID       string          `json:"last_read_message_id,omitempty"`
	LastReadAt              time.Time       `json:"last_read_at,omitempty"`
	NotificationsMutedUntil *time.Time      `json:"notifications_muted_until,omitempty"`
}

// LastMessage holds denormalized last message info for conversation list display.
type LastMessage struct {
	MessageID      string      `json:"message_id"`
	SenderID       string      `json:"sender_id"`
	ContentPreview string      `json:"content_preview"`
	Type           MessageType `json:"type"`
	SentAt         time.Time   `json:"sent_at"`
}

// ConversationSettings holds group-specific settings.
type ConversationSettings struct {
	OnlyAdminsCanSend bool `json:"only_admins_can_send"`
}

const (
	MaxGroupParticipants  = 50
	MaxDirectParticipants = 2
	MaxConversationName   = 100
)

// NewDirectConversation creates a new 1-1 conversation.
func NewDirectConversation(id, userID1, userID2 string) *Conversation {
	now := time.Now()
	participants := []Participant{
		{UserID: userID1, Role: RoleMember, JoinedAt: now},
		{UserID: userID2, Role: RoleMember, JoinedAt: now},
	}
	// Sort participants lexicographically for consistent lookup
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].UserID < participants[j].UserID
	})

	return &Conversation{
		ID:           id,
		Type:         ConversationTypeDirect,
		CreatedBy:    userID1,
		Participants: participants,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// NewGroupConversation creates a new group conversation.
func NewGroupConversation(id, name, createdBy string, memberIDs []string) (*Conversation, error) {
	if name == "" {
		return nil, ErrInvalidInput("group name is required")
	}
	if len(name) > MaxConversationName {
		return nil, ErrInvalidInput("group name exceeds maximum length")
	}
	if len(memberIDs) < 2 {
		return nil, ErrInvalidInput("group must have at least 2 members")
	}
	if len(memberIDs)+1 > MaxGroupParticipants {
		return nil, ErrGroupFull
	}

	now := time.Now()
	participants := make([]Participant, 0, len(memberIDs)+1)
	participants = append(participants, Participant{
		UserID:   createdBy,
		Role:     RoleAdmin,
		JoinedAt: now,
	})

	seen := map[string]bool{createdBy: true}
	for _, uid := range memberIDs {
		if seen[uid] {
			continue
		}
		seen[uid] = true
		participants = append(participants, Participant{
			UserID:   uid,
			Role:     RoleMember,
			JoinedAt: now,
		})
	}

	return &Conversation{
		ID:           id,
		Type:         ConversationTypeGroup,
		Name:         name,
		CreatedBy:    createdBy,
		Participants: participants,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// HasMember checks if a user is a participant.
func (c *Conversation) HasMember(userID string) bool {
	for _, p := range c.Participants {
		if p.UserID == userID {
			return true
		}
	}
	return false
}

// IsAdmin checks if a user has admin role.
func (c *Conversation) IsAdmin(userID string) bool {
	for _, p := range c.Participants {
		if p.UserID == userID && p.Role == RoleAdmin {
			return true
		}
	}
	return false
}

// AddParticipant adds a new member to a group conversation.
func (c *Conversation) AddParticipant(userID string) error {
	if c.Type == ConversationTypeDirect {
		return ErrInvalidInput("cannot add participants to direct conversations")
	}
	if c.HasMember(userID) {
		return ErrAlreadyMember
	}
	if len(c.Participants) >= MaxGroupParticipants {
		return ErrGroupFull
	}
	c.Participants = append(c.Participants, Participant{
		UserID:   userID,
		Role:     RoleMember,
		JoinedAt: time.Now(),
	})
	c.UpdatedAt = time.Now()
	return nil
}

// RemoveParticipant removes a member from a group conversation.
func (c *Conversation) RemoveParticipant(userID string) error {
	if c.Type == ConversationTypeDirect {
		return ErrInvalidInput("cannot remove participants from direct conversations")
	}
	for i, p := range c.Participants {
		if p.UserID == userID {
			c.Participants = append(c.Participants[:i], c.Participants[i+1:]...)
			c.UpdatedAt = time.Now()
			return nil
		}
	}
	return ErrNotChatMember
}

// GetParticipant returns the participant info for a user.
func (c *Conversation) GetParticipant(userID string) *Participant {
	for i, p := range c.Participants {
		if p.UserID == userID {
			return &c.Participants[i]
		}
	}
	return nil
}

// UpdateLastMessage updates the denormalized last message field.
func (c *Conversation) UpdateLastMessage(msg *Message) {
	preview := msg.Content.Text
	if len(preview) > 100 {
		preview = preview[:100]
	}
	c.LastMessage = &LastMessage{
		MessageID:      msg.ID,
		SenderID:       msg.SenderID,
		ContentPreview: preview,
		Type:           msg.Type,
		SentAt:         msg.CreatedAt,
	}
	c.UpdatedAt = msg.CreatedAt
}

// ParticipantIDs returns all participant user IDs.
func (c *Conversation) ParticipantIDs() []string {
	ids := make([]string, len(c.Participants))
	for i, p := range c.Participants {
		ids[i] = p.UserID
	}
	return ids
}

// DirectChatKey returns a consistent key for direct chat lookup (sorted user IDs).
func DirectChatKey(userID1, userID2 string) (string, string) {
	if userID1 < userID2 {
		return userID1, userID2
	}
	return userID2, userID1
}
