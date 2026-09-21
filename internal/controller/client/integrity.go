package client

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// integrityHandler 承载 GET /versions/:version/integrity。
//
// 分层约束：本文件只做 query 绑定、状态码、ETag / Cache-Control / Vary 写头；
// 状态闸、hash_algo、签名等全部领域规则位于 internal/service/update。
type integrityHandler struct {
	updates *update.Service
}

func newIntegrityHandler(updates *update.Service) *integrityHandler {
	return &integrityHandler{updates: updates}
}

// integrity 处理 GET /api/v1/projects/:project_ref/versions/:version/integrity。
func (h *integrityHandler) integrity(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		checkErrorResponse(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}

	if strings.TrimSpace(c.Query("os")) == "" || strings.TrimSpace(c.Query("arch")) == "" {
		checkErrorResponse(c, http.StatusBadRequest, "INVALID_QUERY_PARAM",
			"os and arch are required", nil)
		return
	}

	osSlug, archSlug := normalizeRequestPlatform(c.Query("os"), c.Query("arch"))

	res, err := h.updates.Integrity(c.Request.Context(), p.ID, osSlug, archSlug, update.IntegrityInput{
		Version:         c.Param("version"),
		OS:              osSlug,
		Arch:            archSlug,
		HashAlgo:        c.Query("hash_algo"),
		Compact:         queryFlag(c.Query("compact")),
		IncludeFileURLs: queryFlag(c.Query("include_file_urls")),
		HwRev:           strings.TrimSpace(c.Query("hw_rev")),
		Channel:         strings.TrimSpace(c.Query("channel")),
	})
	if err != nil {
		writeCheckError(c, err)
		return
	}

	if len(res.Vary) > 0 {
		c.Header("Vary", strings.Join(res.Vary, ", "))
	}
	writeCachedHeaders(c, res.ETag, res.CacheControl)
	if !res.Private && update.MatchesETag(c.GetHeader("If-None-Match"), res.ETag) {
		c.Status(http.StatusNotModified)
		return
	}
	response.JSON(c, http.StatusOK, res.Body)
}

// queryFlag 解析布尔 query 参数；未传视为 false。
func queryFlag(raw string) bool {
	v := parseOptionalBool(raw)
	return v != nil && *v
}
