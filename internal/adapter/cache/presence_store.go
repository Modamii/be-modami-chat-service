package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	presenceKey  = "chat:presence"
	typingPrefix = "chat:typing:"
	typingTTL    = 10 * time.Second
	staleCutoff  = 5 * time.Minute
)

// PresenceStore implements port.PresenceStore using Redis sorted sets.
type PresenceStore struct {
	client *redis.Client
}

// NewPresenceStore creates a new Redis presence store.
func NewPresenceStore(client *redis.Client) *PresenceStore {
	return &PresenceStore{client: client}
}

func (s *PresenceStore) SetOnline(ctx context.Context, userID string) error {
	score := float64(time.Now().Unix())
	if err := s.client.ZAdd(ctx, presenceKey, redis.Z{Score: score, Member: userID}).Err(); err != nil {
		return fmt.Errorf("set online: %w", err)
	}
	return nil
}

func (s *PresenceStore) SetOffline(ctx context.Context, userID string) error {
	if err := s.client.ZRem(ctx, presenceKey, userID).Err(); err != nil {
		return fmt.Errorf("set offline: %w", err)
	}
	return nil
}

func (s *PresenceStore) IsOnline(ctx context.Context, userID string) (bool, error) {
	_, err := s.client.ZScore(ctx, presenceKey, userID).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check online: %w", err)
	}
	return true, nil
}

func (s *PresenceStore) GetOnlineUsers(ctx context.Context, userIDs []string) ([]string, error) {
	pipe := s.client.Pipeline()
	cmds := make([]*redis.FloatCmd, len(userIDs))
	for i, uid := range userIDs {
		cmds[i] = pipe.ZScore(ctx, presenceKey, uid)
	}
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("get online users: %w", err)
	}

	online := make([]string, 0)
	for i, cmd := range cmds {
		if cmd.Err() == nil {
			online = append(online, userIDs[i])
		}
	}
	return online, nil
}

func (s *PresenceStore) SetTyping(ctx context.Context, conversationID, userID string) error {
	key := typingPrefix + conversationID
	score := float64(time.Now().Unix())
	if err := s.client.ZAdd(ctx, key, redis.Z{Score: score, Member: userID}).Err(); err != nil {
		return fmt.Errorf("set typing: %w", err)
	}
	s.client.Expire(ctx, key, typingTTL)
	return nil
}

func (s *PresenceStore) ClearTyping(ctx context.Context, conversationID, userID string) error {
	key := typingPrefix + conversationID
	if err := s.client.ZRem(ctx, key, userID).Err(); err != nil {
		return fmt.Errorf("clear typing: %w", err)
	}
	return nil
}

// CleanupStalePresence removes users who haven't refreshed their presence.
func (s *PresenceStore) CleanupStalePresence(ctx context.Context) error {
	cutoff := fmt.Sprintf("%d", time.Now().Add(-staleCutoff).Unix())
	if err := s.client.ZRemRangeByScore(ctx, presenceKey, "-inf", cutoff).Err(); err != nil {
		return fmt.Errorf("cleanup stale presence: %w", err)
	}
	return nil
}
