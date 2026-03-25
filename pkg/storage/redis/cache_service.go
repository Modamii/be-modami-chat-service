package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedisCacheService(client *redis.Client) RedisCacheService {
	return &RedisClient{
		client: client,
	}
}

func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	var data []byte
	var err error
	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, err = json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
	}
	return r.client.Set(ctx, key, data, expiration).Err()
}

func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return val, err
}

func (r *RedisClient) GetObject(ctx context.Context, key string, dest interface{}) error {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return fmt.Errorf("key not found: %s", key)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}

func (r *RedisClient) Delete(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (r *RedisClient) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	pipe := r.client.TxPipeline()
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	status := pipe.JSONSet(ctx, key, "$", data)
	if status.Err() != nil {
		return fmt.Errorf("failed to set JSON: %w", status.Err())
	}
	statusExpire := pipe.Expire(ctx, key, expiration)
	if statusExpire.Err() != nil {
		return fmt.Errorf("failed to expire key: %w", statusExpire.Err())
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute pipeline: %w", err)
	}
	return nil
}

func (r *RedisClient) GetJSON(ctx context.Context, key string, dest interface{}) error {
	data, err := r.client.JSONGet(ctx, key, "$").Result()
	if err == redis.Nil {
		return err
	}
	if err != nil {
		return err
	}
	rawData := []byte(data)
	if len(rawData) > 0 && rawData[0] == '[' {
		var arrayResult []json.RawMessage
		if err := json.Unmarshal(rawData, &arrayResult); err == nil && len(arrayResult) > 0 {
			rawData = arrayResult[0]
		}
	}
	return json.Unmarshal(rawData, dest)
}

func (r *RedisClient) MSetJSON(ctx context.Context, pairs map[string]interface{}, expiration time.Duration) error {
	pipe := r.client.Pipeline()
	for key, value := range pairs {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
		}
		pipe.Set(ctx, key, data, expiration)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisClient) MGetJSON(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string)
	pipe := r.client.Pipeline()
	for _, key := range keys {
		pipe.Get(ctx, key)
	}
	cmds, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}
	for i, cmd := range cmds {
		if cmd.Err() == nil {
			if strCmd, ok := cmd.(*redis.StringCmd); ok {
				val, err := strCmd.Result()
				if err == nil {
					result[keys[i]] = val
				}
			}
		}
	}
	return result, nil
}

func (r *RedisClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	return r.client.LPush(ctx, key, values...).Err()
}

func (r *RedisClient) RPush(ctx context.Context, key string, values ...interface{}) error {
	return r.client.RPush(ctx, key, values...).Err()
}

func (r *RedisClient) LPop(ctx context.Context, key string) (string, error) {
	return r.client.LPop(ctx, key).Result()
}

func (r *RedisClient) RPop(ctx context.Context, key string) (string, error) {
	return r.client.RPop(ctx, key).Result()
}

func (r *RedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(ctx, key, start, stop).Result()
}

func (r *RedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return r.client.SAdd(ctx, key, members...).Err()
}

func (r *RedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.client.SMembers(ctx, key).Result()
}

func (r *RedisClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	return r.client.SRem(ctx, key, members...).Err()
}

func (r *RedisClient) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return r.client.SIsMember(ctx, key, member).Result()
}

func (r *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	return r.client.HSet(ctx, key, values...).Err()
}

func (r *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	return r.client.HGet(ctx, key, field).Result()
}

func (r *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

func (r *RedisClient) HDel(ctx context.Context, key string, fields ...string) error {
	return r.client.HDel(ctx, key, fields...).Err()
}

func (r *RedisClient) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	return r.client.ZAdd(ctx, key, members...).Err()
}

func (r *RedisClient) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.ZRange(ctx, key, start, stop).Result()
}

func (r *RedisClient) ZRangeByScore(ctx context.Context, key string, opt *redis.ZRangeBy) ([]string, error) {
	return r.client.ZRangeByScore(ctx, key, opt).Result()
}

func (r *RedisClient) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return r.client.ZRem(ctx, key, members...).Err()
}

func (r *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.client.Expire(ctx, key, expiration).Err()
}

func (r *RedisClient) ExpireAt(ctx context.Context, key string, tm time.Time) error {
	return r.client.ExpireAt(ctx, key, tm).Err()
}

func (r *RedisClient) PExpire(ctx context.Context, key string, milliseconds int64) error {
	return r.client.PExpire(ctx, key, time.Duration(milliseconds)*time.Millisecond).Err()
}

func (r *RedisClient) PExpireAt(ctx context.Context, key string, tm time.Time) error {
	return r.client.PExpireAt(ctx, key, tm).Err()
}

func (r *RedisClient) Persist(ctx context.Context, key string) error {
	return r.client.Persist(ctx, key).Err()
}

func (r *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, key).Result()
}

func (r *RedisClient) PTTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.PTTL(ctx, key).Result()
}

func (r *RedisClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	return r.client.Keys(ctx, pattern).Result()
}

func (r *RedisClient) Pipeline() redis.Pipeliner {
	return r.client.Pipeline()
}

func (r *RedisClient) TxPipeline() redis.Pipeliner {
	return r.client.TxPipeline()
}

func (r *RedisClient) JSONIncrementField(ctx context.Context, key, path string, value int64) error {
	cmd := r.client.Do(ctx, "JSON.NUMINCRBY", key, path, value)
	if cmd.Err() != nil {
		return fmt.Errorf("failed to increment JSON field %s in key %s: %w", path, key, cmd.Err())
	}
	return nil
}

func (r *RedisClient) JSONIncrementMultipleFields(ctx context.Context, key string, increments map[string]int64) error {
	pipe := r.client.TxPipeline()
	for path, value := range increments {
		pipe.Do(ctx, "JSON.NUMINCRBY", key, path, value)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to increment multiple JSON fields in key %s: %w", key, err)
	}
	return nil
}
