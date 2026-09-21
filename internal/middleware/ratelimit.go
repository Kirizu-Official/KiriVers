package middleware

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// 限流执行器（docs/app-init.md §14 / C11-5, C11-6）。
//
// 语义：
//   - 进程内滑动窗口（60s），多副本时配额按实例计——§14 无状态自托管语义允许；
//   - 256 分片互斥，降低高并发下单锁竞争（窗口为时间戳环形裁剪）；
//   - HTTP 304 仍计数：限流判定位于 handler 业务（含 304 短路）之前；
//   - 429 → RATE_LIMITED + Retry-After（整秒）+ private, no-store；
//   - 管理 API 不限；CI 上传/发版按 `ci:{tokenFingerprint}` 键，与匿名
//     `ip:{addr}` 键天然不同命名空间（C11-6 验收项）。

const (
	// RateLimitWindowSeconds 滑动窗口宽度（秒）。
	RateLimitWindowSeconds = 60
	// RateLimitShardCount Limiter 分片数：按 key 哈希分散锁竞争。
	RateLimitShardCount = 256
	// CodeRateLimited 429 错误码（§12.2）。
	CodeRateLimited = "RATE_LIMITED"
)

// rateLimitWindow 是窗口宽度（纳秒）。
const rateLimitWindow = RateLimitWindowSeconds * time.Second

// limiterWindow 是单个键的滑动窗口：窗口内的请求时间戳（UnixNano 升序）。
type limiterWindow struct {
	hits []int64
}

// limiterShard 持有一组键窗口与专属锁；shard 由 key 哈希选择。
type limiterShard struct {
	mu      sync.Mutex
	windows map[string]*limiterWindow
}

// Limiter 是进程内滑动窗口限流器；main 装配单例，handler 内双维度检查
// 与路由 IP 中间件共用同一实例。
type Limiter struct {
	shards [RateLimitShardCount]limiterShard
}

// NewLimiter 构造限流器。
func NewLimiter() *Limiter {
	l := &Limiter{}
	for i := range l.shards {
		l.shards[i].windows = make(map[string]*limiterWindow)
	}
	return l
}

// Allow 判定 key 在当前滑动窗口内是否还允许一次请求。
//
// perMinute 语义：>0 为每分钟允许次数；<=0 一律放行（0 = 显式关闭该维度，
// 回退默认由 RateLimitFor 处理）。超限返回 (retryAfter, false)，retryAfter
// 为窗口最旧一次请求离开 60s 窗口所需的等待时间。
func (l *Limiter) Allow(key string, perMinute int, now time.Time) (time.Duration, bool) {
	if perMinute <= 0 {
		return 0, true
	}
	shard := &l.shards[shardIndex(key)]
	nowNano := now.UnixNano()
	cutoff := nowNano - int64(rateLimitWindow)

	shard.mu.Lock()
	defer shard.mu.Unlock()
	w := shard.windows[key]
	if w == nil {
		w = &limiterWindow{}
		shard.windows[key] = w
	}
	// 裁剪窗口外（≤ cutoff）的旧命中。
	for len(w.hits) > 0 && w.hits[0] <= cutoff {
		w.hits = w.hits[1:]
	}
	if len(w.hits) >= perMinute {
		// retryAfter = 最旧命中 + 窗口 - now（整秒化在响应出口做）。
		retry := time.Duration(w.hits[0]+int64(rateLimitWindow)-nowNano) * time.Nanosecond
		if retry < 0 {
			retry = 0
		}
		return retry, false
	}
	w.hits = append(w.hits, nowNano)
	return 0, true
}

// shardIndex 用 FNV-1a 把 key 分散到 256 个分片。
func shardIndex(key string) int {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return int(h & (RateLimitShardCount - 1))
}

// ---------- 限流键构造（键命名空间：dev: / ip: / ci: 互不相同） ----------

// RateLimitIPKey 匿名源 IP 维度的限流键。
func RateLimitIPKey(addr string) string { return "ip:" + addr }

// RateLimitDeviceKey 项目内设备（哈希后）维度的限流键。deviceHash 恒为
// HMAC/明文策略结果，原始 device_id 不进入键、更不进入日志。
func RateLimitDeviceKey(projectSlug, deviceHash string) string {
	return "dev:" + projectSlug + ":" + deviceHash
}

// RateLimitCIKey CI Token 维度的限流键；与 ip:/dev: 命名空间天然不同
//（C11-6 验收项：CI Token 限流键与匿名 IP 限流键不同）。
func RateLimitCIKey(tokenFingerprint string) string { return "ci:" + tokenFingerprint }

// ---------- 项目配置读取 ----------

// RateLimitFor 读取项目 RateLimit jsonb 中 key 的生效值：
//   - 项目显式配置且合法：直接生效；0 = 显式关闭该维度（不限）；
//   - 缺键 / 负值 / 非法类型：回退文档默认（model.DefaultRateLimit）。
//
// project 为 nil（项目未解析的防御路径）时返回默认值。
func RateLimitFor(project *model.Project, key string) int {
	fallback := defaultRateLimitValue(key)
	if project == nil || project.RateLimit == nil {
		return fallback
	}
	raw, ok := project.RateLimit[key]
	if !ok {
		return fallback
	}
	v, ok := rateLimitIntValue(raw)
	if !ok || v < 0 {
		return fallback
	}
	return v
}

// defaultRateLimitValue 返回 key 的文档默认值；未知 key 返回 0（不限）。
func defaultRateLimitValue(key string) int {
	for k, v := range model.DefaultRateLimit() {
		if k == key {
			if n, ok := rateLimitIntValue(v); ok {
				return n
			}
		}
	}
	return 0
}

// rateLimitIntValue 把 jsonb 反序列化出的 any（float64 / int / json.Number /
// string）转为非负整数；非法返回 false。
func rateLimitIntValue(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// ---------- Gin 接入 ----------

// AllowKeyOrAbort 检查单个限流键；超限时写出 429 RATE_LIMITED 响应并返回
// false（handler 直接 return，中间件配合 c.Abort）。限流判定必须发生在任何
// 304 短路之前——HTTP 304 仍计入限额（§14）。
func AllowKeyOrAbort(c *gin.Context, l *Limiter, key string, perMinute int) bool {
	if l == nil {
		return true
	}
	retryAfter, ok := l.Allow(key, perMinute, time.Now())
	if ok {
		return true
	}
	WriteRateLimited(c, retryAfter)
	c.Abort()
	return false
}

// WriteRateLimited 写出统一的 429 响应：RATE_LIMITED + Retry-After（整秒，
// 至少 1）+ private, no-store（§14 / §13.12）。
func WriteRateLimited(c *gin.Context, retryAfter time.Duration) {
	c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
	c.Header("Cache-Control", "private, no-store")
	response.Error(c, http.StatusTooManyRequests, CodeRateLimited, "rate limit exceeded", nil)
}

// retryAfterSeconds 把重试等待取整为至少 1 的整秒数（Retry-After 头要求整秒）。
func retryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	return int(math.Ceil(d.Seconds()))
}

// IPRateLimit 是纯 IP 维度的限流中间件（integrity / changelog / 公开 feed 等
// 无 body 身份的路由）：key = ip:{addr}，配额读项目 store_per_ip_per_minute。
// 必须挂在 ClientProjectAuth 之后（需要 ContextProject 读取项目配额）。
func IPRateLimit(limiter *Limiter, rateKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil {
			c.Next()
			return
		}
		perMinute := RateLimitFor(ProjectFrom(c), rateKey)
		if AllowKeyOrAbort(c, limiter, RateLimitIPKey(c.ClientIP()), perMinute) {
			c.Next()
		}
	}
}

// ContextCITokenFingerprint 是 ProjectAccess 命中 CI Token 时写入的指纹
//（明文前 8 字符，审计与限流共用，不含完整密钥）。
const ContextCITokenFingerprint = "ci_token_fingerprint"

// CIRateLimit 是 CI Token 维度的限流中间件（挂 CI 上传/发版路由）：
//   - 命中 CI Token（context 有指纹）→ key = ci:{fingerprint}，
//     配额读项目 ci_per_token_per_minute；
//   - 实例管理员 / 项目 Token → 不限（C11-6：管理配额与公开/CI 配额分离）。
func CIRateLimit(limiter *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		fp := c.GetString(ContextCITokenFingerprint)
		if limiter == nil || fp == "" {
			c.Next()
			return
		}
		perMinute := RateLimitFor(ProjectFrom(c), model.RateLimitKeyCIToken)
		if AllowKeyOrAbort(c, limiter, RateLimitCIKey(fp), perMinute) {
			c.Next()
		}
	}
}
