package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// DBUnavailableNotFound 在数据库运行中不可用时，把 /api 业务路径变成 JSON 404 NOT_FOUND，
// 与启动期未挂业务路由时的 NoRoute 语义一致。/health、/openapi.json 与非 /api
// 路径（管理台静态资源）放行。available 为 nil 视为可用（测试装配）。
func DBUnavailableNotFound(available func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if available == nil || available() {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if dbProbeExempt(path) || !strings.HasPrefix(path, "/api") {
			c.Next()
			return
		}
		response.NotFound(c)
		c.Abort()
	}
}

func dbProbeExempt(path string) bool {
	switch path {
	case "/api/v1/health", "/api/v1/openapi.json":
		return true
	default:
		return false
	}
}
