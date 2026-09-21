package client

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// RegisterMedia 挂载客户端 UUID 公开的媒体 GET/HEAD（ClientProjectResolve，无 Token / urlsign）。
func RegisterMedia(rg *gin.RouterGroup, projects *service.ProjectService, media *service.MediaService, limiter *middleware.Limiter) {
	if media == nil || projects == nil {
		return
	}
	h := &mediaHandler{media: media}
	g := rg.Group("/projects/:project_ref")
	g.Use(middleware.ClientProjectResolve(projects))
	g.GET("/media/:id", middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP), h.get)
	g.HEAD("/media/:id", middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP), h.get)
}

type mediaHandler struct {
	media *service.MediaService
}

func (h *mediaHandler) get(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusNotFound, "NOT_FOUND", "media not found", nil)
		return
	}
	meta, err := h.media.GetMeta(c.Request.Context(), p.ID, id)
	if err != nil {
		writeMediaClientErr(c, err)
		return
	}

	disposition := "attachment"
	if model.MediaInline(meta.ContentType) {
		disposition = "inline"
	}
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Type", meta.ContentType)
	c.Header("Content-Disposition", mediaContentDisposition(disposition, meta.FileName))
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	setMediaChecksumHeaders(c, meta)

	rangeHeader := c.GetHeader("Range")
	if rangeHeader != "" {
		start, end, ok := parseRange(rangeHeader, meta.Size)
		if !ok {
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", meta.Size))
			c.Status(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		length := end - start + 1
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, meta.Size))
		c.Header("Content-Length", strconv.FormatInt(length, 10))
		c.Status(http.StatusPartialContent)
		if c.Request.Method == http.MethodHead {
			return
		}
		rc, err := h.media.Open(c.Request.Context(), meta, start, end)
		if err != nil {
			writeMediaClientErr(c, err)
			return
		}
		defer rc.Close()
		_, _ = io.Copy(c.Writer, rc)
		return
	}

	c.Header("Content-Length", strconv.FormatInt(meta.Size, 10))
	c.Status(http.StatusOK)
	if c.Request.Method == http.MethodHead {
		return
	}
	rc, err := h.media.Open(c.Request.Context(), meta, 0, -1)
	if err != nil {
		writeMediaClientErr(c, err)
		return
	}
	defer rc.Close()
	_, _ = io.Copy(c.Writer, rc)
}

func setMediaChecksumHeaders(c *gin.Context, meta *model.ProjectMedia) {
	if meta == nil {
		return
	}
	sha := strings.TrimSpace(meta.SHA256)
	if sha != "" {
		c.Header("ETag", strconv.Quote(sha))
		if raw, err := hex.DecodeString(sha); err == nil && len(raw) > 0 {
			c.Header("Digest", "sha-256="+base64.StdEncoding.EncodeToString(raw))
		}
	}
	md5hex := strings.TrimSpace(meta.MD5)
	if md5hex != "" {
		if raw, err := hex.DecodeString(md5hex); err == nil && len(raw) > 0 {
			c.Header("Content-MD5", base64.StdEncoding.EncodeToString(raw))
		}
	}
}

// mediaContentDisposition 用 project_media.file_name 生成 Content-Disposition。
// ASCII 文件名走 quoted filename=；含非 ASCII 时另加 RFC 5987 filename*（同一列）。
func mediaContentDisposition(disposition, fileName string) string {
	ascii := asciiDispositionFileName(fileName)
	header := disposition + "; filename=" + strconv.Quote(ascii)
	if fileName != ascii {
		header += "; filename*=UTF-8''" + rfc5987Encode(fileName)
	}
	return header
}

func asciiDispositionFileName(name string) string {
	if name == "" {
		return "file"
	}
	var b strings.Builder
	for _, r := range name {
		if r < unicode.MaxASCII && r >= 0x20 && r != '"' && r != '\\' && r != ';' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "file"
	}
	return out
}

func rfc5987Encode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '!' || c == '#' || c == '$' || c == '&' || c == '+' || c == '-' ||
			c == '.' || c == '^' || c == '_' || c == '`' || c == '|' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func writeMediaClientErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrProjectNotFound):
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrMediaNotFound):
		response.Error(c, http.StatusNotFound, "NOT_FOUND", "media not found", nil)
	case errors.Is(err, service.ErrMediaStorageUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "NOT_READY", err.Error(), nil)
	default:
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}
