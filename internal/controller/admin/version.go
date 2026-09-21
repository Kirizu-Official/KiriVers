package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type versionWriteReq struct {
	Channel             string                 `json:"channel"`
	VersionInteger      *int64                 `json:"version_integer"`
	VersionSemver       *string                `json:"version_semver"`
	Changelog           *string                `json:"changelog"`
	ChangelogI18n       model.ChangelogMap     `json:"changelog_i18n"`
	GitTag              *string                `json:"git_tag"`
	GitCommit           *string                `json:"git_commit"`
	GitLogFrom          *string                `json:"git_log_from"`
	GitLogTo            *string                `json:"git_log_to"`
	IsLTS               *bool                  `json:"is_lts"`
	IsCritical          *bool                  `json:"is_critical"`
	GrayStartPercent    *int                   `json:"gray_start_percent"`
	GrayStepPercent     *int                   `json:"gray_step_percent"`
	GrayIntervalSeconds *int                   `json:"gray_interval_seconds"`
	MinSourceVersion    *string                `json:"min_source_version"`
	AutoPublishWhen     *model.AutoPublishRule `json:"auto_publish_when"`
}

type versionPatchReq struct {
	VersionInteger   *int64                 `json:"version_integer"`
	VersionSemver    *string                `json:"version_semver"`
	Changelog        *string                `json:"changelog"`
	ChangelogI18n    model.ChangelogMap     `json:"changelog_i18n"`
	GitTag           *string                `json:"git_tag"`
	GitCommit        *string                `json:"git_commit"`
	GitLogFrom       *string                `json:"git_log_from"`
	GitLogTo         *string                `json:"git_log_to"`
	IsLTS            *bool                  `json:"is_lts"`
	IsCritical       *bool                  `json:"is_critical"`
	MinSourceVersion *string                `json:"min_source_version"`
	AutoPublishWhen  *model.AutoPublishRule `json:"auto_publish_when"`
}

type versionLineWriteReq struct {
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	MinOS         *string `json:"min_os"`
	MinAPILevel   *int    `json:"min_api_level"`
	PlatformNotes string  `json:"platform_notes"`
}

func publicVersion(v *model.Version, lines []model.VersionLine) gin.H {
	h := gin.H{
		"id":                    v.ID,
		"project_id":            v.ProjectID,
		"channel":               v.ChannelSlug,
		"version_integer":       v.VersionInteger,
		"version_semver":        v.VersionSemver,
		"status":                v.Status,
		"changelog":             v.Changelog,
		"is_lts":                v.IsLTS,
		"is_critical":           v.IsCritical,
		"gray_start_percent":    v.GrayStartPercent,
		"gray_step_percent":     v.GrayStepPercent,
		"gray_interval_seconds": v.GrayIntervalSeconds,
		"created_at":            formatTimeUTC(v.CreatedAt),
		"updated_at":            formatTimeUTC(v.UpdatedAt),
	}
	if v.GrayStartedAt != nil {
		h["gray_started_at"] = formatTimeUTC(*v.GrayStartedAt)
	}
	if v.GrayCompletedAt != nil {
		h["gray_completed_at"] = formatTimeUTC(*v.GrayCompletedAt)
	}
	if v.GitTag != nil {
		h["git_tag"] = *v.GitTag
	}
	if v.GitCommit != nil {
		h["git_commit"] = *v.GitCommit
	}
	if v.GitLogFrom != nil {
		h["git_log_from"] = *v.GitLogFrom
	}
	if v.GitLogTo != nil {
		h["git_log_to"] = *v.GitLogTo
	}
	if v.MinSourceVersion != nil {
		h["min_source_version"] = *v.MinSourceVersion
	}
	if v.PublishTime != nil {
		h["publish_time"] = formatTimeUTC(*v.PublishTime)
	}
	if v.AutoPublishWhen != nil {
		h["auto_publish_when"] = v.AutoPublishWhen
	}
	if lines != nil {
		lineItems := make([]gin.H, 0, len(lines))
		for _, l := range lines {
			lineItems = append(lineItems, publicVersionLine(&l, nil))
		}
		h["lines"] = lineItems
	}
	return h
}

// versionJSON 是带实际发布率的版本 JSON（列表 chip / 详情条）。
func (h *projectHandler) versionJSON(c *gin.Context, v *model.Version, lines []model.VersionLine) gin.H {
	out := publicVersion(v, lines)
	if v != nil {
		h.attachGrayActual(c, []model.Version{*v}, []gin.H{out})
	}
	return out
}

func (h *projectHandler) attachGrayActual(c *gin.Context, versions []model.Version, items []gin.H) {
	if h.projects == nil || len(versions) == 0 || len(versions) != len(items) {
		return
	}
	percents, err := h.projects.GrayActualPercents(c.Request.Context(), versions[0].ProjectID, versions)
	if err != nil {
		return
	}
	for i := range items {
		if p, ok := percents[versions[i].ID]; ok {
			items[i]["actual_percent"] = p
		}
	}
}

func publicVersionLine(l *model.VersionLine, artifacts []model.Artifact) gin.H {
	h := gin.H{
		"id":             l.ID,
		"version_id":     l.VersionID,
		"project_id":     l.ProjectID,
		"os":             l.OS,
		"arch":           l.Arch,
		"status":         l.Status,
		"packs_ready_at": nil,
		"created_at":     formatTimeUTC(l.CreatedAt),
		"updated_at":     formatTimeUTC(l.UpdatedAt),
	}
	if l.PacksReadyAt != nil {
		h["packs_ready_at"] = formatTimeUTC(*l.PacksReadyAt)
	}
	if l.RootHash != "" {
		h["root_hash"] = l.RootHash
	}
	if l.MinOS != nil {
		h["min_os"] = *l.MinOS
	}
	if l.MinAPILevel != nil {
		h["min_api_level"] = *l.MinAPILevel
	}
	if l.PlatformNotes != "" {
		h["platform_notes"] = l.PlatformNotes
	}
	if artifacts != nil {
		items := make([]gin.H, 0, len(artifacts))
		for i := range artifacts {
			items = append(items, publicArtifact(&artifacts[i]))
		}
		h["artifacts"] = items
	}
	return h
}

func writeVersionErr(c *gin.Context, err error) {
	var conflictErr *service.VersionConflictError
	switch {
	case errors.As(err, &conflictErr):
		response.Error(c, http.StatusConflict, "VERSION_ALREADY_EXISTS", err.Error(), conflictErr.Field)
	case errors.Is(err, service.ErrVersionNotFound):
		response.Error(c, http.StatusNotFound, "VERSION_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrVersionLineNotFound):
		response.Error(c, http.StatusNotFound, "VERSION_LINE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrEngineMismatch):
		response.Error(c, http.StatusBadRequest, "ENGINE_MISMATCH", err.Error(), nil)
	case errors.Is(err, service.ErrChannelSuffixMismatch):
		response.Error(c, http.StatusBadRequest, "CHANNEL_SUFFIX_MISMATCH", err.Error(), nil)
	case errors.Is(err, service.ErrChannelConflict):
		response.Error(c, http.StatusConflict, "CHANNEL_CONFLICT", err.Error(), nil)
	case errors.Is(err, service.ErrArtifactRequired):
		response.Error(c, http.StatusConflict, "ARTIFACT_REQUIRED", err.Error(), nil)
	case errors.Is(err, service.ErrGrayNotAllowedOnCritical):
		response.Error(c, http.StatusBadRequest, "GRAY_NOT_ALLOWED_ON_CRITICAL", err.Error(), nil)
	case errors.Is(err, service.ErrIntermediateUnavailable):
		response.Error(c, http.StatusConflict, "INTERMEDIATE_UNAVAILABLE", err.Error(), nil)
	case errors.Is(err, service.ErrUploadIncomplete):
		response.Error(c, http.StatusConflict, "UPLOAD_INCOMPLETE", err.Error(), nil)
	case errors.Is(err, service.ErrInvalidVersionTransition), errors.Is(err, service.ErrPublishedVersionImmutable):
		response.Error(c, http.StatusConflict, "INVALID_TRANSITION", err.Error(), nil)
	case errors.Is(err, service.ErrVersionLineAlreadyExists):
		response.Error(c, http.StatusConflict, "VERSION_LINE_ALREADY_EXISTS", err.Error(), nil)
	case errors.Is(err, service.ErrChannelNotFound):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	case errors.Is(err, pathutil.ErrInvalidPath), errors.Is(err, service.ErrDuplicateManifestPath):
		response.Error(c, http.StatusBadRequest, "INVALID_PATH", err.Error(), nil)
	case errors.Is(err, service.ErrZipManifestMismatch):
		response.Error(c, http.StatusBadRequest, "ZIP_MANIFEST_MISMATCH", err.Error(), nil)
	case errors.Is(err, service.ErrArchiveRequired):
		response.Error(c, http.StatusConflict, "ARCHIVE_REQUIRED", err.Error(), nil)
	case errors.Is(err, service.ErrAutoPublishPending):
		response.Error(c, http.StatusConflict, "AUTO_PUBLISH_PENDING", err.Error(), nil)
	case errors.Is(err, service.ErrZipLayoutInvalid):
		response.Error(c, http.StatusBadRequest, "ZIP_LAYOUT_INVALID", err.Error(), nil)
	case errors.Is(err, service.ErrJobNotFound):
		response.Error(c, http.StatusNotFound, "JOB_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrJobForbidden):
		response.Error(c, http.StatusForbidden, "FORBIDDEN", err.Error(), nil)
	case errors.Is(err, service.ErrJobFailed):
		response.Error(c, http.StatusInternalServerError, "JOB_FAILED", err.Error(), nil)
	case service.IsInvalidRequest(err):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	default:
		writeProjectErr(c, err)
	}
}

func (h *projectHandler) existsVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	exists, v, lines, err := h.projects.CheckVersionExists(c.Request.Context(), p.ID, c.Param("version"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	if !exists {
		response.JSON(c, http.StatusOK, gin.H{"exists": false})
		return
	}
	lineItems := make([]gin.H, 0, len(lines))
	for _, l := range lines {
		lineItems = append(lineItems, gin.H{
			"os":     l.OS,
			"arch":   l.Arch,
			"status": l.Status,
		})
	}
	response.JSON(c, http.StatusOK, gin.H{
		"exists":          true,
		"status":          v.Status,
		"channel":         v.ChannelSlug,
		"version_integer": v.VersionInteger,
		"version_semver":  v.VersionSemver,
		"lines":           lineItems,
	})
}

func (h *projectHandler) putVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req versionWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	v, created, err := h.projects.PutVersion(c.Request.Context(), p.ID, c.Param("version"), service.VersionWriteInput{
		Channel:             req.Channel,
		VersionInteger:      req.VersionInteger,
		VersionSemver:       req.VersionSemver,
		Changelog:           req.Changelog,
		ChangelogI18n:       req.ChangelogI18n,
		GitTag:              req.GitTag,
		GitCommit:           req.GitCommit,
		GitLogFrom:          req.GitLogFrom,
		GitLogTo:            req.GitLogTo,
		IsLTS:               req.IsLTS,
		IsCritical:          req.IsCritical,
		GrayStartPercent:    req.GrayStartPercent,
		GrayStepPercent:     req.GrayStepPercent,
		GrayIntervalSeconds: req.GrayIntervalSeconds,
		MinSourceVersion:    req.MinSourceVersion,
		AutoPublishWhen:     req.AutoPublishWhen,
	})
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	lines, _ := h.projects.ListVersionLines(c.Request.Context(), p.ID, c.Param("version"))
	response.JSON(c, status, h.versionJSON(c, v, lines))
}

func (h *projectHandler) getVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	v, err := h.projects.ResolveVersion(c.Request.Context(), p.ID, c.Param("version"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	lines, _ := h.projects.ListVersionLines(c.Request.Context(), p.ID, c.Param("version"))
	response.JSON(c, http.StatusOK, h.versionJSON(c, v, lines))
}

func (h *projectHandler) listVersions(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	osQ := strings.TrimSpace(c.Query("os"))
	archQ := strings.TrimSpace(c.Query("arch"))
	if strings.TrimSpace(c.Query("latest")) == "true" || strings.TrimSpace(c.Query("latest")) == "1" {
		if osQ == "" || archQ == "" {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "latest requires os and arch", nil)
			return
		}
		list, latest, err := h.projects.ListPublishedReadyVersionsForPlatform(c.Request.Context(), p, osQ, archQ)
		if err != nil {
			writeVersionErr(c, err)
			return
		}
		items := make([]gin.H, 0, len(list))
		for i := range list {
			items = append(items, publicVersion(&list[i], nil))
		}
		h.attachGrayActual(c, list, items)
		response.JSON(c, http.StatusOK, gin.H{"versions": items, "latest": latest})
		return
	}
	list, err := h.projects.ListVersionsFiltered(c.Request.Context(), p.ID, service.VersionListFilter{
		Channel: strings.TrimSpace(c.Query("channel")),
		Status:  strings.TrimSpace(c.Query("status")),
		OS:      osQ,
		Arch:    archQ,
	})
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicVersion(&list[i], nil))
	}
	h.attachGrayActual(c, list, items)
	response.JSON(c, http.StatusOK, gin.H{"versions": items})
}

func (h *projectHandler) patchVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req versionPatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	v, err := h.projects.PatchVersion(c.Request.Context(), p.ID, c.Param("version"), service.VersionPatchInput{
		VersionInteger:   req.VersionInteger,
		VersionSemver:    req.VersionSemver,
		Changelog:        req.Changelog,
		ChangelogI18n:    req.ChangelogI18n,
		GitTag:           req.GitTag,
		GitCommit:        req.GitCommit,
		GitLogFrom:       req.GitLogFrom,
		GitLogTo:         req.GitLogTo,
		IsLTS:            req.IsLTS,
		IsCritical:       req.IsCritical,
		MinSourceVersion: req.MinSourceVersion,
		AutoPublishWhen:  req.AutoPublishWhen,
	})
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	lines, _ := h.projects.ListVersionLines(c.Request.Context(), p.ID, c.Param("version"))
	response.JSON(c, http.StatusOK, h.versionJSON(c, v, lines))
}

func (h *projectHandler) deleteVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	if err := h.projects.DeleteVersion(c.Request.Context(), p.ID, c.Param("version")); err != nil {
		writeVersionErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
	// 成功后写审计（§11.2 / C15-3）：资源 ID 用版本引用（行已删除）。
	h.audit.recordAudit(c, &p.ID, model.AuditActionVersionDelete, "version", c.Param("version"), nil)
}

func (h *projectHandler) publishVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req struct {
		RequiredLines []string `json:"required_lines"`
		AllowPartial  *bool    `json:"allow_partial"`
	}
	_ = c.ShouldBindJSON(&req)

	var opts []service.PublishVersionInput
	if len(req.RequiredLines) > 0 || req.AllowPartial != nil {
		opts = append(opts, service.PublishVersionInput{
			RequiredLines: req.RequiredLines,
			AllowPartial:  req.AllowPartial,
		})
	}

	v, err := h.projects.PublishVersion(c.Request.Context(), p.ID, c.Param("version"), opts...)
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	lines, _ := h.projects.ListVersionLines(c.Request.Context(), p.ID, c.Param("version"))
	response.JSON(c, http.StatusOK, h.versionJSON(c, v, lines))
	// 成功后写审计（§11.2 / C15-3）。
	h.audit.recordAudit(c, &p.ID, model.AuditActionVersionPublish, "version", v.ID.String(), nil)
}

func (h *projectHandler) deprecateVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	v, err := h.projects.DeprecateVersion(c.Request.Context(), p.ID, c.Param("version"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	lines, _ := h.projects.ListVersionLines(c.Request.Context(), p.ID, c.Param("version"))
	response.JSON(c, http.StatusOK, h.versionJSON(c, v, lines))
	// 成功后写审计（§11.2 / C15-3）。
	h.audit.recordAudit(c, &p.ID, model.AuditActionVersionDeprecate, "version", v.ID.String(), nil)
}

func (h *projectHandler) revokeVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	v, err := h.projects.RevokeVersion(c.Request.Context(), p.ID, c.Param("version"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	lines, _ := h.projects.ListVersionLines(c.Request.Context(), p.ID, c.Param("version"))
	response.JSON(c, http.StatusOK, h.versionJSON(c, v, lines))
	// 成功后写审计（§11.2 / C15-3）。
	h.audit.recordAudit(c, &p.ID, model.AuditActionVersionRevoke, "version", v.ID.String(), nil)
}

// promoteVersionReq 是渠道晋升请求体（C14-2，§4.3 / §5.8）。
type promoteVersionReq struct {
	TargetChannel string `json:"target_channel"`
}

// promoteVersion 处理 POST /versions/:version/promote（C14-2）。
// 后缀规则允许 → 直改渠道；冲突 → 400 CHANNEL_SUFFIX_MISMATCH，
// details 提示「新建更高比较键 Version + artifacts/reuse 复用 blob」。
func (h *projectHandler) promoteVersion(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req promoteVersionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	v, err := h.projects.PromoteVersion(c.Request.Context(), p.ID, c.Param("version"), service.PromoteVersionInput{
		TargetChannel: req.TargetChannel,
	})
	if err != nil {
		var suffixErr *service.ChannelSuffixMismatchError
		if errors.As(err, &suffixErr) {
			response.Error(c, http.StatusBadRequest, "CHANNEL_SUFFIX_MISMATCH", err.Error(), gin.H{
				"remedy":         "create a new version with a higher compare key on the target channel and reuse artifacts via POST /api/v1/admin/projects/{project_ref}/versions/{version}/artifacts/reuse",
				"target_channel": req.TargetChannel,
			})
			return
		}
		writeVersionErr(c, err)
		return
	}
	lines, _ := h.projects.ListVersionLines(c.Request.Context(), p.ID, c.Param("version"))
	response.JSON(c, http.StatusOK, h.versionJSON(c, v, lines))
	// 成功后写审计（C14-2 渠道晋升，§11.2 / C15-3）。
	h.audit.recordAudit(c, &p.ID, model.AuditActionVersionPromote, "version", v.ID.String(),
		gin.H{"target_channel": req.TargetChannel})
}

func (h *projectHandler) createVersionLine(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req versionLineWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	rows, err := h.projects.ListMatrix(c.Request.Context(), p.ID)
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	if len(rows) > 0 {
		osSlug := platform.CanonicalOSWrite(req.OS)
		archSlug := platform.CanonicalArch(req.Arch)
		found := false
		for i := range rows {
			if rows[i].OS == osSlug && rows[i].Arch == archSlug {
				found = true
				break
			}
		}
		if !found {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "os and arch must exist on the project matrix", nil)
			return
		}
	}
	line, err := h.projects.AddVersionLine(c.Request.Context(), p.ID, c.Param("version"), service.VersionLineWriteInput{
		OS:            req.OS,
		Arch:          req.Arch,
		MinOS:         req.MinOS,
		MinAPILevel:   req.MinAPILevel,
		PlatformNotes: req.PlatformNotes,
	})
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicVersionLine(line, nil))
}

func (h *projectHandler) listVersionLines(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	rows, err := h.projects.ListVersionLinesWithArtifacts(c.Request.Context(), p.ID, c.Param("version"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		items = append(items, publicVersionLine(&rows[i].Line, rows[i].Artifacts))
	}
	response.JSON(c, http.StatusOK, gin.H{"lines": items})
}

func (h *projectHandler) getVersionLine(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	line, err := h.projects.GetVersionLine(c.Request.Context(), p.ID, c.Param("version"), c.Param("os"), c.Param("arch"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicVersionLine(line, nil))
}

func (h *projectHandler) readyVersionLine(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	line, err := h.projects.ReadyVersionLine(c.Request.Context(), p.ID, c.Param("version"), c.Param("os"), c.Param("arch"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicVersionLine(line, nil))
}

func (h *projectHandler) lineDefaults(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	osQ := strings.TrimSpace(c.Query("os"))
	archQ := strings.TrimSpace(c.Query("arch"))
	if osQ == "" || archQ == "" {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "os and arch are required", nil)
		return
	}
	minOS, minAPI, err := h.projects.LineDefaults(c.Request.Context(), p, c.Param("version"), osQ, archQ)
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{
		"min_os":        minOS,
		"min_api_level": minAPI,
	})
}

func (h *projectHandler) yankVersionLine(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	line, err := h.projects.YankVersionLine(c.Request.Context(), p.ID, c.Param("version"), c.Param("os"), c.Param("arch"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicVersionLine(line, nil))
}

func (h *projectHandler) disableVersionLine(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	line, err := h.projects.DisableVersionLine(c.Request.Context(), p.ID, c.Param("version"), c.Param("os"), c.Param("arch"))
	if err != nil {
		writeVersionErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicVersionLine(line, nil))
}
