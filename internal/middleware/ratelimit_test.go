package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// TestLimiterAllowWindow 滑动窗口基础语义：限额内放行；超限拒绝并给出
// Retry-After（等待到窗口最旧命中离开）；窗口滑过后恢复。
func TestLimiterAllowWindow(t *testing.T) {
	l := NewLimiter()
	base := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	// 每 10s 一次请求，3/min 配额：第 4 次（t=30s，窗口内已有 t=0,10,20）应拒绝。
	for i := 0; i < 3; i++ {
		retry, ok := l.Allow("k", 3, base.Add(time.Duration(i*10)*time.Second))
		if !ok || retry != 0 {
			t.Fatalf("hit %d should pass, got ok=%v retry=%v", i, ok, retry)
		}
	}
	retry, ok := l.Allow("k", 3, base.Add(30*time.Second))
	if ok {
		t.Fatal("4th hit within window must be rejected")
	}
	// 最旧命中 t=0 离开窗口的时刻是 t=60s → retry = 30s。
	if retry != 30*time.Second {
		t.Fatalf("retryAfter = %v, want 30s", retry)
	}
	// t=60s+ε：t=0 已滑出，放行。
	if _, ok := l.Allow("k", 3, base.Add(61*time.Second)); !ok {
		t.Fatal("window must slide and admit again")
	}
}

// TestLimiterZeroMeansUnlimited 0 = 显式关闭（不限）；负值同样放行（回退
// 默认由 RateLimitFor 负责）。
func TestLimiterZeroMeansUnlimited(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	for i := 0; i < 1000; i++ {
		if _, ok := l.Allow("off", 0, now); !ok {
			t.Fatal("perMinute=0 must be unlimited")
		}
		if _, ok := l.Allow("neg", -5, now); !ok {
			t.Fatal("perMinute<0 must not reject inside Allow")
		}
	}
}

// TestLimiterKeysDistinct CI Token 键与匿名 IP 键 / 设备键命名空间互不相同
// （验收：CI Token 限流键与匿名 IP 限流键不同）。
func TestLimiterKeysDistinct(t *testing.T) {
	ip := RateLimitIPKey("1.2.3.4")
	ci := RateLimitCIKey("1.2.3.4") // 即便指纹字符串与 IP 相同，前缀也不同
	dev := RateLimitDeviceKey("demo", "abc")
	if ip == ci || ip == dev || ci == dev {
		t.Fatalf("key namespaces must differ: %q %q %q", ip, ci, dev)
	}
	// 同一 Limiter 实例上 CI 配额打满不影响匿名 IP 维度独立计数。
	l := NewLimiter()
	now := time.Now()
	for i := 0; i < 2; i++ {
		if _, ok := l.Allow(RateLimitCIKey("fp"), 2, now); !ok {
			t.Fatal("ci key should admit")
		}
	}
	if _, ok := l.Allow(RateLimitCIKey("fp"), 2, now); ok {
		t.Fatal("ci key should be exhausted")
	}
	if _, ok := l.Allow(RateLimitIPKey("1.2.3.4"), 2, now); !ok {
		t.Fatal("exhausted ci key must not affect the anonymous ip key")
	}
}

// TestLimiterConcurrent 并发安全（-race）：多 goroutine 高频打同一分片与
// 不同分片，总放行数不得超过 配额×键数。
func TestLimiterConcurrent(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	const perKey = 50
	const keys = 64
	var mu sync.Mutex
	admitted := map[string]int{}
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := RateLimitIPKey(strconv.Itoa((g*100+i)%keys))
				if _, ok := l.Allow(key, perKey, now); ok {
					mu.Lock()
					admitted[key]++
					mu.Unlock()
				}
			}
		}(g)
	}
	wg.Wait()
	if len(admitted) > keys {
		t.Fatalf("unexpected keys: %d", len(admitted))
	}
	for k, n := range admitted {
		if n > perKey {
			t.Fatalf("key %s admitted %d > %d", k, n, perKey)
		}
	}
}

// TestRateLimitFor 回退与关闭语义：缺键回退默认；0 显式关闭；负值/非法回退默认。
func TestRateLimitFor(t *testing.T) {
	def := func(k string) int {
		n, _ := rateLimitIntValue(model.DefaultRateLimit()[k])
		return n
	}
	// 无项目 → 默认。
	if got := RateLimitFor(nil, model.RateLimitKeyCheckPerDevice); got != def(model.RateLimitKeyCheckPerDevice) {
		t.Fatalf("nil project: got %d", got)
	}
	// 空袋 → 默认。
	p := &model.Project{RateLimit: model.JSONObject{}}
	if got := RateLimitFor(p, model.RateLimitKeyDiffPerIP); got != def(model.RateLimitKeyDiffPerIP) {
		t.Fatalf("empty bag: got %d", got)
	}
	// 0 = 显式关闭。
	p.RateLimit = model.JSONObject{model.RateLimitKeyCheckPerDevice: 0}
	if got := RateLimitFor(p, model.RateLimitKeyCheckPerDevice); got != 0 {
		t.Fatalf("explicit 0: got %d, want 0 (unlimited)", got)
	}
	// 负值回退默认。
	p.RateLimit = model.JSONObject{model.RateLimitKeyCheckPerDevice: -3}
	if got := RateLimitFor(p, model.RateLimitKeyCheckPerDevice); got != def(model.RateLimitKeyCheckPerDevice) {
		t.Fatalf("negative: got %d, want default", got)
	}
	// 非法类型回退默认。
	p.RateLimit = model.JSONObject{model.RateLimitKeyCheckPerDevice: "abc"}
	if got := RateLimitFor(p, model.RateLimitKeyCheckPerDevice); got != def(model.RateLimitKeyCheckPerDevice) {
		t.Fatalf("invalid type: got %d, want default", got)
	}
	// jsonb 反序列化出的 float64 可读。
	p.RateLimit = model.JSONObject{model.RateLimitKeyCheckPerDevice: json.Number("25")}
	if got := RateLimitFor(p, model.RateLimitKeyCheckPerDevice); got != 25 {
		t.Fatalf("json.Number: got %d, want 25", got)
	}
	// 未知键默认不限。
	if got := RateLimitFor(p, "unknown_key"); got != 0 {
		t.Fatalf("unknown key: got %d, want 0", got)
	}
}

// TestIPRateLimitMiddleware IP 中间件：超限 429 + RATE_LIMITED + Retry-After
// + private no-store；配额内放行。
func TestIPRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := NewLimiter()
	p := &model.Project{Slug: "demo", RateLimit: model.JSONObject{model.RateLimitKeyStorePerIP: 1}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(ContextProject, p); c.Next() })
	r.GET("/x", IPRateLimit(l, model.RateLimitKeyStorePerIP), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	do := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		return w
	}
	if w := do(); w.Code != http.StatusOK {
		t.Fatalf("first request: %d", w.Code)
	}
	w := do()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Fatal("missing Retry-After header")
	} else if n, err := strconv.Atoi(got); err != nil || n < 1 {
		t.Fatalf("Retry-After = %q, want integer >= 1", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code != CodeRateLimited {
		t.Fatalf("error code = %q err=%v", body.Error.Code, err)
	}
}
