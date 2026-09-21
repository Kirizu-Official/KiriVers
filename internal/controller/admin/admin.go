package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// Register 挂载登录（匿名）、管理员 CRUD（需完整会话）以及项目 CRUD/Token（需实例管理员或项目 Token）。
// telemetry 供按哈希删除遥测事件（可 nil）；limiter 为进程内限流器（可 nil），
// 仅作用于 CI Token 维度（ci:{fingerprint} 键），实例管理员与项目 Token 不限。
// audit 供管理动作审计（§11.2 / C15-3，可 nil；nil 时不写审计、审计查询 503）。
// announcements 为 nil 时不注册公告 CRUD / reorder 路由。
func Register(rg *gin.RouterGroup, admins *service.AdminService, projects *service.ProjectService, telemetry *service.TelemetryService, limiter *middleware.Limiter, audit *service.AuditService, announcements *service.AnnouncementService, geoip *service.GeoipService) {
	if admins == nil {
		return
	}
	ah := &auditHelper{svc: audit}
	h := &handler{admins: admins}
	rg.POST("/auth/login", h.login)

	pending := rg.Group("")
	pending.Use(middleware.AdminPendingAuth(admins))
	pending.POST("/auth/2fa/totp/setup", h.totpSetup)
	pending.POST("/auth/2fa/totp/confirm", h.totpConfirm)
	pending.POST("/auth/2fa/recovery/ack", h.recoveryAck)
	pending.POST("/auth/2fa/verify", h.verify2FA)
	pending.POST("/auth/2fa/webauthn/login/begin", h.webauthnLoginBegin)
	pending.POST("/auth/2fa/webauthn/login/finish", h.webauthnLoginFinish)
	pending.POST("/auth/2fa/webauthn/register/begin", h.webauthnRegisterBeginPending)
	pending.POST("/auth/2fa/webauthn/register/finish", h.webauthnRegisterFinishPending)
	pending.POST("/auth/2fa/skip-passkey", h.skipPasskey)

	authed := rg.Group("")
	authed.Use(middleware.AdminAuth(admins))
	authed.POST("/auth/logout", h.logout)
	authed.GET("/auth/2fa", h.twoFAStatus)
	authed.POST("/auth/2fa/totp/rotate/setup", h.totpRotateSetup)
	authed.POST("/auth/2fa/totp/rotate/confirm", h.totpRotateConfirm)
	authed.POST("/auth/2fa/recovery/regenerate", h.recoveryRegenerate)
	authed.GET("/auth/2fa/passkeys", h.listPasskeys)
	authed.POST("/auth/2fa/passkeys/begin", h.passkeyBegin)
	authed.POST("/auth/2fa/passkeys/finish", h.passkeyFinish)
	authed.DELETE("/auth/2fa/passkeys/:id", h.deletePasskey)
	// 本实例编译信息（版本 / commit / 构建时间 / Go / 平台 / CGO）：管理台"关于"与
	// footer 使用。仅完整会话可读，pending 会话取不到（见 buildinfo.go）。
	authed.GET("/build-info", handleBuildInfo)

	if projects != nil {
		projects.SetAdmins(admins)
	}

	platform := rg.Group("")
	platform.Use(middleware.RequireInstanceAdmin(admins, projects))
	platform.GET("/admins", h.list)
	platform.POST("/admins", h.create)
	platform.PATCH("/admins/:id", h.update)
	platform.DELETE("/admins/:id", h.remove)
	if projects != nil {
		nh := &projectHandler{projects: projects, audit: ah}
		platform.GET("/nodes", nh.listNodes)
		platform.GET("/projects/:project_ref/node-sync", nh.listNodeSync)
	}

	if geoip != nil {
		gh := &geoipHandler{geoip: geoip}
		platform.GET("/geoip/databases", gh.list)
		platform.POST("/geoip/databases", gh.upload)
		platform.PATCH("/geoip/databases/:id", gh.patch)
		platform.DELETE("/geoip/databases/:id", gh.remove)
	}

	if projects == nil {
		return
	}
	ph := &projectHandler{projects: projects, audit: ah}
	authed.GET("/projects", ph.list)
	platform.POST("/projects", ph.create)

	read := rg.Group("/projects/:project_ref")
	read.Use(middleware.ProjectAccess(admins, projects, model.ScopeProjectRead))
	read.GET("", ph.get)
	read.GET("/store-listings", ph.listStoreListings)
	read.GET("/channels", ph.listChannels)
	read.GET("/languages", ph.listLanguages)
	read.GET("/matrix", ph.listMatrix)
	read.GET("/hw-revs", ph.listHwRevs)
	read.GET("/platforms/catalog", ph.platformCatalog)
	read.GET("/install-policy-rules", ph.getProjectInstallPolicy)
	read.GET("/install-policy-rules/reference", ph.getInstallPolicyReference)
	read.GET("/channels/:slug/install-policy-rules", ph.getChannelInstallPolicy)
	read.GET("/clients", ph.listClients)
	read.GET("/clients/stats", ph.clientStats)
	read.GET("/clients/:client_id", ph.getClient)
	read.GET("/members", ph.listMembers)
	read.GET("/stats/series", ph.statsSeries)
	read.GET("/versions/:version/gray", ph.getVersionGray)
	read.GET("/versions/:version/gray/clients", ph.listVersionGrayClients)
	read.GET("/versions/:version/gray/series", ph.listVersionGraySeries)
	read.GET("/versions/:version/gray/allowlist", ph.listVersionGrayAllowlist)
	read.GET("/versions/:version/exists", ph.existsVersion)
	read.GET("/versions/:version", ph.getVersion)
	read.GET("/versions", ph.listVersions)
	read.GET("/versions/:version/line-defaults", ph.lineDefaults)
	read.GET("/versions/:version/lines", ph.listVersionLines)
	read.GET("/versions/:version/lines/:os/:arch", ph.getVersionLine)
	read.GET("/versions/:version/lines/:os/:arch/manifest", ph.getManifest)
	// Publish webhook 投递记录排障查询（C14-4 / §5.8），游标分页。
	read.GET("/webhook/deliveries", ph.listWebhookDeliveries)
	// 项目级审计查询（§11.2 / C15-3），倒序游标分页。
	read.GET("/audit", ph.listAudit)

	artifact := rg.Group("/projects/:project_ref")
	artifact.Use(middleware.ProjectAccess(admins, projects, model.ScopeArtifactWrite))
	// CI Token 维度限流（C11-6）：ci:{fingerprint} 键，默认 600/分钟；
	// 实例管理员 / 项目 Token 无指纹，直接放行（管理配额与 CI/公开配额分离）。
	artifact.Use(middleware.CIRateLimit(limiter))
	artifact.PUT("/versions/:version", ph.putVersion)
	artifact.PATCH("/versions/:version", ph.patchVersion)
	artifact.POST("/versions/:version/lines", ph.createVersionLine)
	artifact.POST("/versions/:version/lines/:os/:arch/ready", ph.readyVersionLine)
	artifact.POST("/versions/:version/lines/:os/:arch/yank", ph.yankVersionLine)
	artifact.POST("/versions/:version/lines/:os/:arch/disable", ph.disableVersionLine)
	artifact.PUT("/versions/:version/lines/:os/:arch/artifacts", ph.uploadArtifact)
	artifact.POST("/versions/:version/lines/:os/:arch/artifacts", ph.uploadArtifact)
	artifact.PUT("/versions/:version/lines/:os/:arch/manifest", ph.putManifest)
	artifact.POST("/versions/:version/lines/:os/:arch/build-archive", ph.buildArchive)
	artifact.POST("/versions/:version/lines/:os/:arch/artifacts/tus", ph.createTusUpload)
	artifact.HEAD("/artifacts/tus/:upload_id", ph.getTusUpload)
	artifact.PATCH("/artifacts/tus/:upload_id", ph.patchTusUpload)
	artifact.DELETE("/artifacts/tus/:upload_id", ph.deleteTusUpload)
	artifact.POST("/versions/:version/lines/:os/:arch/artifacts/presign", ph.presignUpload)
	artifact.POST("/ci/releases", ph.ciRelease)
	artifact.POST("/versions/:version/artifacts/bundle", ph.uploadBundle)
	// 产物复用（C14-1 / §5.8）：引用已有存储对象，零字节拷贝。
	artifact.POST("/versions/:version/artifacts/reuse", ph.reuseArtifact)
	// 单文件二进制差量生成（C10-7 / §7.2）：目标版本 = :version，源版本在请求体。
	artifact.POST("/versions/:version/artifacts/delta", ph.createDeltaJob)
	artifact.POST("/artifacts/cleanup", ph.cleanupArtifacts)

	rg.GET("/jobs/:job_id", middleware.AnyAdminOrProjectAccess(admins, projects), ph.getJob)

	publish := rg.Group("/projects/:project_ref")
	publish.Use(middleware.ProjectAccess(admins, projects, model.ScopeReleasePublish))
	publish.Use(middleware.CIRateLimit(limiter))
	publish.POST("/versions/:version/publish", ph.publishVersion)
	publish.POST("/versions/:version/deprecate", ph.deprecateVersion)
	publish.POST("/versions/:version/revoke", ph.revokeVersion)
	// 渠道晋升（C14-2 / §4.3 / §5.8）：后缀规则允许时直改渠道。
	publish.POST("/versions/:version/promote", ph.promoteVersion)
	// 增量灰度旋钮 / 立即全量 / 白名单（按名册 UUID）：发布运维语义，
	// 与 publish/deprecate/revoke 同一资源能力（release:publish）。
	publish.PATCH("/versions/:version/gray", ph.patchVersionGray)
	publish.POST("/versions/:version/gray/complete", ph.completeVersionGray)
	publish.POST("/versions/:version/gray/allowlist", ph.addVersionGrayAllowlist)
	publish.DELETE("/versions/:version/gray/allowlist", ph.deleteVersionGrayAllowlist)
	publish.PATCH("/versions/:version/lines/:os/:arch", ph.patchVersionLine)

	write := rg.Group("/projects/:project_ref")
	write.Use(middleware.ProjectAccess(admins, projects, model.ScopeProjectAdmin))
	write.PATCH("", ph.patch)
	write.DELETE("", ph.remove)
	write.POST("/tokens", ph.createToken)
	write.GET("/tokens", ph.listTokens)
	write.DELETE("/tokens/:token_id", ph.deleteToken)
	write.POST("/ci-tokens", ph.createCIToken)
	write.GET("/ci-tokens", ph.listCITokens)
	write.DELETE("/ci-tokens/:token_id", ph.deleteCIToken)
	write.POST("/store-listings", ph.createStoreListing)
	write.PATCH("/store-listings/:listing_id", ph.patchStoreListing)
	write.DELETE("/store-listings/:listing_id", ph.deleteStoreListing)
	write.POST("/channels", ph.createChannel)
	write.PATCH("/channels/:slug", ph.patchChannel)
	write.DELETE("/channels/:slug", ph.deleteChannel)
	write.PUT("/install-policy-rules", ph.putProjectInstallPolicy)
	write.PUT("/channels/:slug/install-policy-rules", ph.putChannelInstallPolicy)
	write.POST("/languages", ph.createLanguage)
	write.PATCH("/languages/:code", ph.patchLanguage)
	write.DELETE("/languages/:code", ph.deleteLanguage)
	write.POST("/matrix", ph.createMatrix)
	write.PATCH("/matrix/:os/:arch", ph.patchMatrix)
	write.POST("/hw-revs", ph.createHwRev)
	write.PATCH("/hw-revs/:slug", ph.patchHwRev)
	write.DELETE("/hw-revs/:slug", ph.deleteHwRev)
	write.DELETE("/versions/:version", ph.deleteVersion)
	write.DELETE("/clients/:client_id", ph.deleteClient)
	write.POST("/members", ph.createMember)
	write.PATCH("/members/:admin_id", ph.patchMember)
	write.DELETE("/members/:admin_id", ph.deleteMember)

	if announcements != nil {
		anh := &announcementHandler{projects: projects, announcements: announcements, audit: ah}
		read.GET("/announcements", anh.list)
		read.GET("/announcements/:announcement_id", anh.get)
		write.POST("/announcements", anh.create)
		write.PATCH("/announcements/:announcement_id", anh.patch)
		write.DELETE("/announcements/:announcement_id", anh.remove)
		write.PUT("/announcements/reorder", anh.reorder)
	}

	// 遥测隐私删除（§13.11 按哈希删除；C15-4 扩展同删灰度白名单并写审计）。
	RegisterTelemetry(rg, admins, projects, telemetry, audit)
}

type handler struct {
	admins *service.AdminService
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminWriteReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func publicAdmin(a *model.Admin) gin.H {
	body := gin.H{
		"id":         a.ID,
		"username":   a.Username,
		"created_at": formatTimeUTC(a.CreatedAt),
		"updated_at": formatTimeUTC(a.UpdatedAt),
	}
	if a.LastLoginAt != nil {
		body["last_login_at"] = formatTimeUTC(*a.LastLoginAt)
	} else {
		body["last_login_at"] = nil
	}
	if a.LastLoginIP != nil {
		body["last_login_ip"] = *a.LastLoginIP
	} else {
		body["last_login_ip"] = nil
	}
	body["is_platform_admin"] = a.IsPlatformAdmin
	return body
}

func (h *handler) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	out, err := h.admins.Login(c.Request.Context(), req.Username, req.Password, c.ClientIP())
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeLoginResult(c, out)
}

func (h *handler) list(c *gin.Context) {
	list, err := h.admins.List(c.Request.Context())
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicAdmin(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"admins": items})
}

func (h *handler) create(c *gin.Context) {
	var req adminWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	admin, err := h.admins.Create(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicAdmin(admin))
}

func (h *handler) update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
		return
	}
	var req adminWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	admin, err := h.admins.Update(c.Request.Context(), id, req.Username, req.Password)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicAdmin(admin))
}

func (h *handler) remove(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
		return
	}
	if err := h.admins.Delete(c.Request.Context(), id); err != nil {
		writeAdminErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeLoginResult(c *gin.Context, out *service.LoginResult) {
	if out.Status == service.LoginStatusComplete {
		response.JSON(c, http.StatusOK, gin.H{
			"status":       service.LoginStatusComplete,
			"access_token": out.Token,
			"token_type":   "Bearer",
			"expires_in":   out.ExpiresIn,
			"admin":        publicAdmin(out.Admin),
		})
		return
	}
	body := gin.H{
		"status":        service.LoginStatusPending,
		"pending_token": out.PendingToken,
		"token_type":    "Bearer",
		"expires_in":    out.ExpiresIn,
		"stage":         out.Stage,
	}
	if out.Stage == service.StageSecondFactor {
		body["has_passkey"] = out.HasPasskey
	}
	if len(out.RecoveryCodes) > 0 {
		body["recovery_codes"] = out.RecoveryCodes
	}
	if out.Secret != "" {
		body["secret"] = out.Secret
		body["otpauth_url"] = out.OTPAuthURL
	}
	response.JSON(c, http.StatusOK, body)
}

func writeAdminErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrInvalidToken):
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil)
	case errors.Is(err, service.ErrTOTPRateLimited):
		response.Error(c, http.StatusTooManyRequests, "TOTP_RATE_LIMITED", err.Error(), nil)
	case errors.Is(err, service.ErrWebAuthnNotReady):
		response.Error(c, http.StatusServiceUnavailable, "NOT_READY", err.Error(), nil)
	case errors.Is(err, service.ErrAdminNotFound):
		response.Error(c, http.StatusNotFound, "ADMIN_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrPasskeyNotFound):
		response.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrUsernameTaken):
		response.Error(c, http.StatusConflict, "USERNAME_TAKEN", err.Error(), nil)
	case errors.Is(err, service.ErrLastAdmin):
		response.Error(c, http.StatusConflict, "LAST_ADMIN", err.Error(), nil)
	case errors.Is(err, service.ErrInvalidUsername), errors.Is(err, service.ErrWeakPassword),
		errors.Is(err, service.ErrInvalidStage), errors.Is(err, service.ErrRecoveryNotAcked),
		errors.Is(err, service.ErrTOTPNotConfirmed), service.IsInvalidRequest(err):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	case errors.Is(err, service.ErrCacheUnavailable):
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	default:
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}
