package mongo

import (
	"time"

	"be-modami-chat-service/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// messageDoc is the MongoDB document representation of a Message.
type messageDoc struct {
	ID             bson.ObjectID `bson:"_id"`
	ConversationID bson.ObjectID `bson:"conversation_id"`
	SenderID       string        `bson:"sender_id"`
	Type           string        `bson:"type"`
	Content        contentDoc    `bson:"content"`
	Media          *mediaDoc     `bson:"media,omitempty"`
	ReplyTo        *replyDoc     `bson:"reply_to,omitempty"`
	ForwardedFrom  *forwardDoc   `bson:"forwarded_from,omitempty"`
	Reactions      []reactionDoc `bson:"reactions"`
	Edited         bool          `bson:"edited"`
	Deleted        bool          `bson:"deleted"`
	CreatedAt      time.Time     `bson:"created_at"`
}

type contentDoc struct {
	Text     string   `bson:"text"`
	Mentions []string `bson:"mentions,omitempty"`
}

type mediaDoc struct {
	URL          string `bson:"url"`
	ThumbnailURL string `bson:"thumbnail_url,omitempty"`
	MIMEType     string `bson:"mime_type"`
	FileSize     int64  `bson:"file_size"`
	FileName     string `bson:"file_name,omitempty"`
	Width        int    `bson:"width,omitempty"`
	Height       int    `bson:"height,omitempty"`
}

type replyDoc struct {
	MessageID      bson.ObjectID `bson:"message_id"`
	SenderID       string        `bson:"sender_id"`
	ContentPreview string        `bson:"content_preview"`
}

type forwardDoc struct {
	OriginalMessageID bson.ObjectID `bson:"original_message_id"`
	OriginalSenderID  string        `bson:"original_sender_id"`
}

type reactionDoc struct {
	Emoji   string   `bson:"emoji"`
	UserIDs []string `bson:"user_ids"`
}

// conversationDoc is the MongoDB document representation of a Conversation.
type conversationDoc struct {
	ID           bson.ObjectID    `bson:"_id"`
	Type         string           `bson:"type"`
	Name         string           `bson:"name,omitempty"`
	AvatarURL    string           `bson:"avatar_url,omitempty"`
	CreatedBy    string           `bson:"created_by"`
	Participants []participantDoc `bson:"participants"`
	LastMessage  *lastMessageDoc  `bson:"last_message,omitempty"`
	Settings     settingsDoc      `bson:"settings"`
	CreatedAt    time.Time        `bson:"created_at"`
	UpdatedAt    time.Time        `bson:"updated_at"`
}

type participantDoc struct {
	UserID                  string     `bson:"user_id"`
	Role                    string     `bson:"role"`
	JoinedAt                time.Time  `bson:"joined_at"`
	LastReadMessageID       string     `bson:"last_read_message_id,omitempty"`
	LastReadAt              time.Time  `bson:"last_read_at,omitempty"`
	NotificationsMutedUntil *time.Time `bson:"notifications_muted_until,omitempty"`
}

type lastMessageDoc struct {
	MessageID      bson.ObjectID `bson:"message_id"`
	SenderID       string        `bson:"sender_id"`
	ContentPreview string        `bson:"content_preview"`
	Type           string        `bson:"type"`
	SentAt         time.Time     `bson:"sent_at"`
}

type settingsDoc struct {
	OnlyAdminsCanSend bool `bson:"only_admins_can_send"`
}

// --- Mappers ---

func messageFromDomain(m *domain.Message) *messageDoc {
	doc := &messageDoc{
		ID:             mustObjectID(m.ID),
		ConversationID: mustObjectID(m.ConversationID),
		SenderID:       m.SenderID,
		Type:           string(m.Type),
		Content: contentDoc{
			Text:     m.Content.Text,
			Mentions: m.Content.Mentions,
		},
		Reactions: make([]reactionDoc, len(m.Reactions)),
		Edited:    m.Edited,
		Deleted:   m.Deleted,
		CreatedAt: m.CreatedAt,
	}

	if m.Media != nil {
		doc.Media = &mediaDoc{
			URL:          m.Media.URL,
			ThumbnailURL: m.Media.ThumbnailURL,
			MIMEType:     m.Media.MIMEType,
			FileSize:     m.Media.FileSize,
			FileName:     m.Media.FileName,
			Width:        m.Media.Width,
			Height:       m.Media.Height,
		}
	}

	if m.ReplyTo != nil {
		doc.ReplyTo = &replyDoc{
			MessageID:      mustObjectID(m.ReplyTo.MessageID),
			SenderID:       m.ReplyTo.SenderID,
			ContentPreview: m.ReplyTo.ContentPreview,
		}
	}

	if m.ForwardedFrom != nil {
		doc.ForwardedFrom = &forwardDoc{
			OriginalMessageID: mustObjectID(m.ForwardedFrom.OriginalMessageID),
			OriginalSenderID:  m.ForwardedFrom.OriginalSenderID,
		}
	}

	for i, r := range m.Reactions {
		doc.Reactions[i] = reactionDoc{Emoji: r.Emoji, UserIDs: r.UserIDs}
	}

	return doc
}

func messageToDomain(doc *messageDoc) *domain.Message {
	m := &domain.Message{
		ID:             doc.ID.Hex(),
		ConversationID: doc.ConversationID.Hex(),
		SenderID:       doc.SenderID,
		Type:           domain.MessageType(doc.Type),
		Content: domain.MessageContent{
			Text:     doc.Content.Text,
			Mentions: doc.Content.Mentions,
		},
		Reactions: make([]domain.Reaction, len(doc.Reactions)),
		Edited:    doc.Edited,
		Deleted:   doc.Deleted,
		CreatedAt: doc.CreatedAt,
	}

	if doc.Media != nil {
		m.Media = &domain.MediaInfo{
			URL:          doc.Media.URL,
			ThumbnailURL: doc.Media.ThumbnailURL,
			MIMEType:     doc.Media.MIMEType,
			FileSize:     doc.Media.FileSize,
			FileName:     doc.Media.FileName,
			Width:        doc.Media.Width,
			Height:       doc.Media.Height,
		}
	}

	if doc.ReplyTo != nil {
		m.ReplyTo = &domain.ReplyInfo{
			MessageID:      doc.ReplyTo.MessageID.Hex(),
			SenderID:       doc.ReplyTo.SenderID,
			ContentPreview: doc.ReplyTo.ContentPreview,
		}
	}

	if doc.ForwardedFrom != nil {
		m.ForwardedFrom = &domain.ForwardInfo{
			OriginalMessageID: doc.ForwardedFrom.OriginalMessageID.Hex(),
			OriginalSenderID:  doc.ForwardedFrom.OriginalSenderID,
		}
	}

	for i, r := range doc.Reactions {
		m.Reactions[i] = domain.Reaction{Emoji: r.Emoji, UserIDs: r.UserIDs}
	}

	return m
}

func conversationToDomain(doc *conversationDoc) *domain.Conversation {
	c := &domain.Conversation{
		ID:           doc.ID.Hex(),
		Type:         domain.ConversationType(doc.Type),
		Name:         doc.Name,
		AvatarURL:    doc.AvatarURL,
		CreatedBy:    doc.CreatedBy,
		Participants: make([]domain.Participant, len(doc.Participants)),
		Settings: domain.ConversationSettings{
			OnlyAdminsCanSend: doc.Settings.OnlyAdminsCanSend,
		},
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}

	if doc.LastMessage != nil {
		c.LastMessage = &domain.LastMessage{
			MessageID:      doc.LastMessage.MessageID.Hex(),
			SenderID:       doc.LastMessage.SenderID,
			ContentPreview: doc.LastMessage.ContentPreview,
			Type:           domain.MessageType(doc.LastMessage.Type),
			SentAt:         doc.LastMessage.SentAt,
		}
	}

	for i, p := range doc.Participants {
		c.Participants[i] = domain.Participant{
			UserID:                  p.UserID,
			Role:                    domain.ParticipantRole(p.Role),
			JoinedAt:                p.JoinedAt,
			LastReadMessageID:       p.LastReadMessageID,
			LastReadAt:              p.LastReadAt,
			NotificationsMutedUntil: p.NotificationsMutedUntil,
		}
	}

	return c
}

func conversationFromDomain(c *domain.Conversation) *conversationDoc {
	doc := &conversationDoc{
		ID:           mustObjectID(c.ID),
		Type:         string(c.Type),
		Name:         c.Name,
		AvatarURL:    c.AvatarURL,
		CreatedBy:    c.CreatedBy,
		Participants: make([]participantDoc, len(c.Participants)),
		Settings: settingsDoc{
			OnlyAdminsCanSend: c.Settings.OnlyAdminsCanSend,
		},
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}

	if c.LastMessage != nil {
		doc.LastMessage = &lastMessageDoc{
			MessageID:      mustObjectID(c.LastMessage.MessageID),
			SenderID:       c.LastMessage.SenderID,
			ContentPreview: c.LastMessage.ContentPreview,
			Type:           string(c.LastMessage.Type),
			SentAt:         c.LastMessage.SentAt,
		}
	}

	for i, p := range c.Participants {
		doc.Participants[i] = participantDoc{
			UserID:                  p.UserID,
			Role:                    string(p.Role),
			JoinedAt:                p.JoinedAt,
			LastReadMessageID:       p.LastReadMessageID,
			LastReadAt:              p.LastReadAt,
			NotificationsMutedUntil: p.NotificationsMutedUntil,
		}
	}

	return doc
}

func mustObjectID(hex string) bson.ObjectID {
	oid, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		return bson.ObjectID{}
	}
	return oid
}
