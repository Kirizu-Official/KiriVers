// Package controller 装配 admin 与 client 双平面路由，不写业务规则。
// 双平面由两个独立 gin.Engine 分别承载（单进程双监听，09-14-split-dual-server）：
// RegisterClient 挂公开客户端面，RegisterAdmin 挂管理面（后台管理 + CI Agent）。
package controller

import (
	"context"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/controller/admin"
	"github.com/Kirizu-Official/KiriVers/internal/controller/client"
	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
	"github.com/Kirizu-Official/KiriVers/pkg/urlsign"
)

// Deps 是 HTTP 层需要的业务依赖。Admin 为 nil 时不注册管理账号路由；
// Updates 为 nil 时不注册 update/check 与 changelog 路由。
// Telemetry 为 nil 时遥测上报/隐私删除返回 503；Limiter 为 nil 时不启用限流。
// Limiter 必须是全进程共享单例：handler 内双维度检查与路由 IP 中间件共用。
// Signer 是私有存储短时签名器（§13.7 / C15-1，可 nil）：私有项目下载校验
// 签名、公开项目直链不变（C15-2）；Audit 为 nil 时不写审计（§11.2 / C15-3）。
// Announcements 为 nil 时不注册公告路由。
type Deps struct {
	Admin         *service.AdminService
	Project       *service.ProjectService
	Updates       *update.Service
	Telemetry     *service.TelemetryService
	Limiter       *middleware.Limiter
	Signer        *urlsign.Signer
	Audit         *service.AuditService
	Announcements *service.AnnouncementService
	Media         *service.MediaService
	GeoIP         *service.GeoipService
	Ready         func(context.Context) error
	// DBAvailable 为 false 时，除 /health /openapi.json 外的 /api 路径
	// 返回 404 NOT_FOUND（与启动期未挂业务路由时 NoRoute 一致）。nil 视为可用。
	DBAvailable func() bool
	// AdminStaticDir 是管理平面托管的编译后前端根（默认 frontend/dist）。
	// 空字符串显式关闭 UI（即使 AdminStaticFS 含 index.html）。磁盘上的
	// index.html 优先；缺失时回退到 AdminStaticFS。两者都没有则为 API-only。
	// 不得用 gin.Static / StaticFS 注册，以免破坏 OpenAPI 路由双向匹配。
	AdminStaticDir string
	// AdminStaticFS 是编译进二进制的 frontend/dist（cmd 传入 frontend.Dist()）。
	// 单元测试保持 nil，用 t.TempDir() 或 fstest.MapFS 注入，不依赖仓库是否 yarn build。
	AdminStaticFS fs.FS
	// ClientProxyURL 是管理平面反代 /api/v1/projects/** 的本进程 client 基址。
	// 空则不反代（单元测试保持该路径 JSON 404）。
	ClientProxyURL string
}

// RegisterClient 将客户端平面路由挂到引擎上：探活/契约 +
// /api/v1 下的公开更新查询、下载、feed 与遥测端点。
func RegisterClient(engine *gin.Engine, deps Deps) {
	registerPlane(engine, deps, clientOpenAPISpec)
	client.Register(engine.Group("/api/v1"), deps.Project, deps.Updates, deps.Telemetry, deps.Limiter, deps.Signer, deps.Announcements)
	client.RegisterMedia(engine.Group("/api/v1"), deps.Project, deps.Media, deps.Limiter)
}

// RegisterAdmin 将管理平面路由挂到引擎上：探活/契约 +
// /api/v1/admin 下的后台管理与 CI Agent 管理接口，并覆盖 NoRoute 以托管静态管理台
// （不注册 gin.Static / StaticFS，避免破坏 TestOpenAPIRoutesSync）。
func RegisterAdmin(engine *gin.Engine, deps Deps) {
	registerPlane(engine, deps, adminOpenAPISpec)
	admin.Register(engine.Group("/api/v1").Group("/admin"), deps.Admin, deps.Project, deps.Telemetry, deps.Limiter, deps.Audit, deps.Announcements, deps.GeoIP)
	admin.RegisterMedia(engine.Group("/api/v1").Group("/admin"), deps.Admin, deps.Project, deps.Media)
	mountClientProjectsProxy(engine, deps.ClientProxyURL)
	mountAdminStatic(engine, deps.AdminStaticDir, deps.AdminStaticFS)
}

// registerPlane 挂载两平面共用的部分：NoRoute JSON 404、/health
// 与本平面 /openapi.json（spec 按平面传入）。health 恒 200；body.ready
// 为 true 当且仅当数据库与对象存储均可用（deps.Ready 可 nil → ready=false）。
func registerPlane(engine *gin.Engine, deps Deps, spec []byte) {
	engine.Use(middleware.DBUnavailableNotFound(deps.DBAvailable))
	engine.NoRoute(response.NotFound)
	v1 := engine.Group("/api/v1")
	v1.GET("/openapi.json", serveOpenAPI(spec))
	v1.GET("/health", health(deps.Ready))
}

// health 进程存活探针：无外部依赖失败也不改变 HTTP 200。
// ready 字段复用 CheckReady（DB Ping + 存储 Head .ready）。
func health(readyFn func(context.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		response.JSON(c, http.StatusOK, healthPayload(probeReady(readyFn, c.Request.Context())))
	}
}

func probeReady(readyFn func(context.Context) error, ctx context.Context) bool {
	return readyFn != nil && readyFn(ctx) == nil
}

func healthPayload(ready bool) gin.H {
	return gin.H{"status": "ok", "ready": ready}
}
