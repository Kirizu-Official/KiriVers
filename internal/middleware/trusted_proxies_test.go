package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestApplyTrustedProxiesInvalidCIDR(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := ApplyTrustedProxies(engine, []string{"not-a-cidr"}); err == nil {
		t.Fatal("invalid CIDR must return error")
	}
}

func TestApplyTrustedProxiesEmptyDoesNotTrustXFF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Load() 规范化后是空切片而不是 nil；两者都必须 SetTrustedProxies(nil)。
	for _, proxies := range [][]string{nil, {}} {
		engine := gin.New()
		if err := ApplyTrustedProxies(engine, proxies); err != nil {
			t.Fatal(err)
		}
		engine.GET("/ip", func(c *gin.Context) {
			c.String(http.StatusOK, c.ClientIP())
		})

		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if got := w.Body.String(); got != "192.0.2.1" {
			t.Fatalf("empty trust list must use RemoteAddr host, proxies=%v got %q", proxies, got)
		}
	}
}

func TestApplyTrustedProxiesTrustsXFFWhenListed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := ApplyTrustedProxies(engine, []string{"192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	engine.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 192.0.2.1")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if got := w.Body.String(); got != "203.0.113.9" {
		t.Fatalf("trusted proxy must use leftmost XFF, got %q", got)
	}
}
