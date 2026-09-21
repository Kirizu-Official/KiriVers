// Package client 是客户端更新查询、校验与商店 feed 的 HTTP 接入层。
// 探活（/health、/ready）与本平面 openapi.json 由 controller 包的 registerPlane
// 统一挂载（双平面共用），不再在此注册。
package client

import (
	"github.com/gin-gonic/gin"

	storecontroller "github.com/Kirizu-Official/KiriVers/internal/controller/client/store"
	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/urlsign"
)

// Register 挂载可选的公开项目设置 GET（供 CORS / HTTPS / ClientProjectAuth 验收），
// 以及 POST update/check 与 GET changelog/:channel/:os/:arch（updates 非 nil 时；走既有 client token 中间件开关）。
// announcements 非 nil 时挂 GET /projects/:project_ref/announcements（ClientProjectAuth + feed IP 限流）。
// telemetry 供 check/diff 的连续失败降级与遥测上报端点（可 nil）；limiter 为
// 全进程共享限流器（可 nil，nil = 不限流，仅测试装配省略）。
// signer 是私有存储短时签名器（§13.7 / C15-1，可 nil）：私有项目下载校验
// ?exp=&sig= 签名，公开项目直链不受影响（C15-2）。
func Register(rg *gin.RouterGroup, projects *service.ProjectService, updates *update.Service, telemetry *service.TelemetryService, limiter *middleware.Limiter, signer *urlsign.Signer, announcements *service.AnnouncementService) {
	if projects != nil {
		pg := rg.Group("/projects/:project_ref")
		pg.Use(middleware.ClientProjectAuth(projects))
		pg.GET("", publicGet)
		pg.OPTIONS("", publicOptions)

		rep := &clientReportHandler{projects: projects, telemetry: telemetry, limiter: limiter}
		pg.POST("/clients/report", rep.report)

		ch := &clientHandler{projects: projects, signer: signer}
		pg.GET("/packages/:ref", ch.downloadPackage)
		pg.HEAD("/packages/:ref", ch.downloadPackage)

		cat := &catalogHandler{projects: projects}
		pg.GET("/channels", middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP), cat.listChannels)
		pg.GET("/matrix", middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP), cat.listMatrix)
		pg.GET("/languages", middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP), cat.listLanguages)

		if updates != nil {
			chk := &checkHandler{updates: updates, telemetry: telemetry, limiter: limiter, projects: projects}
			// changelog / integrity：无 body 身份，纯 IP 维度中间件限流
			//（store_per_ip_per_minute，§14 与公开 feed 共用配额键）。
			pg.POST("/update/check", chk.updateCheck)
			pg.GET("/changelog/:channel/:os/:arch", middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP), chk.changelog)

			// integrity（§8/§10.2）与 diff（§10.3）端点本体；
			// diff 为 POST，恒 private no-store（脏路径属每设备输入）。
			it := &integrityHandler{updates: updates}
			pg.GET("/versions/:version/integrity", middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP), it.integrity)
			df := &diffHandler{updates: updates, telemetry: telemetry, limiter: limiter}
			pg.POST("/update/diff", df.diff)
			pk := &packHandler{updates: updates, telemetry: telemetry, limiter: limiter}
			pg.POST("/update/pack", pk.pack)

			// 商店协议 feed（§9 / §9.2）：独立分组挂 StoreAuth——feed 鉴权按
			// store_token → require_client_token → 放行的顺序裁决（C16-7），
			// 与页面级 ClientProjectAuth 语义不同；开关关闭 → 404（C16-4）。
			fg := rg.Group("/projects/:project_ref")
			fg.Use(middleware.StoreAuth(projects))
			storecontroller.Register(fg, projects, updates, signer, limiter, nil)
		}

		if announcements != nil {
			anh := &announcementHandler{announcements: announcements}
			pg.GET("/announcements", middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP), anh.list)
		}

		// 遥测上报（§10.4）：只收不挡，202 语义；设备维度限流（无 device 回退 IP）。
		tm := &telemetryHandler{projects: projects, telemetry: telemetry, limiter: limiter}
		pg.POST("/telemetry/report", tm.report)
	}
}
