package port

import (
	"context"
)

// CacheStore manages ephemeral cached data.
type CacheStore interface {
	IncrementUnread(ctx context.Context, userID, conversationID string) error
	ClearUnread(ctx context.Context, userID, conversationID string) error
	GetUnreadCounts(ctx context.Context, userID string) (map[string]int64, error)
}

// PresenceStore manages user presence and typing indicators.
type PresenceStore interface {
	SetOnline(ctx context.Context, userID string) error
	SetOffline(ctx context.Context, userID string) error
	IsOnline(ctx context.Context, userID string) (bool, error)
	GetOnlineUsers(ctx context.Context, userIDs []string) ([]string, error)
	SetTyping(ctx context.Context, conversationID, userID string) error
	ClearTyping(ctx context.Context, conversationID, userID string) error
}

// RateLimiter checks and enforces rate limits.
type RateLimiter interface {
	AllowMessage(ctx context.Context, userID string) (bool, error)
}
