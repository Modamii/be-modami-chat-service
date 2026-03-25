package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	messagesCollection     = "messages"
	conversationsCollection = "conversations"
)

// EnsureIndexes creates required indexes for the chat collections.
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	// Messages indexes
	msgColl := db.Collection(messagesCollection)
	msgIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "conversation_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "conversation_id", Value: 1},
				{Key: "_id", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "sender_id", Value: 1}},
		},
	}
	if _, err := msgColl.Indexes().CreateMany(ctx, msgIndexes); err != nil {
		return fmt.Errorf("create message indexes: %w", err)
	}

	// Conversations indexes
	convColl := db.Collection(conversationsCollection)
	convIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "participants.user_id", Value: 1}},
		},
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
				{Key: "participants.user_id", Value: 1},
			},
		},
		{
			Keys: bson.D{{Key: "updated_at", Value: -1}},
		},
		// Unique index for direct conversations to prevent duplicates.
		// Uses a partial filter on type="direct".
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
				{Key: "participants.user_id", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "type", Value: "direct"}}),
		},
	}
	if _, err := convColl.Indexes().CreateMany(ctx, convIndexes); err != nil {
		return fmt.Errorf("create conversation indexes: %w", err)
	}

	return nil
}
