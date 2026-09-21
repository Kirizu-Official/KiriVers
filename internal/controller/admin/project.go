package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type projectHandler struct {
	projects *service.ProjectService
	// audit 供管理动作审计（§11.2 / C15-3，nil 安全）。
	audit *auditHelper
}

type projectWriteReq struct {
	Name                          *string           `json:"name"`
	Slug                          *string           `json:"slug"`
	CompareEngine                 *string           `json:"compare_engine"`
	MinimumSupportedVersion       *string           `json:"minimum_supported_version"`
	DefaultLocale                 *string           `json:"default_locale"`
	RequireClientToken            *bool             `json:"require_client_token"`
	StoreToken                    *string           `json:"store_token"`
	ForceHTTPS                    *bool             `json:"force_https"`
	CORSOrigins                   *[]string         `json:"cors_origins"`
	DeviceIDPolicy                *string           `json:"device_id_policy"`
	GrayWeightTenureActivity      *bool             `json:"gray_weight_tenure_activity"`
	StorageVisibility             *string           `json:"storage_visibility"`
	StoragePrefix                 *string           `json:"storage_prefix"`
	StorageBucket                 *string           `json:"storage_bucket"`
	WebhookURL                    *string           `json:"webhook_url"`
	WebhookSecretReset            *bool             `json:"webhook_secret_reset"`
	SigningAlgo                   *string           `json:"signing_algo"`
	SigningPublicKey              *string           `json:"signing_public_key"`
	SigningPrivateKey             *string           `json:"signing_private_key"`
	ChangelogScope                *string           `json:"changelog_scope"`
	ChangelogLayout               *string           `json:"changelog_layout"`
	ChangelogClientOverride       *bool             `json:"changelog_client_override"`
	ChangelogIncludeRevoked       *bool             `json:"changelog_include_revoked"`
	ChangelogIncludePlatformNotes *bool             `json:"changelog_include_platform_notes"`
	ChangelogDefaultEntries       *int              `json:"changelog_default_entries"`
	ChangelogMaxEntries           *int              `json:"changelog_max_entries"`
	RateLimit                     *model.JSONObject `json:"rate_limit"`
	CacheSMaxageSeconds           *int              `json:"cache_s_maxage_seconds"`
	SlugAliasRetentionDays        *int              `json:"slug_alias_retention_days"`
	TelemetryRetentionDays        *int              `json:"telemetry_retention_days"`
	// SignedURLTTLSeconds 私有存储短时签名 URL 项目级 TTL 秒数（§13.7 / C15-1）；
	// 0 = 回退实例默认 3600。
	SignedURLTTLSeconds *int                      `json:"signed_url_ttl_seconds"`
	FileListMaxFiles    *int                      `json:"file_list_max_files"`
	SystemChannelNames  *model.SystemChannelNames `json:"system_channel_names"`
	BootstrapAdminToken *string                   `json:"bootstrap_admin_token"`
	OwnerUsername       *string                   `json:"owner_username"`
	OwnerPassword       *string                   `json:"owner_password"`
}

type createTokenReq struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at"`
}

func (h *projectHandler) create(c *gin.Context) {
	var req projectWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	if req.Slug == nil || strings.TrimSpace(*req.Slug) == "" {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "slug is required", nil)
		return
	}
	p, storePlain, err := h.projects.Create(c.Request.Context(), req.toInput())
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, h.projectJSON(p, storePlain, nil))
}

func (h *projectHandler) list(c *gin.Context) {
	admin := middleware.AdminFrom(c)
	if admin == nil {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	list, err := h.projects.ListVisible(c.Request.Context(), admin)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	stats, err := projectStatsMap(c.Request.Context(), h.projects, list)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		st := stats[list[i].ID]
		items = append(items, h.projectJSON(&list[i], "", &st))
	}
	response.JSON(c, http.StatusOK, gin.H{"projects": items})
}

func (h *projectHandler) get(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	stats, err := projectStatsMap(c.Request.Context(), h.projects, []model.Project{*p})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	st := stats[p.ID]
	response.JSON(c, http.StatusOK, h.projectJSON(p, "", &st))
}

func (h *projectHandler) patch(c *gin.Context) {
	var req projectWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	p, storePlain, err := h.projects.Patch(c.Request.Context(), c.Param("project_ref"), req.toInput())
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	stats, err := projectStatsMap(c.Request.Context(), h.projects, []model.Project{*p})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	st := stats[p.ID]
	response.JSON(c, http.StatusOK, h.projectJSON(p, storePlain, &st))
	// 成功后写审计（§11.2）：项目配置变更（含 webhook URL / 密钥重置，C14-3）。
	h.audit.recordAudit(c, &p.ID, model.AuditActionProjectUpdate, "project", p.ID.String(), nil)
}

func (h *projectHandler) remove(c *gin.Context) {
	if err := h.projects.SoftDelete(c.Request.Context(), c.Param("project_ref")); err != nil {
		writeProjectErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *projectHandler) createToken(c *gin.Context) {
	var req createTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid expires_at", nil)
			return
		}
		utc := t.UTC()
		exp = &utc
	}
	issued, err := h.projects.CreateToken(c.Request.Context(), c.Param("project_ref"), req.Name, req.Scopes, exp)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	body := publicToken(issued.Token)
	body["token"] = issued.Plaintext
	response.JSON(c, http.StatusCreated, body)
	// 成功后写审计（C15-3）：只记 Token ID/名称，明文绝不入审计（C15-5）。
	h.audit.recordAudit(c, &issued.Token.ProjectID, model.AuditActionProjectTokenCreate,
		"project_token", issued.Token.ID.String(), gin.H{"name": issued.Token.Name})
}

func (h *projectHandler) listTokens(c *gin.Context) {
	list, err := h.projects.ListTokens(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicToken(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"tokens": items})
}

func (h *projectHandler) deleteToken(c *gin.Context) {
	id, err := uuid.Parse(c.Param("token_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid token id", nil)
		return
	}
	if err := h.projects.DeleteToken(c.Request.Context(), c.Param("project_ref"), id); err != nil {
		writeProjectErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
	// 成功后写审计（C15-3）。
	h.audit.recordAudit(c, h.projectIDFromRef(c), model.AuditActionProjectTokenDelete, "project_token", id.String(), nil)
}

// projectIDFromRef 尽力解析当前路由的 :project_ref；失败返回 nil（审计
// ProjectID 可空，主体与动作照记）。
func (h *projectHandler) projectIDFromRef(c *gin.Context) *uuid.UUID {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		return nil
	}
	return &p.ID
}

func (r projectWriteReq) toInput() service.CreateProjectInput {
	in := service.CreateProjectInput{
		Slug:                     r.Slug,
		Name:                     r.Name,
		CompareEngine:            r.CompareEngine,
		MinimumSupportedVersion:  r.MinimumSupportedVersion,
		DefaultLocale:            r.DefaultLocale,
		RequireClientToken:       r.RequireClientToken,
		StoreToken:               r.StoreToken,
		ForceHTTPS:               r.ForceHTTPS,
		CORSOrigins:              r.CORSOrigins,
		DeviceIDPolicy:           r.DeviceIDPolicy,
		GrayWeightTenureActivity: r.GrayWeightTenureActivity,
		StorageVisibility:        r.StorageVisibility,
		StoragePrefix:            r.StoragePrefix,
		StorageBucket:            r.StorageBucket,
		WebhookURL:               r.WebhookURL,
		WebhookSecretReset:       r.WebhookSecretReset,
		SigningAlgo:              r.SigningAlgo,
		SigningPublicKey:         r.SigningPublicKey,
		SigningPrivateKey:        r.SigningPrivateKey,
		ChangelogScope:           r.ChangelogScope,
		ChangelogLayout:          r.ChangelogLayout,
		ChangelogClientOverride:  r.ChangelogClientOverride,
		ChangelogIncludeRevoked:  r.ChangelogIncludeRevoked,
		ChangelogIncludeNotes:    r.ChangelogIncludePlatformNotes,
		ChangelogDefaultEntries:  r.ChangelogDefaultEntries,
		ChangelogMaxEntries:      r.ChangelogMaxEntries,
		RateLimit:                r.RateLimit,
		CacheSMaxageSeconds:      r.CacheSMaxageSeconds,
		SlugAliasRetentionDays:   r.SlugAliasRetentionDays,
		TelemetryRetentionDays:   r.TelemetryRetentionDays,
		SignedURLTTLSeconds:      r.SignedURLTTLSeconds,
		FileListMaxFiles:         r.FileListMaxFiles,
		BootstrapAdminToken:      r.BootstrapAdminToken,
		OwnerUsername:            r.OwnerUsername,
		OwnerPassword:            r.OwnerPassword,
	}
	if r.SystemChannelNames != nil {
		in.SystemChannelNames = *r.SystemChannelNames
	}
	return in
}

func (h *projectHandler) projectJSON(p *model.Project, storePlaintext string, stats *service.ProjectStats) gin.H {
	return publicProject(p, storePlaintext, stats, h.projects.FileListMaxFilesLimit(), h.projects.ChangelogDefaultEntriesLimit(), h.projects.ChangelogMaxEntriesLimit(), h.projects.StorageDriver())
}

// publicProject 组装管理端项目 JSON：snake_case、RFC 3339 UTC，永不输出私钥与 token 哈希。
func publicProject(p *model.Project, storePlaintext string, stats *service.ProjectStats, fileListLimit, changelogDefaultLimit, changelogMaxLimit int, storageDriver string) gin.H {
	cors := p.CORSOrigins
	if cors == nil {
		cors = model.StringList{}
	}
	rate := p.RateLimit
	if rate == nil {
		rate = model.JSONObject{}
	}
	body := gin.H{
		"uuid":                             p.ID,
		"name":                             p.DisplayName(),
		"slug":                             p.Slug,
		"compare_engine":                   p.CompareEngine,
		"minimum_supported_version":        p.MinimumSupportedVersion,
		"default_locale":                   p.DefaultLocale,
		"require_client_token":             p.RequireClientToken,
		"has_store_token":                  p.HasStoreToken(),
		"force_https":                      p.ForceHTTPS,
		"cors_origins":                     cors,
		"device_id_policy":                 p.DeviceIDPolicy,
		"gray_weight_tenure_activity":      p.GrayWeightTenureActivity,
		"storage_visibility":               p.StorageVisibility,
		"storage_prefix":                   p.StoragePrefix,
		"storage_bucket":                   p.StorageBucket,
		"storage_driver":                   normalizeStorageDriver(storageDriver),
		"webhook_url":                      p.WebhookURL,
		"signing_algo":                     p.SigningAlgo,
		"signing_public_key":               p.SigningPublicKey,
		"has_signing_private_key":          p.HasSigningPrivateKey(),
		"changelog_scope":                  p.ChangelogScope,
		"changelog_layout":                 p.ChangelogLayout,
		"changelog_client_override":        p.ChangelogClientOverride,
		"changelog_include_revoked":        p.ChangelogIncludeRevoked,
		"changelog_include_platform_notes": p.ChangelogIncludeNotes,
		"changelog_default_entries":        p.ChangelogDefaultEntries,
		"changelog_max_entries":            p.ChangelogMaxEntries,
		"changelog_default_entries_limit":  changelogDefaultLimit,
		"changelog_max_entries_limit":      changelogMaxLimit,
		"rate_limit":                       rate,
		"cache_s_maxage_seconds":           p.CacheSMaxageSeconds,
		"slug_alias_retention_days":        p.SlugAliasRetentionDays,
		"telemetry_retention_days":         p.TelemetryRetentionDays,
		"signed_url_ttl_seconds":           p.SignedURLTTLSeconds,
		"file_list_max_files":              p.FileListMaxFiles,
		"file_list_max_files_limit":        fileListLimit,
		"created_at":                       formatTimeUTC(p.CreatedAt),
		"updated_at":                       formatTimeUTC(p.UpdatedAt),
	}
	if stats != nil {
		body["stats"] = stats
	}
	if storePlaintext != "" {
		body["store_token"] = storePlaintext
	}
	return body
}

func normalizeStorageDriver(driver string) string {
	d := strings.ToLower(strings.TrimSpace(driver))
	if d == "" {
		return "local"
	}
	return d
}

func projectStatsMap(ctx context.Context, svc *service.ProjectService, list []model.Project) (map[uuid.UUID]service.ProjectStats, error) {
	ids := make([]uuid.UUID, 0, len(list))
	engines := make(map[uuid.UUID]string, len(list))
	for i := range list {
		ids = append(ids, list[i].ID)
		engines[list[i].ID] = list[i].CompareEngine
	}
	return svc.StatsFor(ctx, ids, engines, time.Now().UTC())
}

func publicToken(t *model.ProjectToken) gin.H {
	body := gin.H{
		"id":          t.ID,
		"project_id":  t.ProjectID,
		"name":        t.Name,
		"scopes":      t.Scopes,
		"fingerprint": t.Fingerprint,
		"created_at":  formatTimeUTC(t.CreatedAt),
		"updated_at":  formatTimeUTC(t.UpdatedAt),
	}
	if t.ExpiresAt != nil {
		body["expires_at"] = formatTimeUTC(*t.ExpiresAt)
	} else {
		body["expires_at"] = nil
	}
	return body
}

func formatTimeUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func writeProjectErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrProjectNotFound):
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrMemberNotFound):
		response.Error(c, http.StatusNotFound, "MEMBER_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrAdminNotFound):
		response.Error(c, http.StatusNotFound, "ADMIN_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrUsernameTaken):
		response.Error(c, http.StatusConflict, "USERNAME_TAKEN", err.Error(), nil)
	case errors.Is(err, service.ErrForbidden):
		response.Error(c, http.StatusForbidden, "FORBIDDEN", err.Error(), nil)
	case errors.Is(err, service.ErrLastOwner):
		response.Error(c, http.StatusBadRequest, "LAST_OWNER", err.Error(), nil)
	case errors.Is(err, service.ErrMemberExists):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	case errors.Is(err, service.ErrTokenNotFound):
		response.Error(c, http.StatusNotFound, "TOKEN_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrSlugTaken):
		response.Error(c, http.StatusConflict, "SLUG_TAKEN", err.Error(), nil)
	case errors.Is(err, service.ErrCompareEngineImmutable):
		response.Error(c, http.StatusConflict, "COMPARE_ENGINE_IMMUTABLE", err.Error(), nil)
	case errors.Is(err, service.ErrPackageTypeImmutable):
		response.Error(c, http.StatusConflict, "PACKAGE_TYPE_IMMUTABLE", err.Error(), nil)
	case errors.Is(err, service.ErrSystemChannel):
		response.Error(c, http.StatusConflict, "SYSTEM_CHANNEL", err.Error(), nil)
	case errors.Is(err, service.ErrMatrixExists):
		response.Error(c, http.StatusConflict, "MATRIX_EXISTS", err.Error(), nil)
	case errors.Is(err, service.ErrChannelNotFound):
		response.Error(c, http.StatusNotFound, "CHANNEL_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrMatrixNotFound):
		response.Error(c, http.StatusNotFound, "MATRIX_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrHwRevNotFound):
		response.Error(c, http.StatusNotFound, "HW_REV_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrLanguageTaken):
		response.Error(c, http.StatusConflict, "LANGUAGE_TAKEN", err.Error(), nil)
	case errors.Is(err, service.ErrLanguageNotFound):
		response.Error(c, http.StatusNotFound, "LANGUAGE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrHwRevUnknown):
		response.Error(c, http.StatusBadRequest, "HW_REV_UNKNOWN", err.Error(), nil)
	case errors.Is(err, service.ErrInvalidSlug),
		errors.Is(err, service.ErrInvalidProjectSettings),
		errors.Is(err, service.ErrBootstrapTokenRejected),
		errors.Is(err, service.ErrInvalidTokenScopes),
		errors.Is(err, service.ErrInvalidTokenName),
		errors.Is(err, service.ErrInvalidChannel),
		errors.Is(err, service.ErrInvalidMatrix),
		errors.Is(err, service.ErrInvalidHwRev),
		errors.Is(err, service.ErrInvalidLanguage),
		errors.Is(err, service.ErrLanguageIsDefault),
		errors.Is(err, service.ErrLanguageLast),
		errors.Is(err, service.ErrStoreListingTaken),
		errors.Is(err, service.ErrInvalidStoreListing):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	case errors.Is(err, service.ErrStoreListingNotFound):
		response.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
	case errors.Is(err, pathutil.ErrInvalidPath):
		response.Error(c, http.StatusBadRequest, "INVALID_PATH", err.Error(), nil)
	case service.IsInvalidRequest(err):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	default:
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}
