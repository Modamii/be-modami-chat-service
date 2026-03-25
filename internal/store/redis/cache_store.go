package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"be-modami-chat-service/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	unreadPrefix   = "chat:unread:"
	convMetaPrefix = "chat:conv_meta:"
	convMetaTTL    = 30 * time.Minute
)

// CacheStore implements service.CacheStore using Redis.
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

func (s *CacheStore) SetConversationMeta(ctx context.Context, conv *domain.Conversation) error {
	key := convMetaPrefix + conv.ID
	data, err := json.Marshal(conv)
	if err != nil {
		return fmt.Errorf("marshal conversation meta: %w", err)
	}
	if err := s.client.Set(ctx, key, data, convMetaTTL).Err(); err != nil {
		return fmt.Errorf("set conversation meta: %w", err)
	}
	return nil
}

func (s *CacheStore) GetConversationMeta(ctx context.Context, conversationID string) (*domain.Conversation, error) {
	key := convMetaPrefix + conversationID
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get conversation meta: %w", err)
	}

	var conv domain.Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, fmt.Errorf("unmarshal conversation meta: %w", err)
	}
	return &conv, nil
}

func (s *CacheStore) InvalidateConversationMeta(ctx context.Context, conversationID string) error {
	key := convMetaPrefix + conversationID
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("invalidate conversation meta: %w", err)
	}
	return nil
}
