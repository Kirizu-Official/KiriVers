// Package cache 提供进程基础设施缓存（内存或 Redis）。
// 业务层只依赖 Store，不得 import Redis SDK；本包不读环境变量、不碰 Gin/GORM。
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// ErrInvalidTTL 表示 SetWithTTL 收到了非正 TTL；目录键应改用无过期的 Set。
var ErrInvalidTTL = errors.New("cache: ttl must be positive")

const (
	// KeyPrefix 是本进程写入 Redis 的统一前缀，降低与共用 DB 的键冲突。
	KeyPrefix = "kirivers:"
	// DefaultReconnectInterval 是 YAML 未给出间隔时 failover 重连周期。
	DefaultReconnectInterval = 10 * time.Minute
	defaultPingTimeout       = 3 * time.Second
	defaultDialTimeout       = 2 * time.Second
)

// Store 是内存与 Redis 后端的共同合同。Get 在未命中时返回 (nil, false, nil)。
// Redis 连接类错误必须作为 error 返回（由 failoverStore 触发回退），不得伪装成 miss。
// Set 写入无过期条目（目录/身份键）。会话等短命键必须用 SetWithTTL。
type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte) error
	SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// SetNX 仅当键不存在（或已过期）时写入；true 表示本调用抢到。ttl<=0 表示不过期。
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	Delete(ctx context.Context, keys ...string) error
	DeletePrefix(ctx context.Context, prefix string) error
	Close() error
}

// Options 由 cmd 从进程配置映射而来（本包不读环境变量）。
type Options struct {
	Driver            string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	ReconnectInterval time.Duration
	PingTimeout       time.Duration
	DialTimeout       time.Duration
	Logger            zerolog.Logger
}

// ProjectIDKey 是按 UUID 缓存的项目身份键。
func ProjectIDKey(id uuid.UUID) string {
	return KeyPrefix + "project:id:" + id.String()
}

// ProjectSlugKey 是按 live slug 缓存的项目身份键。
func ProjectSlugKey(slug string) string {
	return KeyPrefix + "project:slug:" + slug
}

// ProjectAliasKey 是按未过期 alias slug 缓存的项目身份键。
func ProjectAliasKey(slug string) string {
	return KeyPrefix + "project:alias:" + slug
}

// ProjectLookupsKey 记录某项目已填充的身份键列表，供失效时无需 SCAN。
func ProjectLookupsKey(id uuid.UUID) string {
	return KeyPrefix + "project:" + id.String() + ":lookups"
}

// CatalogKey 是某项目在已规范化 (os, arch) 上的目录快照键。
func CatalogKey(projectID uuid.UUID, os, arch string) string {
	return KeyPrefix + "catalog:" + projectID.String() + ":" + os + ":" + arch
}

// CatalogPrefix 是某项目全部目录快照键的前缀（含尾部冒号）。
func CatalogPrefix(projectID uuid.UUID) string {
	return KeyPrefix + "catalog:" + projectID.String() + ":"
}

// PackOccupancyKey 是动态打包 SET NX 占用键（避免两节点同时打同一 fileset）。
func PackOccupancyKey(idempotencyKey string) string {
	return KeyPrefix + "pack:occ:" + idempotencyKey
}

// PackDoneKey 是动态打包完成后的状态键。
func PackDoneKey(idempotencyKey string) string {
	return KeyPrefix + "pack:done:" + idempotencyKey
}

const (
	// AdminKeyPrefix 是管理员会话与受限登录 token 的统一前缀。
	AdminKeyPrefix = KeyPrefix + "admin:"
	adminSessInfix = "sess:"
	adminPendInfix = "pend:"
)

// AdminSessKey 是完整管理会话键。payload 仅含管理员身份，不含 2FA 密钥。
func AdminSessKey(token string) string {
	return AdminKeyPrefix + adminSessInfix + token
}

// AdminPendKey 是登录受限 token 键（强制绑定 / 第二因素）。
func AdminPendKey(token string) string {
	return AdminKeyPrefix + adminPendInfix + token
}

// AdminWebAuthnKey 是完整会话下 WebAuthn 短时 ceremony 键（非 2FA 凭证）。
func AdminWebAuthnKey(adminID uuid.UUID) string {
	return AdminKeyPrefix + "wa:" + adminID.String()
}

// InvalidateProject 删除该项目的身份键、lookups 与全部目录快照。
// store 为 nil 或 id 为零值时为 no-op。
func InvalidateProject(ctx context.Context, store Store, id uuid.UUID) error {
	if store == nil || id == uuid.Nil {
		return nil
	}
	lookupsKey := ProjectLookupsKey(id)
	raw, ok, err := store.Get(ctx, lookupsKey)
	if err != nil {
		return err
	}
	var keys []string
	if ok && len(raw) > 0 {
		_ = json.Unmarshal(raw, &keys)
	}
	keys = append(keys, ProjectIDKey(id), lookupsKey)
	if err := store.Delete(ctx, keys...); err != nil {
		return err
	}
	return store.DeletePrefix(ctx, CatalogPrefix(id))
}

func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func optionDuration(v, fallback time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return fallback
}
