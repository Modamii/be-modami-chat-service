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

// MessageRepo implements service.MessageRepository using MongoDB.
type MessageRepo struct {
	coll *mongo.Collection
}

// NewMessageRepo creates a new MongoDB message repository.
func NewMessageRepo(db *mongo.Database) *MessageRepo {
	return &MessageRepo{coll: db.Collection(messagesCollection)}
}

func (r *MessageRepo) Create(ctx context.Context, msg *domain.Message) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	doc := messageFromDomain(msg)
	_, err := r.coll.InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrDuplicateMessage
		}
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (r *MessageRepo) FindByID(ctx context.Context, id string) (*domain.Message, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	var doc messageDoc
	if err := r.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("find message: %w", err)
	}
	return messageToDomain(&doc), nil
}

func (r *MessageRepo) FindByConversation(ctx context.Context, conversationID string, page domain.PageRequest) (*domain.PageResponse[*domain.Message], error) {
	convOID, err := bson.ObjectIDFromHex(conversationID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	filter := bson.M{"conversation_id": convOID}

	// Cursor-based pagination: fetch messages older than the cursor
	if page.Cursor != nil {
		cursorOID, err := bson.ObjectIDFromHex(page.Cursor.ID)
		if err == nil {
			filter["$or"] = bson.A{
				bson.M{"created_at": bson.M{"$lt": page.Cursor.CreatedAt}},
				bson.M{
					"created_at": page.Cursor.CreatedAt,
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
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(limit + 1) // fetch one extra to determine HasMore

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find messages: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []messageDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode messages: %w", err)
	}

	hasMore := len(docs) > int(limit)
	if hasMore {
		docs = docs[:limit]
	}

	items := make([]*domain.Message, len(docs))
	for i := range docs {
		items[i] = messageToDomain(&docs[i])
	}

	resp := &domain.PageResponse[*domain.Message]{
		Items:   items,
		HasMore: hasMore,
	}

	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		resp.NextCursor = &domain.Cursor{
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		}
	}

	return resp, nil
}

func (r *MessageRepo) Update(ctx context.Context, msg *domain.Message) error {
	doc := messageFromDomain(msg)
	result, err := r.coll.ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc)
	if err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MessageRepo) AddReaction(ctx context.Context, messageID, emoji, userID string) error {
	oid, err := bson.ObjectIDFromHex(messageID)
	if err != nil {
		return domain.ErrNotFound
	}

	// Try to add user to existing reaction
	result, err := r.coll.UpdateOne(ctx,
		bson.M{
			"_id":             oid,
			"reactions.emoji": emoji,
		},
		bson.M{
			"$addToSet": bson.M{"reactions.$.user_ids": userID},
		},
	)
	if err != nil {
		return fmt.Errorf("add reaction: %w", err)
	}

	// If no existing reaction with this emoji, push a new one
	if result.MatchedCount == 0 {
		_, err = r.coll.UpdateOne(ctx,
			bson.M{"_id": oid},
			bson.M{
				"$push": bson.M{
					"reactions": reactionDoc{
						Emoji:   emoji,
						UserIDs: []string{userID},
					},
				},
			},
		)
		if err != nil {
			return fmt.Errorf("push new reaction: %w", err)
		}
	}

	return nil
}

func (r *MessageRepo) RemoveReaction(ctx context.Context, messageID, emoji, userID string) error {
	oid, err := bson.ObjectIDFromHex(messageID)
	if err != nil {
		return domain.ErrNotFound
	}

	// Pull user from the reaction's user_ids
	_, err = r.coll.UpdateOne(ctx,
		bson.M{
			"_id":             oid,
			"reactions.emoji": emoji,
		},
		bson.M{
			"$pull": bson.M{"reactions.$.user_ids": userID},
		},
	)
	if err != nil {
		return fmt.Errorf("remove reaction: %w", err)
	}

	// Clean up empty reactions (where user_ids is empty)
	_, err = r.coll.UpdateOne(ctx,
		bson.M{"_id": oid},
		bson.M{
			"$pull": bson.M{
				"reactions": bson.M{"user_ids": bson.M{"$size": 0}},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("cleanup empty reaction: %w", err)
	}

	return nil
}
