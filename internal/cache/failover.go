package cache

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// failoverStore 在 driver=redis 时包装 Redis 主后端与内存待机。
// 运行中 Redis 操作错误（非 miss）切到内存，并按间隔 Ping；成功则清空
// kirivers: 前缀、丢弃内存内容、切回 Redis。不改写进程配置。
type failoverStore struct {
	mu        sync.RWMutex
	primary   Store
	redis     *redisStore
	memory    *memoryStore
	failed    bool
	interval  time.Duration
	pingWait  time.Duration
	stop      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
	log       zerolog.Logger
}

func newFailoverStore(redis *redisStore, memory *memoryStore, interval, pingWait time.Duration, log zerolog.Logger) *failoverStore {
	if interval <= 0 {
		interval = DefaultReconnectInterval
	}
	if pingWait <= 0 {
		pingWait = defaultPingTimeout
	}
	f := &failoverStore{
		primary:  redis,
		redis:    redis,
		memory:   memory,
		interval: interval,
		pingWait: pingWait,
		stop:     make(chan struct{}),
		log:      log,
	}
	f.wg.Add(1)
	go f.reconnectLoop()
	return f
}

func (f *failoverStore) current() Store {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.primary
}

func (f *failoverStore) failOver(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failed {
		return
	}
	f.failed = true
	f.primary = f.memory
	f.log.Error().Err(err).Msg("redis cache failed; falling back to memory")
}

func (f *failoverStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	s := f.current()
	val, ok, err := s.Get(ctx, key)
	if err != nil && s == f.redis {
		f.failOver(err)
		return f.memory.Get(ctx, key)
	}
	return val, ok, err
}

func (f *failoverStore) Set(ctx context.Context, key string, value []byte) error {
	s := f.current()
	err := s.Set(ctx, key, value)
	if err != nil && s == f.redis {
		f.failOver(err)
		return f.memory.Set(ctx, key, value)
	}
	return err
}

func (f *failoverStore) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	s := f.current()
	err := s.SetWithTTL(ctx, key, value, ttl)
	if err != nil && s == f.redis {
		f.failOver(err)
		return f.memory.SetWithTTL(ctx, key, value, ttl)
	}
	return err
}

func (f *failoverStore) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	s := f.current()
	ok, err := s.SetNX(ctx, key, value, ttl)
	if err != nil && s == f.redis {
		f.failOver(err)
		return f.memory.SetNX(ctx, key, value, ttl)
	}
	return ok, err
}

func (f *failoverStore) Delete(ctx context.Context, keys ...string) error {
	s := f.current()
	err := s.Delete(ctx, keys...)
	if err != nil && s == f.redis {
		f.failOver(err)
		return f.memory.Delete(ctx, keys...)
	}
	return err
}

func (f *failoverStore) DeletePrefix(ctx context.Context, prefix string) error {
	s := f.current()
	err := s.DeletePrefix(ctx, prefix)
	if err != nil && s == f.redis {
		f.failOver(err)
		return f.memory.DeletePrefix(ctx, prefix)
	}
	return err
}

func (f *failoverStore) Close() error {
	var closeErr error
	f.closeOnce.Do(func() {
		close(f.stop)
		f.wg.Wait()
		closeErr = f.redis.Close()
	})
	return closeErr
}

func (f *failoverStore) reconnectLoop() {
	defer f.wg.Done()
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for {
		select {
		case <-f.stop:
			return
		case <-ticker.C:
			f.tryRecover()
		}
	}
}

func (f *failoverStore) tryRecover() {
	f.mu.RLock()
	failed := f.failed
	f.mu.RUnlock()
	if !failed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.pingWait)
	err := f.redis.Ping(ctx)
	cancel()
	if err != nil {
		return
	}
	flushCtx, flushCancel := context.WithTimeout(context.Background(), f.pingWait)
	err = f.redis.DeletePrefix(flushCtx, KeyPrefix)
	flushCancel()
	if err != nil {
		f.log.Error().Err(err).Msg("redis cache flush failed; staying on memory")
		return
	}
	// Flush 已成功：在同一把锁里清空内存并切回 Redis，避免测试/请求
	// 观测到「前缀已删但仍以内存为主」的窗口。
	f.mu.Lock()
	if f.failed {
		f.memory.clear()
		f.primary = f.redis
		f.failed = false
		f.log.Info().Msg("redis cache reconnected")
	}
	f.mu.Unlock()
}
