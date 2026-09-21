// Package response 是唯一 HTTP JSON 出口。
// 错误体形状固定为 { "error": { "code", "message", "details" } }，与产品 PRD §12.2 一致。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 是错误 JSON 的外层。
type Body struct {
	Error Detail `json:"error"`
}

// Detail 是错误码与可读信息。
type Detail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// JSON 写入成功响应。字段须为 snake_case 的 struct tag。
func JSON(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

// Error 写入统一错误包。Handler 不得自行拼 error JSON。
func Error(c *gin.Context, status int, code, message string, details any) {
	if c.Writer.Written() {
		return
	}
	c.JSON(status, Body{
		Error: Detail{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

// NotFound 用于未知路由，避免 Gin 默认 HTML。
func NotFound(c *gin.Context) {
	Error(c, http.StatusNotFound, "NOT_FOUND", "not found", nil)
}
