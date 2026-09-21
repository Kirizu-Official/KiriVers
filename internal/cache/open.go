package cache

import (
	"context"
	"fmt"
	"strings"
)

// Open 按 driver 构造 Store：memory（默认）或 redis。
// driver=redis 时必须 Ping 成功，否则返回 error（cmd 在 listen 前 fatal）。
// 未知 driver 返回 error。本包不读环境变量。
func Open(opts Options) (Store, error) {
	log := opts.Logger
	switch strings.ToLower(strings.TrimSpace(opts.Driver)) {
	case "", "memory":
		return newMemoryStore(), nil
	case "redis":
		redis := newRedisStore(opts)
		if err := pingRedis(context.Background(), redis, optionDuration(opts.PingTimeout, defaultPingTimeout)); err != nil {
			_ = redis.Close()
			return nil, fmt.Errorf("cache redis ping: %w", err)
		}
		interval := opts.ReconnectInterval
		if interval <= 0 {
			interval = DefaultReconnectInterval
		}
		return newFailoverStore(redis, newMemoryStore(), interval, optionDuration(opts.PingTimeout, defaultPingTimeout), log), nil
	default:
		return nil, fmt.Errorf("unknown cache driver: %s", opts.Driver)
	}
}
