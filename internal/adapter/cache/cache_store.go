package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const (
	unreadPrefix = "chat:unread:"
)

// CacheStore implements port.CacheStore using Redis.
type CacheStore struct {
	client *redis.Client
}

// NewCacheStore creates a new Redis cache store.
func NewCacheStore(client *redis.Client) *CacheStore {
	return &CacheStore{client: client}
}

func (s *CacheStore) IncrementUnread(ctx context.Context, userID, conversationID string) error {
	key := unreadPrefix + userID
	if err := s.client.HIncrBy(ctx, key, conversationID, 1).Err(); err != nil {
		return fmt.Errorf("increment unread: %w", err)
	}
	return nil
}

func (s *CacheStore) ClearUnread(ctx context.Context, userID, conversationID string) error {
	key := unreadPrefix + userID
	if err := s.client.HDel(ctx, key, conversationID).Err(); err != nil {
		return fmt.Errorf("clear unread: %w", err)
	}
	return nil
}

func (s *CacheStore) GetUnreadCounts(ctx context.Context, userID string) (map[string]int64, error) {
	key := unreadPrefix + userID
	result, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get unread counts: %w", err)
	}

	counts := make(map[string]int64, len(result))
	for convID, val := range result {
		var count int64
		fmt.Sscanf(val, "%d", &count)
		counts[convID] = count
	}
	return counts, nil
}
