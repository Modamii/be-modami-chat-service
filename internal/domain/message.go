package domain

import "time"

// MessageType represents the type of message content.
type MessageType string

const (
	MessageTypeText     MessageType = "text"
	MessageTypeImage    MessageType = "image"
	MessageTypeVideo    MessageType = "video"
	MessageTypeFile     MessageType = "file"
	MessageTypeAudio    MessageType = "audio"
	MessageTypeLocation MessageType = "location"
	MessageTypeSystem   MessageType = "system"
)

// Message represents a chat message in a conversation.
type Message struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	SenderID       string         `json:"sender_id"`
	Type           MessageType    `json:"type"`
	Content        MessageContent `json:"content"`
	Media          *MediaInfo     `json:"media,omitempty"`
	ReplyTo        *ReplyInfo     `json:"reply_to,omitempty"`
	ForwardedFrom  *ForwardInfo   `json:"forwarded_from,omitempty"`
	Reactions      []Reaction     `json:"reactions"`
	Edited         bool           `json:"edited"`
	Deleted        bool           `json:"deleted"`
	CreatedAt      time.Time      `json:"created_at"`
}

// MessageContent holds the text content and any mentions.
type MessageContent struct {
	Text     string   `json:"text,omitempty"`
	Mentions []string `json:"mentions,omitempty"`
}

// MediaInfo holds metadata for media messages.
type MediaInfo struct {
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	MIMEType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	FileName     string `json:"file_name,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
}

// ReplyInfo holds denormalized info about the message being replied to.
type ReplyInfo struct {
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
	ContentPreview string `json:"content_preview"`
}

// ForwardInfo holds info about the original forwarded message.
type ForwardInfo struct {
	OriginalMessageID string `json:"original_message_id"`
	OriginalSenderID  string `json:"original_sender_id"`
}

// Reaction groups emoji reactions by emoji type.
type Reaction struct {
	Emoji   string   `json:"emoji"`
	UserIDs []string `json:"user_ids"`
}

const (
	MaxMessageContentLength = 4096
	MaxReactionsPerMessage  = 20
	MaxUsersPerReaction     = 50
)

// NewMessage creates a new text message with defaults.
func NewMessage(id, conversationID, senderID, text string) *Message {
	return &Message{
		ID:             id,
		ConversationID: conversationID,
		SenderID:       senderID,
		Type:           MessageTypeText,
		Content: MessageContent{
			Text: text,
		},
		Reactions: []Reaction{},
		CreatedAt: time.Now(),
	}
}

// NewMediaMessage creates a new media message.
func NewMediaMessage(id, conversationID, senderID string, msgType MessageType, media *MediaInfo) *Message {
	return &Message{
		ID:             id,
		ConversationID: conversationID,
		SenderID:       senderID,
		Type:           msgType,
		Media:          media,
		Reactions:      []Reaction{},
		CreatedAt:      time.Now(),
	}
}

// Validate checks message invariants.
func (m *Message) Validate() error {
	if m.ConversationID == "" {
		return ErrInvalidInput("conversation_id is required")
	}
	if m.SenderID == "" {
		return ErrInvalidInput("sender_id is required")
	}
	if m.Type == MessageTypeText && len(m.Content.Text) == 0 {
		return ErrInvalidInput("text content is required for text messages")
	}
	if len(m.Content.Text) > MaxMessageContentLength {
		return ErrInvalidInput("message content exceeds maximum length")
	}
	if m.Type != MessageTypeText && m.Type != MessageTypeSystem && m.Media == nil {
		return ErrInvalidInput("media info is required for media messages")
	}
	return nil
}

// AddReaction adds a user's reaction, returning true if added (false if already exists).
func (m *Message) AddReaction(emoji, userID string) (bool, error) {
	for i, r := range m.Reactions {
		if r.Emoji == emoji {
			for _, uid := range r.UserIDs {
				if uid == userID {
					return false, nil
				}
			}
			if len(r.UserIDs) >= MaxUsersPerReaction {
				return false, ErrReactionLimitReached
			}
			m.Reactions[i].UserIDs = append(m.Reactions[i].UserIDs, userID)
			return true, nil
		}
	}
	if len(m.Reactions) >= MaxReactionsPerMessage {
		return false, ErrReactionLimitReached
	}
	m.Reactions = append(m.Reactions, Reaction{
		Emoji:   emoji,
		UserIDs: []string{userID},
	})
	return true, nil
}

// RemoveReaction removes a user's reaction, returning true if removed.
func (m *Message) RemoveReaction(emoji, userID string) bool {
	for i, r := range m.Reactions {
		if r.Emoji == emoji {
			for j, uid := range r.UserIDs {
				if uid == userID {
					m.Reactions[i].UserIDs = append(r.UserIDs[:j], r.UserIDs[j+1:]...)
					if len(m.Reactions[i].UserIDs) == 0 {
						m.Reactions = append(m.Reactions[:i], m.Reactions[i+1:]...)
					}
					return true
				}
			}
		}
	}
	return false
}
