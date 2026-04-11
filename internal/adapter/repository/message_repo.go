package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gocql/gocql"

	"be-modami-chat-service/internal/domain"
)

// MessageRepo implements port.MessageRepository using ScyllaDB.
type MessageRepo struct {
	session *gocql.Session
}

// NewMessageRepo creates a new ScyllaDB message repository.
func NewMessageRepo(session *gocql.Session) *MessageRepo {
	return &MessageRepo{session: session}
}

// --- reaction helpers ---

type reactionJSON struct {
	Emoji   string   `json:"emoji"`
	UserIDs []string `json:"user_ids"`
}

func marshalReactions(reactions []domain.Reaction) string {
	if len(reactions) == 0 {
		return "[]"
	}
	items := make([]reactionJSON, len(reactions))
	for i, r := range reactions {
		items[i] = reactionJSON{Emoji: r.Emoji, UserIDs: r.UserIDs}
	}
	data, _ := json.Marshal(items)
	return string(data)
}

func unmarshalReactions(data string) []domain.Reaction {
	if data == "" || data == "null" || data == "[]" {
		return []domain.Reaction{}
	}
	var items []reactionJSON
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return []domain.Reaction{}
	}
	reactions := make([]domain.Reaction, len(items))
	for i, item := range items {
		userIDs := item.UserIDs
		if userIDs == nil {
			userIDs = []string{}
		}
		reactions[i] = domain.Reaction{Emoji: item.Emoji, UserIDs: userIDs}
	}
	return reactions
}

// --- Create ---

func (r *MessageRepo) Create(ctx context.Context, msg *domain.Message) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	// Flatten optional fields to plain strings (empty string == absent).
	var (
		mediaURL, mediaThumbnailURL, mediaMIMEType, mediaFileName string
		mediaFileSize                                             int64
		mediaWidth, mediaHeight                                   int
	)
	if msg.Media != nil {
		mediaURL = msg.Media.URL
		mediaThumbnailURL = msg.Media.ThumbnailURL
		mediaMIMEType = msg.Media.MIMEType
		mediaFileSize = msg.Media.FileSize
		mediaFileName = msg.Media.FileName
		mediaWidth = msg.Media.Width
		mediaHeight = msg.Media.Height
	}

	var replyToID, replyToSenderID, replyToPreview string
	if msg.ReplyTo != nil {
		replyToID = msg.ReplyTo.MessageID
		replyToSenderID = msg.ReplyTo.SenderID
		replyToPreview = msg.ReplyTo.ContentPreview
	}

	var fwdFromID, fwdFromSenderID string
	if msg.ForwardedFrom != nil {
		fwdFromID = msg.ForwardedFrom.OriginalMessageID
		fwdFromSenderID = msg.ForwardedFrom.OriginalSenderID
	}

	reactionsJSON := marshalReactions(msg.Reactions)

	// IF NOT EXISTS acts as a duplicate guard (lightweight transaction).
	applied, err := r.session.Query(`
		INSERT INTO messages (
			conversation_id, created_at, id, sender_id, type,
			content_text, content_mentions,
			media_url, media_thumbnail_url, media_mime_type, media_file_size, media_file_name, media_width, media_height,
			reply_to_id, reply_to_sender_id, reply_to_preview,
			forwarded_from_id, forwarded_from_sender_id,
			reactions, edited, deleted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		IF NOT EXISTS`,
		msg.ConversationID, msg.CreatedAt, msg.ID, msg.SenderID, string(msg.Type),
		msg.Content.Text, msg.Content.Mentions,
		mediaURL, mediaThumbnailURL, mediaMIMEType, mediaFileSize, mediaFileName, mediaWidth, mediaHeight,
		replyToID, replyToSenderID, replyToPreview,
		fwdFromID, fwdFromSenderID,
		reactionsJSON, msg.Edited, msg.Deleted,
	).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	if !applied {
		return domain.ErrDuplicateMessage
	}

	// Insert lookup entry (best-effort; not protected by LWT for performance).
	if err := r.session.Query(
		`INSERT INTO messages_by_id (id, conversation_id, created_at) VALUES (?, ?, ?)`,
		msg.ID, msg.ConversationID, msg.CreatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("insert message lookup: %w", err)
	}

	return nil
}

// --- FindByID ---

func (r *MessageRepo) FindByID(ctx context.Context, id string) (*domain.Message, error) {
	// Step 1: resolve the full primary key from the lookup table.
	var convID string
	var createdAt time.Time
	err := r.session.Query(
		`SELECT conversation_id, created_at FROM messages_by_id WHERE id = ?`, id,
	).WithContext(ctx).Scan(&convID, &createdAt)
	if err == gocql.ErrNotFound {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find message lookup: %w", err)
	}

	// Step 2: fetch the full message row.
	return r.fetchMessage(ctx, convID, createdAt, id)
}

// fetchMessage reads one message row given its complete primary key.
func (r *MessageRepo) fetchMessage(ctx context.Context, convID string, createdAt time.Time, id string) (*domain.Message, error) {
	var (
		senderID, msgType, contentText                            string
		contentMentions                                           []string
		mediaURL, mediaThumbnailURL, mediaMIMEType, mediaFileName string
		mediaFileSize                                             int64
		mediaWidth, mediaHeight                                   int
		replyToID, replyToSenderID, replyToPreview               string
		fwdFromID, fwdFromSenderID                               string
		reactionsJSON                                             string
		edited, deleted                                           bool
	)

	err := r.session.Query(`
		SELECT sender_id, type,
		       content_text, content_mentions,
		       media_url, media_thumbnail_url, media_mime_type, media_file_size, media_file_name, media_width, media_height,
		       reply_to_id, reply_to_sender_id, reply_to_preview,
		       forwarded_from_id, forwarded_from_sender_id,
		       reactions, edited, deleted
		FROM messages
		WHERE conversation_id = ? AND created_at = ? AND id = ?`,
		convID, createdAt, id,
	).WithContext(ctx).Scan(
		&senderID, &msgType,
		&contentText, &contentMentions,
		&mediaURL, &mediaThumbnailURL, &mediaMIMEType, &mediaFileSize, &mediaFileName, &mediaWidth, &mediaHeight,
		&replyToID, &replyToSenderID, &replyToPreview,
		&fwdFromID, &fwdFromSenderID,
		&reactionsJSON, &edited, &deleted,
	)
	if err == gocql.ErrNotFound {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fetch message: %w", err)
	}

	return buildMessage(
		convID, createdAt, id, senderID, msgType,
		contentText, contentMentions,
		mediaURL, mediaThumbnailURL, mediaMIMEType, mediaFileSize, mediaFileName, mediaWidth, mediaHeight,
		replyToID, replyToSenderID, replyToPreview,
		fwdFromID, fwdFromSenderID,
		reactionsJSON, edited, deleted,
	), nil
}

// --- FindByConversation ---

func (r *MessageRepo) FindByConversation(ctx context.Context, conversationID string, page domain.PageRequest) (*domain.PageResponse[*domain.Message], error) {
	limit := page.Limit
	if limit <= 0 {
		limit = domain.DefaultPageSize
	}

	var iter *gocql.Iter

	if page.Cursor != nil {
		// Fetch messages older than the cursor using tuple comparison.
		// Clustering order is DESC so (created_at, id) < cursor selects older rows.
		iter = r.session.Query(`
			SELECT conversation_id, created_at, id, sender_id, type,
			       content_text, content_mentions,
			       media_url, media_thumbnail_url, media_mime_type, media_file_size, media_file_name, media_width, media_height,
			       reply_to_id, reply_to_sender_id, reply_to_preview,
			       forwarded_from_id, forwarded_from_sender_id,
			       reactions, edited, deleted
			FROM messages
			WHERE conversation_id = ?
			  AND (created_at, id) < (?, ?)
			LIMIT ?`,
			conversationID, page.Cursor.CreatedAt, page.Cursor.ID, limit+1,
		).WithContext(ctx).Iter()
	} else {
		iter = r.session.Query(`
			SELECT conversation_id, created_at, id, sender_id, type,
			       content_text, content_mentions,
			       media_url, media_thumbnail_url, media_mime_type, media_file_size, media_file_name, media_width, media_height,
			       reply_to_id, reply_to_sender_id, reply_to_preview,
			       forwarded_from_id, forwarded_from_sender_id,
			       reactions, edited, deleted
			FROM messages
			WHERE conversation_id = ?
			LIMIT ?`,
			conversationID, limit+1,
		).WithContext(ctx).Iter()
	}

	var items []*domain.Message
	var (
		convID, id, senderID, msgType, contentText               string
		contentMentions                                           []string
		mediaURL, mediaThumbnailURL, mediaMIMEType, mediaFileName string
		mediaFileSize                                             int64
		mediaWidth, mediaHeight                                   int
		replyToID, replyToSenderID, replyToPreview               string
		fwdFromID, fwdFromSenderID                               string
		reactionsJSON                                             string
		edited, deleted                                           bool
		createdAt                                                 time.Time
	)

	for iter.Scan(
		&convID, &createdAt, &id, &senderID, &msgType,
		&contentText, &contentMentions,
		&mediaURL, &mediaThumbnailURL, &mediaMIMEType, &mediaFileSize, &mediaFileName, &mediaWidth, &mediaHeight,
		&replyToID, &replyToSenderID, &replyToPreview,
		&fwdFromID, &fwdFromSenderID,
		&reactionsJSON, &edited, &deleted,
	) {
		items = append(items, buildMessage(
			convID, createdAt, id, senderID, msgType,
			contentText, contentMentions,
			mediaURL, mediaThumbnailURL, mediaMIMEType, mediaFileSize, mediaFileName, mediaWidth, mediaHeight,
			replyToID, replyToSenderID, replyToPreview,
			fwdFromID, fwdFromSenderID,
			reactionsJSON, edited, deleted,
		))
		// reset slice for next iteration (gocql reuses the backing array)
		contentMentions = nil
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	resp := &domain.PageResponse[*domain.Message]{Items: items, HasMore: hasMore}
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		resp.NextCursor = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return resp, nil
}

// --- Update ---

func (r *MessageRepo) Update(ctx context.Context, msg *domain.Message) error {
	var (
		mediaURL, mediaThumbnailURL, mediaMIMEType, mediaFileName string
		mediaFileSize                                             int64
		mediaWidth, mediaHeight                                   int
	)
	if msg.Media != nil {
		mediaURL = msg.Media.URL
		mediaThumbnailURL = msg.Media.ThumbnailURL
		mediaMIMEType = msg.Media.MIMEType
		mediaFileSize = msg.Media.FileSize
		mediaFileName = msg.Media.FileName
		mediaWidth = msg.Media.Width
		mediaHeight = msg.Media.Height
	}

	var replyToID, replyToSenderID, replyToPreview string
	if msg.ReplyTo != nil {
		replyToID = msg.ReplyTo.MessageID
		replyToSenderID = msg.ReplyTo.SenderID
		replyToPreview = msg.ReplyTo.ContentPreview
	}

	var fwdFromID, fwdFromSenderID string
	if msg.ForwardedFrom != nil {
		fwdFromID = msg.ForwardedFrom.OriginalMessageID
		fwdFromSenderID = msg.ForwardedFrom.OriginalSenderID
	}

	reactionsJSON := marshalReactions(msg.Reactions)

	err := r.session.Query(`
		UPDATE messages SET
			sender_id = ?,
			type = ?,
			content_text = ?,
			content_mentions = ?,
			media_url = ?,
			media_thumbnail_url = ?,
			media_mime_type = ?,
			media_file_size = ?,
			media_file_name = ?,
			media_width = ?,
			media_height = ?,
			reply_to_id = ?,
			reply_to_sender_id = ?,
			reply_to_preview = ?,
			forwarded_from_id = ?,
			forwarded_from_sender_id = ?,
			reactions = ?,
			edited = ?,
			deleted = ?
		WHERE conversation_id = ? AND created_at = ? AND id = ?`,
		msg.SenderID,
		string(msg.Type),
		msg.Content.Text,
		msg.Content.Mentions,
		mediaURL, mediaThumbnailURL, mediaMIMEType, mediaFileSize, mediaFileName, mediaWidth, mediaHeight,
		replyToID, replyToSenderID, replyToPreview,
		fwdFromID, fwdFromSenderID,
		reactionsJSON,
		msg.Edited,
		msg.Deleted,
		msg.ConversationID, msg.CreatedAt, msg.ID,
	).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	return nil
}

// --- AddReaction ---

func (r *MessageRepo) AddReaction(ctx context.Context, messageID, emoji, userID string) error {
	msg, err := r.FindByID(ctx, messageID)
	if err != nil {
		return err
	}

	// Apply mutation using the domain helper to stay consistent with limits.
	added, err := msg.AddReaction(emoji, userID)
	if err != nil {
		return err
	}
	if !added {
		return nil
	}

	return r.session.Query(
		`UPDATE messages SET reactions = ? WHERE conversation_id = ? AND created_at = ? AND id = ?`,
		marshalReactions(msg.Reactions), msg.ConversationID, msg.CreatedAt, msg.ID,
	).WithContext(ctx).Exec()
}

// --- RemoveReaction ---

func (r *MessageRepo) RemoveReaction(ctx context.Context, messageID, emoji, userID string) error {
	msg, err := r.FindByID(ctx, messageID)
	if err != nil {
		return err
	}

	removed := msg.RemoveReaction(emoji, userID)
	if !removed {
		return nil
	}

	return r.session.Query(
		`UPDATE messages SET reactions = ? WHERE conversation_id = ? AND created_at = ? AND id = ?`,
		marshalReactions(msg.Reactions), msg.ConversationID, msg.CreatedAt, msg.ID,
	).WithContext(ctx).Exec()
}

// --- buildMessage constructs a domain.Message from scanned column values ---

func buildMessage(
	convID string, createdAt time.Time, id, senderID, msgType,
	contentText string, contentMentions []string,
	mediaURL, mediaThumbnailURL, mediaMIMEType string,
	mediaFileSize int64,
	mediaFileName string,
	mediaWidth, mediaHeight int,
	replyToID, replyToSenderID, replyToPreview,
	fwdFromID, fwdFromSenderID,
	reactionsJSON string,
	edited, deleted bool,
) *domain.Message {
	mentions := contentMentions
	if mentions == nil {
		mentions = []string{}
	}

	m := &domain.Message{
		ID:             id,
		ConversationID: convID,
		SenderID:       senderID,
		Type:           domain.MessageType(msgType),
		Content: domain.MessageContent{
			Text:     contentText,
			Mentions: mentions,
		},
		Reactions: unmarshalReactions(reactionsJSON),
		Edited:    edited,
		Deleted:   deleted,
		CreatedAt: createdAt,
	}

	if mediaURL != "" {
		m.Media = &domain.MediaInfo{
			URL:          mediaURL,
			ThumbnailURL: mediaThumbnailURL,
			MIMEType:     mediaMIMEType,
			FileSize:     mediaFileSize,
			FileName:     mediaFileName,
			Width:        mediaWidth,
			Height:       mediaHeight,
		}
	}

	if replyToID != "" {
		m.ReplyTo = &domain.ReplyInfo{
			MessageID:      replyToID,
			SenderID:       replyToSenderID,
			ContentPreview: replyToPreview,
		}
	}

	if fwdFromID != "" {
		m.ForwardedFrom = &domain.ForwardInfo{
			OriginalMessageID: fwdFromID,
			OriginalSenderID:  fwdFromSenderID,
		}
	}

	return m
}
