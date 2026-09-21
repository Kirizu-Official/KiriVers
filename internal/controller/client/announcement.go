package client

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// announcementHandler 承载 GET /projects/:project_ref/announcements。
type announcementHandler struct {
	announcements *service.AnnouncementService
}

func (h *announcementHandler) list(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		announcementError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found")
		return
	}
	res, err := h.announcements.ListClient(c.Request.Context(), p, service.ClientListInput{
		Version:        strings.TrimSpace(c.Query("version")),
		OS:             c.Query("os"),
		Arch:           c.Query("arch"),
		Locale:         c.Query("locale"),
		AcceptLanguage: c.GetHeader("Accept-Language"),
		Now:            time.Now().UTC(),
	})
	if err != nil {
		writeAnnouncementClientErr(c, err)
		return
	}

	c.Header("Vary", strings.Join(service.AppendVaryReferer(res.Vary), ", "))
	if update.MatchesETag(c.GetHeader("If-None-Match"), res.ETag) {
		writeCachedHeaders(c, res.ETag, res.CacheControl)
		c.Status(http.StatusNotModified)
		return
	}
	writeCachedHeaders(c, res.ETag, res.CacheControl)
	items := make([]gin.H, 0, len(res.Items))
	origin := markdownOrigin(c)
	for i := range res.Items {
		item := res.Items[i]
		item.Markdown = service.ExpandSiteURL(item.Markdown, origin)
		items = append(items, publicClientAnnouncement(&item))
	}
	response.JSON(c, http.StatusOK, gin.H{"announcements": items})
}

func publicClientAnnouncement(a *service.ClientAnnouncement) gin.H {
	body := gin.H{
		"id":       a.ID,
		"title":    a.Title,
		"subtitle": a.Subtitle,
		"markdown": a.Markdown,
		"locale":   a.Locale,
	}
	if a.StartsAt != nil {
		body["starts_at"] = a.StartsAt.UTC().Format("2006-01-02T15:04:05Z")
	} else {
		body["starts_at"] = nil
	}
	if a.EndsAt != nil {
		body["ends_at"] = a.EndsAt.UTC().Format("2006-01-02T15:04:05Z")
	} else {
		body["ends_at"] = nil
	}
	return body
}

func writeAnnouncementClientErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidQueryParam):
		announcementError(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error())
	case errors.Is(err, service.ErrProjectNotFound):
		announcementError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
	default:
		announcementError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func announcementError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "private, no-store")
	response.Error(c, status, code, message, nil)
}
