package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/gocql/gocql"

	"be-modami-chat-service/internal/domain"
)

// ConversationRepo implements port.ConversationRepository using ScyllaDB.
type ConversationRepo struct {
	session *gocql.Session
}

// NewConversationRepo creates a new ScyllaDB conversation repository.
func NewConversationRepo(session *gocql.Session) *ConversationRepo {
	return &ConversationRepo{session: session}
}

// --- JSON helpers for participants and last_message ---

type participantJSON struct {
	UserID                  string     `json:"user_id"`
	Role                    string     `json:"role"`
	JoinedAt                time.Time  `json:"joined_at"`
	LastReadMessageID       string     `json:"last_read_message_id,omitempty"`
	LastReadAt              time.Time  `json:"last_read_at,omitempty"`
	NotificationsMutedUntil *time.Time `json:"notifications_muted_until,omitempty"`
}

type lastMessageJSON struct {
	MessageID      string    `json:"message_id"`
	SenderID       string    `json:"sender_id"`
	ContentPreview string    `json:"content_preview"`
	Type           string    `json:"type"`
	SentAt         time.Time `json:"sent_at"`
}

func marshalParticipants(participants []domain.Participant) string {
	items := make([]participantJSON, len(participants))
	for i, p := range participants {
		items[i] = participantJSON{
			UserID:                  p.UserID,
			Role:                    string(p.Role),
			JoinedAt:                p.JoinedAt,
			LastReadMessageID:       p.LastReadMessageID,
			LastReadAt:              p.LastReadAt,
			NotificationsMutedUntil: p.NotificationsMutedUntil,
		}
	}
	data, _ := json.Marshal(items)
	return string(data)
}

func unmarshalParticipants(data string) []domain.Participant {
	if data == "" || data == "null" {
		return []domain.Participant{}
	}
	var items []participantJSON
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return []domain.Participant{}
	}
	participants := make([]domain.Participant, len(items))
	for i, item := range items {
		participants[i] = domain.Participant{
			UserID:                  item.UserID,
			Role:                    domain.ParticipantRole(item.Role),
			JoinedAt:                item.JoinedAt,
			LastReadMessageID:       item.LastReadMessageID,
			LastReadAt:              item.LastReadAt,
			NotificationsMutedUntil: item.NotificationsMutedUntil,
		}
	}
	return participants
}

func marshalLastMessage(lm *domain.LastMessage) string {
	if lm == nil {
		return ""
	}
	data, _ := json.Marshal(lastMessageJSON{
		MessageID:      lm.MessageID,
		SenderID:       lm.SenderID,
		ContentPreview: lm.ContentPreview,
		Type:           string(lm.Type),
		SentAt:         lm.SentAt,
	})
	return string(data)
}

func unmarshalLastMessage(data string) *domain.LastMessage {
	if data == "" || data == "null" {
		return nil
	}
	var item lastMessageJSON
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		return nil
	}
	return &domain.LastMessage{
		MessageID:      item.MessageID,
		SenderID:       item.SenderID,
		ContentPreview: item.ContentPreview,
		Type:           domain.MessageType(item.Type),
		SentAt:         item.SentAt,
	}
}

// directKey builds a consistent lookup key from two user IDs.
func directKey(userID1, userID2 string) string {
	u1, u2 := domain.DirectChatKey(userID1, userID2)
	return u1 + ":" + u2
}

// --- Create ---

func (r *ConversationRepo) Create(ctx context.Context, conv *domain.Conversation) error {
	participantsJSON := marshalParticipants(conv.Participants)
	lastMsgJSON := marshalLastMessage(conv.LastMessage)

	// For direct conversations, atomically claim the lookup key first.
	// This prevents duplicate direct chats between the same two users.
	if conv.Type == domain.ConversationTypeDirect && len(conv.Participants) == 2 {
		key := directKey(conv.Participants[0].UserID, conv.Participants[1].UserID)
		applied, err := r.session.Query(
			`INSERT INTO direct_conversations (lookup_key, conversation_id) VALUES (?, ?) IF NOT EXISTS`,
			key, conv.ID,
		).WithContext(ctx).MapScanCAS(map[string]interface{}{})
		if err != nil {
			return fmt.Errorf("insert direct conversation index: %w", err)
		}
		if !applied {
			return domain.ErrDuplicateConversation
		}
	}

	// Insert the conversation row.
	if err := r.session.Query(`
		INSERT INTO conversations (id, type, name, avatar_url, created_by, participants, last_message, only_admins_can_send, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conv.ID, string(conv.Type), conv.Name, conv.AvatarURL, conv.CreatedBy,
		participantsJSON, lastMsgJSON,
		conv.Settings.OnlyAdminsCanSend,
		conv.CreatedAt, conv.UpdatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("insert conversation: %w", err)
	}

	// Populate forward and reverse participant indexes.
	for _, p := range conv.Participants {
		if err := r.insertParticipantIndexes(ctx, conv.ID, p.UserID); err != nil {
			return err
		}
	}

	return nil
}

// insertParticipantIndexes writes to both user_conversations and
// conversation_participants for one (conversation, user) pair.
func (r *ConversationRepo) insertParticipantIndexes(ctx context.Context, conversationID, userID string) error {
	if err := r.session.Query(
		`INSERT INTO user_conversations (user_id, conversation_id) VALUES (?, ?)`,
		userID, conversationID,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("insert user_conversations: %w", err)
	}

	if err := r.session.Query(
		`INSERT INTO conversation_participants (conversation_id, user_id) VALUES (?, ?)`,
		conversationID, userID,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("insert conversation_participants: %w", err)
	}

	return nil
}

// deleteParticipantIndexes removes both index entries for one (conversation, user) pair.
func (r *ConversationRepo) deleteParticipantIndexes(ctx context.Context, conversationID, userID string) error {
	if err := r.session.Query(
		`DELETE FROM user_conversations WHERE user_id = ? AND conversation_id = ?`,
		userID, conversationID,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("delete user_conversations: %w", err)
	}

	if err := r.session.Query(
		`DELETE FROM conversation_participants WHERE conversation_id = ? AND user_id = ?`,
		conversationID, userID,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("delete conversation_participants: %w", err)
	}

	return nil
}

// getParticipantIDs returns all user IDs currently indexed for a conversation.
func (r *ConversationRepo) getParticipantIDs(ctx context.Context, conversationID string) (map[string]struct{}, error) {
	iter := r.session.Query(
		`SELECT user_id FROM conversation_participants WHERE conversation_id = ?`,
		conversationID,
	).WithContext(ctx).Iter()

	result := make(map[string]struct{})
	var uid string
	for iter.Scan(&uid) {
		result[uid] = struct{}{}
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("get participant ids: %w", err)
	}
	return result, nil
}

// --- FindByID ---

func (r *ConversationRepo) FindByID(ctx context.Context, id string) (*domain.Conversation, error) {
	var (
		convType, name, avatarURL, createdBy string
		participantsJSON, lastMsgJSON        string
		onlyAdminsCanSend                    bool
		createdAt, updatedAt                 time.Time
	)

	err := r.session.Query(`
		SELECT type, name, avatar_url, created_by, participants, last_message,
		       only_admins_can_send, created_at, updated_at
		FROM conversations WHERE id = ?`, id,
	).WithContext(ctx).Scan(
		&convType, &name, &avatarURL, &createdBy,
		&participantsJSON, &lastMsgJSON,
		&onlyAdminsCanSend, &createdAt, &updatedAt,
	)
	if err == gocql.ErrNotFound {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find conversation: %w", err)
	}

	return buildConversation(id, convType, name, avatarURL, createdBy,
		participantsJSON, lastMsgJSON, onlyAdminsCanSend, createdAt, updatedAt), nil
}

// --- FindByUser ---
//
// Fetches the user's conversation IDs, batch-loads each conversation, sorts by
// updated_at DESC in memory, and applies cursor pagination. Suitable for users
// with up to a few thousand conversations.

func (r *ConversationRepo) FindByUser(ctx context.Context, userID string, page domain.PageRequest) (*domain.PageResponse[*domain.Conversation], error) {
	// Collect all conversation IDs for this user.
	iter := r.session.Query(
		`SELECT conversation_id FROM user_conversations WHERE user_id = ?`, userID,
	).WithContext(ctx).Iter()

	var convIDs []string
	var convID string
	for iter.Scan(&convID) {
		convIDs = append(convIDs, convID)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("list user conversations: %w", err)
	}

	if len(convIDs) == 0 {
		return &domain.PageResponse[*domain.Conversation]{Items: []*domain.Conversation{}}, nil
	}

	// Batch-fetch conversation details.
	convs := make([]*domain.Conversation, 0, len(convIDs))
	for _, cid := range convIDs {
		conv, err := r.FindByID(ctx, cid)
		if err == domain.ErrNotFound {
			continue // index may be stale; skip gracefully
		}
		if err != nil {
			return nil, fmt.Errorf("fetch conversation %s: %w", cid, err)
		}
		convs = append(convs, conv)
	}

	// Sort by updated_at DESC, then ID DESC as tiebreaker.
	sort.Slice(convs, func(i, j int) bool {
		if convs[i].UpdatedAt.Equal(convs[j].UpdatedAt) {
			return convs[i].ID > convs[j].ID
		}
		return convs[i].UpdatedAt.After(convs[j].UpdatedAt)
	})

	// Apply cursor: find the first item that is "older" than the cursor.
	start := 0
	if page.Cursor != nil {
		for i, c := range convs {
			if c.UpdatedAt.Before(page.Cursor.CreatedAt) ||
				(c.UpdatedAt.Equal(page.Cursor.CreatedAt) && c.ID < page.Cursor.ID) {
				start = i
				break
			}
			start = len(convs) // nothing matched → empty page
		}
	}

	limit := page.Limit
	if limit <= 0 {
		limit = domain.DefaultPageSize
	}

	end := start + limit
	hasMore := end < len(convs)
	if end > len(convs) {
		end = len(convs)
	}

	items := convs[start:end]
	resp := &domain.PageResponse[*domain.Conversation]{Items: items, HasMore: hasMore}
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		resp.NextCursor = &domain.Cursor{CreatedAt: last.UpdatedAt, ID: last.ID}
	}
	return resp, nil
}

// --- FindDirectChat ---

func (r *ConversationRepo) FindDirectChat(ctx context.Context, userID1, userID2 string) (*domain.Conversation, error) {
	key := directKey(userID1, userID2)

	var convID string
	err := r.session.Query(
		`SELECT conversation_id FROM direct_conversations WHERE lookup_key = ?`, key,
	).WithContext(ctx).Scan(&convID)
	if err == gocql.ErrNotFound {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find direct chat: %w", err)
	}

	return r.FindByID(ctx, convID)
}

// --- Update ---
//
// Full replacement of the conversation row. Also synchronises the participant
// indexes if the participant list changed.

func (r *ConversationRepo) Update(ctx context.Context, conv *domain.Conversation) error {
	// Build the new participant set.
	newSet := make(map[string]struct{}, len(conv.Participants))
	for _, p := range conv.Participants {
		newSet[p.UserID] = struct{}{}
	}

	// Read the old participant set to detect additions and removals.
	oldSet, err := r.getParticipantIDs(ctx, conv.ID)
	if err != nil {
		return err
	}

	// Persist the conversation row.
	participantsJSON := marshalParticipants(conv.Participants)
	lastMsgJSON := marshalLastMessage(conv.LastMessage)

	if err := r.session.Query(`
		UPDATE conversations SET
			type = ?,
			name = ?,
			avatar_url = ?,
			created_by = ?,
			participants = ?,
			last_message = ?,
			only_admins_can_send = ?,
			created_at = ?,
			updated_at = ?
		WHERE id = ?`,
		string(conv.Type), conv.Name, conv.AvatarURL, conv.CreatedBy,
		participantsJSON, lastMsgJSON,
		conv.Settings.OnlyAdminsCanSend,
		conv.CreatedAt, conv.UpdatedAt,
		conv.ID,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("update conversation: %w", err)
	}

	// Sync participant indexes.
	for uid := range newSet {
		if _, exists := oldSet[uid]; !exists {
			if err := r.insertParticipantIndexes(ctx, conv.ID, uid); err != nil {
				return err
			}
		}
	}
	for uid := range oldSet {
		if _, exists := newSet[uid]; !exists {
			if err := r.deleteParticipantIndexes(ctx, conv.ID, uid); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- UpdateLastMessage ---

func (r *ConversationRepo) UpdateLastMessage(ctx context.Context, conversationID string, lastMsg *domain.LastMessage) error {
	return r.session.Query(`
		UPDATE conversations SET last_message = ?, updated_at = ? WHERE id = ?`,
		marshalLastMessage(lastMsg), lastMsg.SentAt, conversationID,
	).WithContext(ctx).Exec()
}

// --- UpdateReadCursor ---

func (r *ConversationRepo) UpdateReadCursor(ctx context.Context, conversationID, userID, messageID string, readAt time.Time) error {
	conv, err := r.FindByID(ctx, conversationID)
	if err != nil {
		return err
	}

	found := false
	for i, p := range conv.Participants {
		if p.UserID == userID {
			conv.Participants[i].LastReadMessageID = messageID
			conv.Participants[i].LastReadAt = readAt
			found = true
			break
		}
	}
	if !found {
		return domain.ErrNotFound
	}

	return r.session.Query(`
		UPDATE conversations SET participants = ? WHERE id = ?`,
		marshalParticipants(conv.Participants), conversationID,
	).WithContext(ctx).Exec()
}

// --- AddParticipant ---

func (r *ConversationRepo) AddParticipant(ctx context.Context, conversationID string, participant domain.Participant) error {
	conv, err := r.FindByID(ctx, conversationID)
	if err != nil {
		return err
	}

	conv.Participants = append(conv.Participants, participant)
	conv.UpdatedAt = time.Now().UTC()

	if err := r.session.Query(`
		UPDATE conversations SET participants = ?, updated_at = ? WHERE id = ?`,
		marshalParticipants(conv.Participants), conv.UpdatedAt, conversationID,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("add participant: %w", err)
	}

	return r.insertParticipantIndexes(ctx, conversationID, participant.UserID)
}

// --- RemoveParticipant ---

func (r *ConversationRepo) RemoveParticipant(ctx context.Context, conversationID, userID string) error {
	conv, err := r.FindByID(ctx, conversationID)
	if err != nil {
		return err
	}

	filtered := conv.Participants[:0]
	for _, p := range conv.Participants {
		if p.UserID != userID {
			filtered = append(filtered, p)
		}
	}
	conv.Participants = filtered
	conv.UpdatedAt = time.Now().UTC()

	if err := r.session.Query(`
		UPDATE conversations SET participants = ?, updated_at = ? WHERE id = ?`,
		marshalParticipants(conv.Participants), conv.UpdatedAt, conversationID,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("remove participant: %w", err)
	}

	return r.deleteParticipantIndexes(ctx, conversationID, userID)
}

// --- buildConversation constructs a domain.Conversation from scanned values ---

func buildConversation(
	id, convType, name, avatarURL, createdBy,
	participantsJSON, lastMsgJSON string,
	onlyAdminsCanSend bool,
	createdAt, updatedAt time.Time,
) *domain.Conversation {
	return &domain.Conversation{
		ID:           id,
		Type:         domain.ConversationType(convType),
		Name:         name,
		AvatarURL:    avatarURL,
		CreatedBy:    createdBy,
		Participants: unmarshalParticipants(participantsJSON),
		LastMessage:  unmarshalLastMessage(lastMsgJSON),
		Settings:     domain.ConversationSettings{OnlyAdminsCanSend: onlyAdminsCanSend},
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
}
