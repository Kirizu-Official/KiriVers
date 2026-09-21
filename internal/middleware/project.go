package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// RequireInstanceAdmin 只允许实例管理员完整会话。
// 管理项目创建/列表：缺 Bearer → 401；项目 Token / CI Token → 403 FORBIDDEN。
func RequireInstanceAdmin(admins *service.AdminService, projects *service.ProjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := parseBearer(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
			c.Abort()
			return
		}
		if admins != nil {
			if admin, err := admins.ParseToken(c.Request.Context(), token); err == nil {
				if !admin.IsPlatformAdmin {
					response.Error(c, http.StatusForbidden, "FORBIDDEN", "platform admin required", nil)
					c.Abort()
					return
				}
				setAdminContext(c, admin, token)
				c.Next()
				return
			}
		}
		if rejectProjectOrCIToken(c, projects, token) {
			return
		}
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		c.Abort()
	}
}

// ProjectAccess 允许实例管理员完整会话，或对本项目持有 minScope（project:admin 视为全部 scope）的项目 Token。
// CI Token 与其它项目的 Token → 403。缺 Bearer → 401。
func ProjectAccess(admins *service.AdminService, projects *service.ProjectService, minScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := parseBearer(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
			c.Abort()
			return
		}
		if admins != nil {
			if admin, err := admins.ParseToken(c.Request.Context(), token); err == nil {
				setAdminContext(c, admin, token)
				if projects == nil {
					response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
					c.Abort()
					return
				}
				proj, rerr := projects.Resolve(c.Request.Context(), c.Param("project_ref"))
				if rerr != nil {
					if errors.Is(rerr, service.ErrProjectNotFound) {
						response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", rerr.Error(), nil)
					} else {
						response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
					}
					c.Abort()
					return
				}
				c.Set(ContextProject, proj)
				if admin.IsPlatformAdmin || projects.HasProjectMembership(c.Request.Context(), admin.ID, proj.ID) {
					c.Next()
					return
				}
				response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
				c.Abort()
				return
			}
		}
		if projects == nil {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
			c.Abort()
			return
		}
		pt, err := projects.LookupProjectToken(c.Request.Context(), token)
		if err == nil {
			if model.TokenExpired(pt.ExpiresAt, time.Now().UTC()) {
				response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
				c.Abort()
				return
			}
			proj, rerr := projects.Resolve(c.Request.Context(), c.Param("project_ref"))
			if rerr != nil {
				if errors.Is(rerr, service.ErrProjectNotFound) {
					response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", rerr.Error(), nil)
				} else {
					response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
				}
				c.Abort()
				return
			}
			if pt.ProjectID != proj.ID || !model.ScopesInclude(pt.Scopes, minScope) {
				response.Error(c, http.StatusForbidden, "FORBIDDEN", "insufficient project token scope", nil)
				c.Abort()
				return
			}
			c.Set(ContextProject, proj)
			c.Set(ContextProjectTokenID, pt.ID.String())
			// 指纹（明文前 8 字符）：审计主体识别（C15-3/C15-5）与限流键语义对齐。
			c.Set(ContextProjectTokenFingerprint, pt.Fingerprint)
			c.Next()
			return
		}
		if !errors.Is(err, service.ErrTokenNotFound) {
			response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
			c.Abort()
			return
		}

		ciTok, ciErr := projects.LookupCIToken(c.Request.Context(), token)
		if ciErr == nil {
			if model.TokenExpired(ciTok.ExpiresAt, time.Now().UTC()) {
				response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
				c.Abort()
				return
			}
			proj, rerr := projects.Resolve(c.Request.Context(), c.Param("project_ref"))
			if rerr != nil {
				if errors.Is(rerr, service.ErrProjectNotFound) {
					response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", rerr.Error(), nil)
				} else {
					response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
				}
				c.Abort()
				return
			}
			if ciTok.ProjectID != proj.ID || !model.ScopesInclude(ciTok.Scopes, minScope) {
				response.Error(c, http.StatusForbidden, "FORBIDDEN", "insufficient ci token scope", nil)
				c.Abort()
				return
			}
			c.Set(ContextProject, proj)
			c.Set("ci_token_id", ciTok.ID.String())
			// CI Token 指纹（明文前 8 字符）：CIRateLimit 以 ci:{fingerprint}
			// 为限流键，与匿名 ip: 命名空间天然不同（C11-6）。
			c.Set(ContextCITokenFingerprint, ciTok.Fingerprint)
			c.Next()
			return
		}

		if rejectProjectOrCIToken(c, projects, token) {
			return
		}
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		c.Abort()
	}
}

// AnyAdminOrProjectAccess 允许实例管理员完整会话、项目 Token 或 CI Token 鉴权（供实例级/项目级通用的任务查询等端点，C07-8）。
func AnyAdminOrProjectAccess(admins *service.AdminService, projects *service.ProjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := parseBearer(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
			c.Abort()
			return
		}
		if admins != nil {
			if admin, err := admins.ParseToken(c.Request.Context(), token); err == nil {
				setAdminContext(c, admin, token)
				if admin.IsPlatformAdmin {
					c.Set("is_instance_admin", true)
				}
				c.Next()
				return
			}
		}
		if projects == nil {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
			c.Abort()
			return
		}
		if pt, err := projects.LookupProjectToken(c.Request.Context(), token); err == nil {
			if model.TokenExpired(pt.ExpiresAt, time.Now().UTC()) {
				response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "token expired", nil)
				c.Abort()
				return
			}
			c.Set(ContextProjectTokenID, pt.ID.String())
			c.Set(ContextProjectTokenFingerprint, pt.Fingerprint)
			c.Set("caller_project_id", pt.ProjectID)
			c.Next()
			return
		}
		if ciTok, err := projects.LookupCIToken(c.Request.Context(), token); err == nil {
			if model.TokenExpired(ciTok.ExpiresAt, time.Now().UTC()) {
				response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "token expired", nil)
				c.Abort()
				return
			}
			c.Set("ci_token_id", ciTok.ID.String())
			c.Set("caller_project_id", ciTok.ProjectID)
			c.Next()
			return
		}
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		c.Abort()
	}
}

// ClientProjectAuth 解析 :project_ref，应用 CORS / 强制 HTTPS，并在 require_client_token 时校验 Bearer 或 X-Project-Token。
func ClientProjectAuth(projects *service.ProjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := resolveProjectOrAbort(c, projects)
		if p == nil {
			return
		}
		if abortCORSPreflight(c, p) {
			return
		}
		if !applyProjectHTTPGuards(c, p) {
			return
		}
		if p.RequireClientToken {
			tok := parseClientCredential(c)
			if tok == "" || !projects.ValidClientToken(c.Request.Context(), p.ID, tok) {
				response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid project token", nil)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// ClientProjectResolve 解析 :project_ref 并应用 CORS / force_https，不校验项目 Token 与 urlsign。
// 供 Markdown 媒体 GET：<img src> 无法携带 Authorization。
func ClientProjectResolve(projects *service.ProjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := resolveProjectOrAbort(c, projects)
		if p == nil {
			return
		}
		if abortCORSPreflight(c, p) {
			return
		}
		if !applyProjectHTTPGuards(c, p) {
			return
		}
		c.Next()
	}
}

// StoreAuth 在配置了 store_token 时校验其明文；否则若 require_client_token 则走项目 Token，二者皆关则放行。
func StoreAuth(projects *service.ProjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := resolveProjectOrAbort(c, projects)
		if p == nil {
			return
		}
		if abortCORSPreflight(c, p) {
			return
		}
		if !applyProjectHTTPGuards(c, p) {
			return
		}
		tok := parseClientCredential(c)
		switch {
		case p.HasStoreToken():
			if !projects.ValidStoreToken(p, tok) {
				response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid feed token", nil)
				c.Abort()
				return
			}
		case p.RequireClientToken:
			if tok == "" || !projects.ValidClientToken(c.Request.Context(), p.ID, tok) {
				response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid project token", nil)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// ProjectFrom 读取 ClientProjectAuth / StoreAuth / ProjectAccess 放入的项目。
func ProjectFrom(c *gin.Context) *model.Project {
	v, ok := c.Get(ContextProject)
	if !ok {
		return nil
	}
	p, _ := v.(*model.Project)
	return p
}

func resolveProjectOrAbort(c *gin.Context, projects *service.ProjectService) *model.Project {
	if projects == nil {
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		c.Abort()
		return nil
	}
	p, err := projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		if errors.Is(err, service.ErrProjectNotFound) {
			response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error(), nil)
		} else {
			response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		}
		c.Abort()
		return nil
	}
	c.Set(ContextProject, p)
	return p
}

// abortCORSPreflight 对 OPTIONS 只回 CORS，不要求 Token / HTTPS（浏览器预检不会带 X-Project-Token）。
func abortCORSPreflight(c *gin.Context, p *model.Project) bool {
	if c.Request.Method != http.MethodOptions {
		return false
	}
	echoCORS(c, p)
	c.Status(http.StatusNoContent)
	c.Abort()
	return true
}

// applyProjectHTTPGuards 写入 CORS，并在 force_https 时拒绝明文（网关可用 X-Forwarded-Proto: https）。
func applyProjectHTTPGuards(c *gin.Context, p *model.Project) bool {
	echoCORS(c, p)
	if p.ForceHTTPS && !requestIsHTTPS(c) {
		response.Error(c, http.StatusForbidden, "FORBIDDEN", "https is required for this project", nil)
		c.Abort()
		return false
	}
	return true
}

const corsAllowHeaders = "Authorization, Content-Type, X-Project-Token, X-Channel-Token"
const corsAllowMethods = "GET, POST, PATCH, DELETE, OPTIONS"

func echoCORS(c *gin.Context, p *model.Project) {
	if p == nil || len(p.CORSOrigins) == 0 {
		return
	}
	origin := c.GetHeader("Origin")
	if origin == "" {
		return
	}
	for _, allowed := range p.CORSOrigins {
		if allowed == "*" || allowed == origin {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Headers", corsAllowHeaders)
			c.Header("Access-Control-Allow-Methods", corsAllowMethods)
			c.Header("Vary", "Origin")
			return
		}
	}
}

func requestIsHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		proto = c.GetHeader("X-Forwarded-Protocol")
	}
	first, _, _ := strings.Cut(proto, ",")
	return strings.EqualFold(strings.TrimSpace(first), "https")
}

// RequestIsHTTPS 报告本请求是否为 HTTPS（TLS 或 X-Forwarded-Proto，与 force_https 相同口径）。
func RequestIsHTTPS(c *gin.Context) bool {
	return requestIsHTTPS(c)
}

// RequestOrigin 返回本请求 origin（scheme://Host），供 ${site_url} 在缺失 Referer 时回退。
func RequestOrigin(c *gin.Context) string {
	scheme := "http"
	if requestIsHTTPS(c) {
		scheme = "https"
	}
	host := c.Request.Host
	if host == "" {
		host = c.Request.URL.Host
	}
	return scheme + "://" + host
}

// rejectProjectOrCIToken 若凭证能在项目/CI 表中命中则写 403 并 abort。
func rejectProjectOrCIToken(c *gin.Context, projects *service.ProjectService, token string) bool {
	if projects == nil || token == "" {
		return false
	}
	if _, err := projects.LookupProjectToken(c.Request.Context(), token); err == nil {
		response.Error(c, http.StatusForbidden, "FORBIDDEN", "project token cannot perform this operation", nil)
		c.Abort()
		return true
	}
	if _, err := projects.LookupCIToken(c.Request.Context(), token); err == nil {
		response.Error(c, http.StatusForbidden, "FORBIDDEN", "ci token cannot perform this operation", nil)
		c.Abort()
		return true
	}
	return false
}
