package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// announcementHandler 管理端公告 CRUD / 重排。
type announcementHandler struct {
	projects      *service.ProjectService
	announcements *service.AnnouncementService
	audit         *auditHelper
}

type announcementWriteReq struct {
	VersionID *string `json:"version_id"`
	OS        *string `json:"os"`
	Arch      *string `json:"arch"`
	Language  string  `json:"language"`
	Title     string  `json:"title"`
	Subtitle  string  `json:"subtitle"`
	Content   string  `json:"content"`
	StartsAt  *string `json:"starts_at"`
	EndsAt    *string `json:"ends_at"`
}

type optionalJSON struct {
	set   bool
	null  bool
	value string
}

func (o *optionalJSON) UnmarshalJSON(data []byte) error {
	o.set = true
	if string(data) == "null" {
		o.null = true
		o.value = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	o.value = s
	return nil
}

type optionalIntJSON struct {
	set   bool
	null  bool
	value *int
}

func (o *optionalIntJSON) UnmarshalJSON(data []byte) error {
	o.set = true
	if string(data) == "null" {
		o.null = true
		o.value = nil
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	o.value = &n
	return nil
}

type announcementPatchReq struct {
	Status    *string      `json:"status"`
	VersionID optionalJSON `json:"version_id"`
	OS        optionalJSON `json:"os"`
	Arch      optionalJSON `json:"arch"`
	Language  *string      `json:"language"`
	Title     *string      `json:"title"`
	Subtitle  *string      `json:"subtitle"`
	Content   *string      `json:"content"`
	StartsAt  optionalJSON `json:"starts_at"`
	EndsAt    optionalJSON `json:"ends_at"`
}

type announcementReorderReq struct {
	IDs []string `json:"ids"`
}

func publicAnnouncement(a *model.Announcement, includeContent bool) gin.H {
	body := gin.H{
		"id":         a.ID,
		"status":     a.Status,
		"sort_order": a.SortOrder,
		"language":   a.Language,
		"title":      a.Title,
		"subtitle":   a.Subtitle,
		"created_at": formatTimeUTC(a.CreatedAt),
		"updated_at": formatTimeUTC(a.UpdatedAt),
	}
	if includeContent {
		body["content"] = a.Content
	}
	if a.VersionID != nil {
		body["version_id"] = a.VersionID.String()
	} else {
		body["version_id"] = nil
	}
	if a.OS != "" {
		body["os"] = a.OS
	} else {
		body["os"] = nil
	}
	if a.Arch != "" {
		body["arch"] = a.Arch
	} else {
		body["arch"] = nil
	}
	if a.StartsAt != nil {
		body["starts_at"] = formatTimeUTC(*a.StartsAt)
	} else {
		body["starts_at"] = nil
	}
	if a.EndsAt != nil {
		body["ends_at"] = formatTimeUTC(*a.EndsAt)
	} else {
		body["ends_at"] = nil
	}
	return body
}

func publicAnnouncementList(list []model.Announcement) []gin.H {
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicAnnouncement(&list[i], false))
	}
	return items
}

func (h *announcementHandler) list(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	list, err := h.announcements.ListAdmin(c.Request.Context(), p.ID)
	if err != nil {
		writeAnnouncementErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"announcements": publicAnnouncementList(list)})
}

func (h *announcementHandler) get(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	id, err := uuid.Parse(c.Param("announcement_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
		return
	}
	row, err := h.announcements.GetAdmin(c.Request.Context(), p.ID, id)
	if err != nil {
		writeAnnouncementErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicAnnouncement(row, true))
}

func (h *announcementHandler) create(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req announcementWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	startsAt, err := parseOptionalRFC3339(derefString(req.StartsAt))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid starts_at", nil)
		return
	}
	endsAt, err := parseOptionalRFC3339(derefString(req.EndsAt))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid ends_at", nil)
		return
	}
	versionID, err := parseOptionalUUID(derefString(req.VersionID))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid version_id", nil)
		return
	}
	row, err := h.announcements.Create(c.Request.Context(), p.ID, service.AnnouncementCreateInput{
		VersionID: versionID,
		OS:        derefString(req.OS),
		Arch:      derefString(req.Arch),
		Language:  req.Language,
		Title:     req.Title,
		Subtitle:  req.Subtitle,
		Content:   req.Content,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
	})
	if err != nil {
		writeAnnouncementErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicAnnouncement(row, true))
	h.audit.recordAudit(c, &p.ID, model.AuditActionAnnouncementCreate, "announcement", row.ID.String(), nil)
}

func (h *announcementHandler) patch(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	id, err := uuid.Parse(c.Param("announcement_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
		return
	}
	var req announcementPatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	in := service.AnnouncementPatchInput{Status: req.Status}
	if req.VersionID.set {
		in.VersionIDSet = true
		if !req.VersionID.null && strings.TrimSpace(req.VersionID.value) != "" {
			id, err := parseOptionalUUID(req.VersionID.value)
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid version_id", nil)
				return
			}
			in.VersionID = id
		}
	}
	if req.OS.set {
		in.OSSet = true
		in.OS = req.OS.value
	}
	if req.Arch.set {
		in.ArchSet = true
		in.Arch = req.Arch.value
	}
	if req.Language != nil {
		in.LanguageSet = true
		in.Language = *req.Language
	}
	if req.Title != nil {
		in.TitleSet = true
		in.Title = *req.Title
	}
	if req.Subtitle != nil {
		in.SubtitleSet = true
		in.Subtitle = *req.Subtitle
	}
	if req.Content != nil {
		in.ContentSet = true
		in.Content = *req.Content
	}
	if req.StartsAt.set {
		in.StartsAtSet = true
		if !req.StartsAt.null && strings.TrimSpace(req.StartsAt.value) != "" {
			t, err := parseOptionalRFC3339(req.StartsAt.value)
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid starts_at", nil)
				return
			}
			in.StartsAt = t
		}
	}
	if req.EndsAt.set {
		in.EndsAtSet = true
		if !req.EndsAt.null && strings.TrimSpace(req.EndsAt.value) != "" {
			t, err := parseOptionalRFC3339(req.EndsAt.value)
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid ends_at", nil)
				return
			}
			in.EndsAt = t
		}
	}
	row, err := h.announcements.Patch(c.Request.Context(), p.ID, id, in)
	if err != nil {
		writeAnnouncementErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicAnnouncement(row, true))
	h.audit.recordAudit(c, &p.ID, model.AuditActionAnnouncementUpdate, "announcement", row.ID.String(), nil)
}

func (h *announcementHandler) remove(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	id, err := uuid.Parse(c.Param("announcement_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
		return
	}
	if err := h.announcements.Delete(c.Request.Context(), p.ID, id); err != nil {
		writeAnnouncementErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
	h.audit.recordAudit(c, &p.ID, model.AuditActionAnnouncementDelete, "announcement", id.String(), nil)
}

func (h *announcementHandler) reorder(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req announcementReorderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	ids := make([]uuid.UUID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
			return
		}
		ids = append(ids, id)
	}
	if err := h.announcements.Reorder(c.Request.Context(), p.ID, ids); err != nil {
		writeAnnouncementErr(c, err)
		return
	}
	list, err := h.announcements.ListAdmin(c.Request.Context(), p.ID)
	if err != nil {
		writeAnnouncementErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"announcements": publicAnnouncementList(list)})
	h.audit.recordAudit(c, &p.ID, model.AuditActionAnnouncementReorder, "announcement", p.ID.String(), gin.H{"count": len(ids)})
}

func writeAnnouncementErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAnnouncementNotFound):
		response.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrVersionNotFound):
		response.Error(c, http.StatusNotFound, "VERSION_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrProjectNotFound):
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error(), nil)
	case service.IsInvalidRequest(err):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	default:
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseOptionalRFC3339(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, err
		}
	}
	utc := t.UTC()
	return &utc, nil
}
