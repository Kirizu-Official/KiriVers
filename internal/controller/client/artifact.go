package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
	"github.com/Kirizu-Official/KiriVers/pkg/urlsign"
)

type clientHandler struct {
	projects *service.ProjectService
	// signer 是私有存储短时签名器（§13.7 / C15-1）；nil 时不具备校验能力，
	// 私有项目的下载一律 403（安全缺省：测试装配省略签名器时私有不可下载）。
	signer *urlsign.Signer
}

func (h *clientHandler) downloadPackage(c *gin.Context) {
	h.serveDownload(c, c.Param("ref"))
}

func (h *clientHandler) serveDownload(c *gin.Context, artifactRef string) {
	if h.projects.Storage() == nil {
		response.Error(c, http.StatusServiceUnavailable, "NOT_READY", "storage backend unavailable", nil)
		return
	}

	// 私有存储签名闸（§13.7 / C15-1 / C15-2）：private 项目要求有效未过期的
	// ?exp=&sig= 签名（缺失 / 过期 / 不匹配 / 无校验器 → 403 FORBIDDEN，
	// 恒定时间比较在 pkg/urlsign 内完成）；public 项目直链不要求签名。
	if p := middleware.ProjectFrom(c); p != nil && p.StorageVisibility == model.StorageVisibilityPrivate {
		if h.signer == nil || h.signer.VerifyRequest(c.Request.URL.Path, c.Query(urlsign.QueryExp), c.Query(urlsign.QuerySig)) != nil {
			response.Error(c, http.StatusForbidden, "FORBIDDEN",
				"missing, expired or invalid download signature", nil)
			return
		}
	}

	hwRev := c.GetHeader("X-Hw-Rev")
	if hwRev == "" {
		hwRev = c.Query("hw_rev")
	}

	dl, err := h.projects.GetArtifactDownload(c.Request.Context(), c.Param("project_ref"), artifactRef, hwRev)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProjectNotFound):
			response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error(), nil)
		case errors.Is(err, service.ErrArtifactNotFound):
			response.Error(c, http.StatusNotFound, "NOT_FOUND", "artifact not found", nil)
		case errors.Is(err, service.ErrHwRevIncompatible):
			response.Error(c, http.StatusConflict, "HW_REV_INCOMPATIBLE", "hardware revision is incompatible with this artifact variant", nil)
		default:
			response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		}
		return
	}

	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Type", dl.ContentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", dl.FileName))

	rangeHeader := c.GetHeader("Range")
	if rangeHeader != "" {
		start, end, ok := parseRange(rangeHeader, dl.Size)
		if !ok {
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", dl.Size))
			c.Status(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		length := end - start + 1
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, dl.Size))
		c.Header("Content-Length", strconv.FormatInt(length, 10))
		c.Status(http.StatusPartialContent)

		if c.Request.Method == http.MethodHead {
			return
		}

		rc, err := h.projects.OpenStoredRange(c.Request.Context(), dl.StorageKey, start, end)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read byte range", nil)
			return
		}
		defer rc.Close()

		_, _ = io.Copy(c.Writer, rc)
		return
	}

	c.Header("Content-Length", strconv.FormatInt(dl.Size, 10))
	c.Status(http.StatusOK)

	if c.Request.Method == http.MethodHead {
		return
	}

	rc, err := h.projects.OpenStoredObject(c.Request.Context(), dl.StorageKey)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read object", nil)
		return
	}
	defer rc.Close()

	_, _ = io.Copy(c.Writer, rc)
}

// parseRange 解析 HTTP Range 请求标头：
// 支持 "bytes=start-end"、"bytes=start-" 以及后缀 "bytes=-suffixLen"。
func parseRange(header string, size int64) (int64, int64, bool) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(header, "bytes=")
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	startStr, endStr := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if startStr == "" {
		suffixLen, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || suffixLen <= 0 {
			return 0, 0, false
		}
		if suffixLen > size {
			suffixLen = size
		}
		return size - suffixLen, size - 1, true
	}
	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	if endStr == "" {
		return start, size - 1, true
	}
	end, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	if end >= size {
		end = size - 1
	}
	return start, end, true
}
