package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCacheService interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	GetObject(ctx context.Context, key string, dest interface{}) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	GetJSON(ctx context.Context, key string, dest interface{}) error
	MSetJSON(ctx context.Context, pairs map[string]interface{}, expiration time.Duration) error
	MGetJSON(ctx context.Context, keys []string) (map[string]string, error)
	LPush(ctx context.Context, key string, values ...interface{}) error
	RPush(ctx context.Context, key string, values ...interface{}) error
	LPop(ctx context.Context, key string) (string, error)
	RPop(ctx context.Context, key string) (string, error)
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	SAdd(ctx context.Context, key string, members ...interface{}) error
	SMembers(ctx context.Context, key string) ([]string, error)
	SRem(ctx context.Context, key string, members ...interface{}) error
	SIsMember(ctx context.Context, key string, member interface{}) (bool, error)
	HSet(ctx context.Context, key string, values ...interface{}) error
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error
	ZAdd(ctx context.Context, key string, members ...redis.Z) error
	ZRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	ZRangeByScore(ctx context.Context, key string, opt *redis.ZRangeBy) ([]string, error)
	ZRem(ctx context.Context, key string, members ...interface{}) error
	Expire(ctx context.Context, key string, expiration time.Duration) error
	ExpireAt(ctx context.Context, key string, tm time.Time) error
	JSONIncrementField(ctx context.Context, key, path string, value int64) error
	JSONIncrementMultipleFields(ctx context.Context, key string, increments map[string]int64) error
	PExpire(ctx context.Context, key string, milliseconds int64) error
	PExpireAt(ctx context.Context, key string, tm time.Time) error
	Persist(ctx context.Context, key string) error
	TTL(ctx context.Context, key string) (time.Duration, error)
	PTTL(ctx context.Context, key string) (time.Duration, error)
	Keys(ctx context.Context, pattern string) ([]string, error)
	Pipeline() redis.Pipeliner
	TxPipeline() redis.Pipeliner
}
