// Package middleware 提供 Gin 中间件：request id、访问日志、Recovery、管理员会话 Token 与项目 Token。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

const requestIDHeader = "X-Request-Id"

type ctxKey string

const requestIDCtxKey ctxKey = "request_id"

// RequestID 保证每个请求有稳定 id，写入 header 与 context。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		c.Set(string(requestIDCtxKey), id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

// AccessLog 用 Zerolog 记录方法、路径、状态与耗时。
func AccessLog(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		rid, _ := c.Get(string(requestIDCtxKey))
		log.Info().
			Interface("request_id", rid).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("elapsed", time.Since(start)).
			Msg("http_request")
	}
}

// Recovery 将 panic 转为 JSON 错误，禁止返回 HTML。
func Recovery(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				rid, _ := c.Get(string(requestIDCtxKey))
				log.Error().Interface("panic", rec).Interface("request_id", rid).Msg("request panic")
				response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
				c.Abort()
			}
		}()
		c.Next()
	}
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b[:])
}
