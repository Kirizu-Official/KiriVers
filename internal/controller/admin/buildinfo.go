package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/buildinfo"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// buildInfoPayload 组装 GET /api/v1/admin/build-info 的 200 载荷。
//
// 契约锁：这里必须保持**唯一一个 gin.H 字面量**且六个键都是字面字符串 ——
// internal/controller/openapi_fields_test.go 的 ginHKeys 按函数名做 AST 抓取，
// 与 openapi.admin.json 的 BuildInfo schema 属性集逐字比对；用变量拼键或再出现
// 第二个字面量都会让字段表测试以 "spec extra / go extra" 失败。
// 键序按字母序排列，与契约文件的属性序一致（纯可读性，测试两侧各自排序）。
func buildInfoPayload() gin.H {
	return gin.H{
		"build_time":  buildinfo.BuildTime(),
		"cgo_enabled": buildinfo.CgoEnabled(),
		"commit":      buildinfo.Commit(),
		"go_version":  buildinfo.GoVersion(),
		"platform":    buildinfo.Platform(),
		"version":     buildinfo.Version(),
	}
}

// handleBuildInfo 回报本实例的编译信息，供管理台"关于"界面与 footer 使用。
//
// 挂在 authed（AdminAuth 完整会话）组内，因此匿名访问者与强制改密 / 2FA 绑定中的
// pending 会话都取不到：编译信息不是恢复流程所需，不给未登录访客留服务端指纹。
// 值全部来自 internal/buildinfo 的进程内快照，不查库、不会 500。
// 2xx 走 pkg/response 的裸 payload（无成功信封）。
func handleBuildInfo(c *gin.Context) {
	response.JSON(c, http.StatusOK, buildInfoPayload())
}
