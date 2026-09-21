package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

const (
	// ContextAdminID 是已认证管理员 UUID。
	ContextAdminID = "admin_id"
	// ContextAdminUsername 是已认证管理员用户名。
	ContextAdminUsername = "admin_username"
	// ContextAdminToken 是当前完整会话的不透明 token，供登出删除单把会话。
	ContextAdminToken = "admin_token"
	// ContextPendingStage 是受限登录所处阶段。
	ContextPendingStage = "admin_pending_stage"
	// ContextProject 是已解析的 *model.Project。
	ContextProject = "project"
	// ContextProjectTokenID 是已认证项目 Token 的 ID。
	ContextProjectTokenID = "project_token_id"
	// ContextProjectTokenFingerprint 是已认证项目 Token 的指纹（明文前 8 字符，
	// 与 CI Token 指纹同语义，审计共用；不含完整密钥）。
	ContextProjectTokenFingerprint = "project_token_fingerprint"
	// ContextAdmin 是已认证 *model.Admin。
	ContextAdmin = "admin"
	// ContextIsPlatformAdmin 是平台管理员标志。
	ContextIsPlatformAdmin = "is_platform_admin"
)

const bearerPrefix = "Bearer "

// parseBearer 按 RFC 6750 解析 Authorization，scheme 大小写不敏感。
func parseBearer(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if len(header) < len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(bearerPrefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// parseClientCredential 读取客户端 Bearer 或 X-Project-Token。
func parseClientCredential(c *gin.Context) string {
	if tok, ok := parseBearer(c); ok {
		return tok
	}
	return strings.TrimSpace(c.GetHeader("X-Project-Token"))
}

// AdminAuth 校验 Authorization: Bearer 完整会话 token（缓存 sess: 键），命中后滑动续期。
func AdminAuth(admins *service.AdminService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := parseBearer(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
			c.Abort()
			return
		}
		admin, err := admins.ParseToken(c.Request.Context(), token)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
			c.Abort()
			return
		}
		setAdminContext(c, admin, token)
		c.Next()
	}
}

// AdminPendingAuth 只接受受限登录 token（缓存 pend: 键），不滑动续期。
func AdminPendingAuth(admins *service.AdminService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := parseBearer(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
			c.Abort()
			return
		}
		admin, stage, err := admins.ParsePendingToken(c.Request.Context(), token)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
			c.Abort()
			return
		}
		c.Set(ContextAdminID, admin.ID.String())
		c.Set(ContextAdminUsername, admin.Username)
		c.Set(ContextAdminToken, token)
		c.Set(ContextPendingStage, stage)
		c.Next()
	}
}

func setAdminContext(c *gin.Context, admin *model.Admin, token string) {
	if admin == nil {
		return
	}
	c.Set(ContextAdmin, admin)
	c.Set(ContextAdminID, admin.ID.String())
	c.Set(ContextAdminUsername, admin.Username)
	c.Set(ContextAdminToken, token)
	c.Set(ContextIsPlatformAdmin, admin.IsPlatformAdmin)
}

// AdminFrom 读取 AdminAuth / ProjectAccess 放入的管理员。
func AdminFrom(c *gin.Context) *model.Admin {
	v, ok := c.Get(ContextAdmin)
	if !ok {
		return nil
	}
	admin, _ := v.(*model.Admin)
	return admin
}
