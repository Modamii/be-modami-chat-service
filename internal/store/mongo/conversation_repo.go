package mongo

import (
	"context"
	"fmt"
	"time"

	"be-modami-chat-service/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ConversationRepo implements service.ConversationRepository using MongoDB.
type ConversationRepo struct {
	coll *mongo.Collection
}

// NewConversationRepo creates a new MongoDB conversation repository.
func NewConversationRepo(db *mongo.Database) *ConversationRepo {
	return &ConversationRepo{coll: db.Collection(conversationsCollection)}
}

func (r *ConversationRepo) Create(ctx context.Context, conv *domain.Conversation) error {
	doc := conversationFromDomain(conv)
	_, err := r.coll.InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrDuplicateConversation
		}
		return fmt.Errorf("insert conversation: %w", err)
	}
	return nil
}

func (r *ConversationRepo) FindByID(ctx context.Context, id string) (*domain.Conversation, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	var doc conversationDoc
	if err := r.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("find conversation: %w", err)
	}
	return conversationToDomain(&doc), nil
}

func (r *ConversationRepo) FindByUser(ctx context.Context, userID string, page domain.PageRequest) (*domain.PageResponse[*domain.Conversation], error) {
	filter := bson.M{"participants.user_id": userID}

	if page.Cursor != nil {
		cursorOID, err := bson.ObjectIDFromHex(page.Cursor.ID)
		if err == nil {
			filter["$or"] = bson.A{
				bson.M{"updated_at": bson.M{"$lt": page.Cursor.CreatedAt}},
				bson.M{
					"updated_at": page.Cursor.CreatedAt,
					"_id":        bson.M{"$lt": cursorOID},
				},
			}
		}
	}

	limit := int64(page.Limit)
	if limit <= 0 {
		limit = int64(domain.DefaultPageSize)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(limit + 1)

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find conversations: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []conversationDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode conversations: %w", err)
	}

	hasMore := len(docs) > int(limit)
	if hasMore {
		docs = docs[:limit]
	}

	items := make([]*domain.Conversation, len(docs))
	for i := range docs {
		items[i] = conversationToDomain(&docs[i])
	}

	resp := &domain.PageResponse[*domain.Conversation]{
		Items:   items,
		HasMore: hasMore,
	}

	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		resp.NextCursor = &domain.Cursor{
			CreatedAt: last.UpdatedAt,
			ID:        last.ID,
		}
	}

	return resp, nil
}

func (r *ConversationRepo) FindDirectChat(ctx context.Context, userID1, userID2 string) (*domain.Conversation, error) {
	// Sort user IDs for consistent lookup
	u1, u2 := domain.DirectChatKey(userID1, userID2)

	filter := bson.M{
		"type": "direct",
		"participants.user_id": bson.M{
			"$all": bson.A{u1, u2},
		},
	}

	var doc conversationDoc
	if err := r.coll.FindOne(ctx, filter).Decode(&doc); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("find direct chat: %w", err)
	}
	return conversationToDomain(&doc), nil
}

func (r *ConversationRepo) Update(ctx context.Context, conv *domain.Conversation) error {
	doc := conversationFromDomain(conv)
	result, err := r.coll.ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc)
	if err != nil {
		return fmt.Errorf("update conversation: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ConversationRepo) UpdateLastMessage(ctx context.Context, conversationID string, lastMsg *domain.LastMessage) error {
	oid, err := bson.ObjectIDFromHex(conversationID)
	if err != nil {
		return domain.ErrNotFound
	}

	update := bson.M{
		"$set": bson.M{
			"last_message": lastMessageDoc{
				MessageID:      mustObjectID(lastMsg.MessageID),
				SenderID:       lastMsg.SenderID,
				ContentPreview: lastMsg.ContentPreview,
				Type:           string(lastMsg.Type),
				SentAt:         lastMsg.SentAt,
			},
			"updated_at": lastMsg.SentAt,
		},
	}

	result, err := r.coll.UpdateByID(ctx, oid, update)
	if err != nil {
		return fmt.Errorf("update last message: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ConversationRepo) UpdateReadCursor(ctx context.Context, conversationID, userID, messageID string, readAt time.Time) error {
	oid, err := bson.ObjectIDFromHex(conversationID)
	if err != nil {
		return domain.ErrNotFound
	}

	result, err := r.coll.UpdateOne(ctx,
		bson.M{
			"_id":                  oid,
			"participants.user_id": userID,
		},
		bson.M{
			"$set": bson.M{
				"participants.$.last_read_message_id": messageID,
				"participants.$.last_read_at":         readAt,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("update read cursor: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ConversationRepo) AddParticipant(ctx context.Context, conversationID string, participant domain.Participant) error {
	oid, err := bson.ObjectIDFromHex(conversationID)
	if err != nil {
		return domain.ErrNotFound
	}

	doc := participantDoc{
		UserID:   participant.UserID,
		Role:     string(participant.Role),
		JoinedAt: participant.JoinedAt,
	}

	result, err := r.coll.UpdateByID(ctx, oid, bson.M{
		"$push": bson.M{"participants": doc},
		"$set":  bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		return fmt.Errorf("add participant: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ConversationRepo) RemoveParticipant(ctx context.Context, conversationID, userID string) error {
	oid, err := bson.ObjectIDFromHex(conversationID)
	if err != nil {
		return domain.ErrNotFound
	}

	result, err := r.coll.UpdateByID(ctx, oid, bson.M{
		"$pull": bson.M{"participants": bson.M{"user_id": userID}},
		"$set":  bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		return fmt.Errorf("remove participant: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
