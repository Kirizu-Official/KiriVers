package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisStore 使用 go-redis/v9 单实例客户端。GET nil 视为 miss。
type redisStore struct {
	client *redis.Client
}

func newRedisStore(opts Options) *redisStore {
	dial := optionDuration(opts.DialTimeout, defaultDialTimeout)
	op := optionDuration(opts.DialTimeout, defaultDialTimeout)
	return &redisStore{
		client: redis.NewClient(&redis.Options{
			Addr:         opts.RedisAddr,
			Password:     opts.RedisPassword,
			DB:           opts.RedisDB,
			DialTimeout:  dial,
			ReadTimeout:  op,
			WriteTimeout: op,
			PoolTimeout:  op,
			MaxRetries:   1,
		}),
	}
}

func (r *redisStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return cloneBytes(val), true, nil
}

func (r *redisStore) Set(ctx context.Context, key string, value []byte) error {
	return r.client.Set(ctx, key, value, 0).Err()
}

func (r *redisStore) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *redisStore) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if ttl < 0 {
		ttl = 0
	}
	return r.client.SetNX(ctx, key, value, ttl).Result()
}

func (r *redisStore) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

func (r *redisStore) DeletePrefix(ctx context.Context, prefix string) error {
	if prefix == "" {
		return nil
	}
	var cursor uint64
	for {
		keys, next, err := r.client.Scan(ctx, cursor, prefix+"*", 64).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (r *redisStore) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *redisStore) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}

func pingRedis(ctx context.Context, r *redisStore, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultPingTimeout
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return r.Ping(pingCtx)
}
