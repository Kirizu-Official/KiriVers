package client

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// publicOptions 承接 CORS 预检；实际 204 由 ClientProjectAuth 在鉴权前写出。
func publicOptions(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// publicGet 返回项目公开设置子集，不含密钥、store token hash、私钥。用于 CORS / HTTPS / Token 中间件验收。
func publicGet(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	response.JSON(c, http.StatusOK, publicProjectSettings(p))
}

func publicProjectSettings(p *model.Project) gin.H {
	return gin.H{
		"uuid":                      p.ID,
		"slug":                      p.Slug,
		"compare_engine":            p.CompareEngine,
		"minimum_supported_version": p.MinimumSupportedVersion,
		"default_locale":            p.DefaultLocale,
		"require_client_token":      p.RequireClientToken,
		"force_https":               p.ForceHTTPS,
		"device_id_policy":          p.DeviceIDPolicy,
		"storage_visibility":        p.StorageVisibility,
		"created_at":                p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		"updated_at":                p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
