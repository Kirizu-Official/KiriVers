package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

var (
	// ErrAnnouncementNotFound 公告不存在或不属于该项目。
	ErrAnnouncementNotFound = errors.New("announcement not found")
	// ErrInvalidQueryParam 客户端查询参数无法解析（如非法 version）。
	ErrInvalidQueryParam = errors.New("invalid query param")
)

// announcementVersionLookup 解析 Version 行（UUID / 整数 / 规范化 SemVer），由 ProjectStore 实现。
type announcementVersionLookup interface {
	GetVersionByID(ctx context.Context, id uuid.UUID) (*model.Version, error)
	GetVersionByInteger(ctx context.Context, projectID uuid.UUID, versionInt int64) (*model.Version, error)
	GetVersionBySemverCanonical(ctx context.Context, projectID uuid.UUID, canonical string) (*model.Version, error)
}

// AnnouncementService 公告领域逻辑：作用域校验、可见性、匹配、语言列、排序。无 gin.Context，不调用 SelectTarget。
type AnnouncementService struct {
	store    repository.AnnouncementStore
	versions announcementVersionLookup
}

// NewAnnouncementService 构造服务。store 或 versions 为 nil 时写路径会失败。
func NewAnnouncementService(store repository.AnnouncementStore, versions announcementVersionLookup) *AnnouncementService {
	return &AnnouncementService{store: store, versions: versions}
}

// AnnouncementCreateInput 是管理端创建输入。Status 忽略，创建恒为 draft。
type AnnouncementCreateInput struct {
	VersionID *uuid.UUID
	OS        string
	Arch      string
	Language  string
	Title     string
	Subtitle  string
	Content   string
	StartsAt  *time.Time
	EndsAt    *time.Time
}

// AnnouncementPatchInput 是管理端部分更新。指针/Set 标志区分省略与清空。
type AnnouncementPatchInput struct {
	Status       *string
	VersionIDSet bool
	VersionID    *uuid.UUID
	OSSet        bool
	OS           string
	ArchSet      bool
	Arch         string
	LanguageSet  bool
	Language     string
	TitleSet     bool
	Title        string
	SubtitleSet  bool
	Subtitle     string
	ContentSet   bool
	Content      string
	StartsAtSet  bool
	StartsAt     *time.Time
	EndsAtSet    bool
	EndsAt       *time.Time
}

// ClientAnnouncement 是客户端平面单条文案（不含 status / sort_order）。
type ClientAnnouncement struct {
	ID        uuid.UUID  `json:"id"`
	Title     string     `json:"title"`
	Subtitle  string     `json:"subtitle"`
	Markdown  string     `json:"markdown"`
	Locale    string     `json:"locale"`
	StartsAt  *time.Time `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at"`
	UpdatedAt time.Time  `json:"-"`
}

// ClientListInput 是客户端 GET 的匹配上下文。Now 为零则用当前 UTC。
type ClientListInput struct {
	Version        string
	OS             string
	Arch           string
	Locale         string
	AcceptLanguage string
	Now            time.Time
}

// ClientListResult 是客户端列表与 CDN 缓存头。
type ClientListResult struct {
	Items        []ClientAnnouncement
	ETag         string
	CacheControl string
	Vary         []string
}

// ListAdmin 返回项目内全部公告索引（含草稿与窗外项，不含正文），已按 sort_order 排序。
func (s *AnnouncementService) ListAdmin(ctx context.Context, projectID uuid.UUID) ([]model.Announcement, error) {
	list, err := s.store.ListIndexByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	s.flipDueScheduled(ctx, list, time.Now().UTC())
	return list, nil
}

// GetAdmin 读取一条管理端公告（含正文）。
func (s *AnnouncementService) GetAdmin(ctx context.Context, projectID, id uuid.UUID) (*model.Announcement, error) {
	row, err := s.store.GetByID(ctx, projectID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAnnouncementNotFound
		}
		return nil, err
	}
	list := []model.Announcement{*row}
	s.flipDueScheduled(ctx, list, time.Now().UTC())
	return &list[0], nil
}

// Create 追加一条草稿公告。
func (s *AnnouncementService) Create(ctx context.Context, projectID uuid.UUID, in AnnouncementCreateInput) (*model.Announcement, error) {
	n, err := s.store.CountByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if n >= model.AnnouncementMaxPerProject {
		return nil, ErrInvalidRequest("too many announcements")
	}
	language, title, subtitle, content, err := validateAnnouncementFields(in.Language, in.Title, in.Subtitle, in.Content)
	if err != nil {
		return nil, err
	}
	scope, err := s.resolveScope(ctx, projectID, in.VersionID, strings.TrimSpace(in.OS), strings.TrimSpace(in.Arch))
	if err != nil {
		return nil, err
	}
	if err := validateAnnouncementWindow(in.StartsAt, in.EndsAt); err != nil {
		return nil, err
	}
	max, err := s.store.MaxSortOrder(ctx, projectID)
	if err != nil {
		return nil, err
	}
	row := &model.Announcement{
		ProjectID: projectID,
		Status:    model.AnnouncementStatusDraft,
		SortOrder: max + 1,
		VersionID: scope.versionID,
		OS:        scope.os,
		Arch:      scope.arch,
		Language:  language,
		Title:     title,
		Subtitle:  subtitle,
		Content:   content,
		StartsAt:  cloneTime(in.StartsAt),
		EndsAt:    cloneTime(in.EndsAt),
	}
	if err := s.store.Create(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

// Patch 更新一条公告；允许切换作用域与 draft↔published。
func (s *AnnouncementService) Patch(ctx context.Context, projectID, id uuid.UUID, in AnnouncementPatchInput) (*model.Announcement, error) {
	row, err := s.GetAdmin(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	versionID := row.VersionID
	osRaw := row.OS
	archRaw := row.Arch
	if in.VersionIDSet {
		versionID = in.VersionID
	}
	if in.OSSet {
		osRaw = strings.TrimSpace(in.OS)
	}
	if in.ArchSet {
		archRaw = strings.TrimSpace(in.Arch)
	}
	if in.VersionIDSet || in.OSSet || in.ArchSet {
		scope, err := s.resolveScope(ctx, projectID, versionID, osRaw, archRaw)
		if err != nil {
			return nil, err
		}
		row.VersionID = scope.versionID
		row.OS = scope.os
		row.Arch = scope.arch
	}
	if in.Status != nil {
		st := strings.TrimSpace(*in.Status)
		switch st {
		case model.AnnouncementStatusDraft, model.AnnouncementStatusScheduled, model.AnnouncementStatusPublished:
			row.Status = st
		default:
			return nil, ErrInvalidRequest("status must be draft, scheduled or published")
		}
	}
	language := row.Language
	title := row.Title
	subtitle := row.Subtitle
	content := row.Content
	if in.LanguageSet {
		language = in.Language
	}
	if in.TitleSet {
		title = in.Title
	}
	if in.SubtitleSet {
		subtitle = in.Subtitle
	}
	if in.ContentSet {
		content = in.Content
	}
	if in.LanguageSet || in.TitleSet || in.SubtitleSet || in.ContentSet {
		language, title, subtitle, content, err = validateAnnouncementFields(language, title, subtitle, content)
		if err != nil {
			return nil, err
		}
		row.Language = language
		row.Title = title
		row.Subtitle = subtitle
		row.Content = content
	}
	if in.StartsAtSet {
		row.StartsAt = cloneTime(in.StartsAt)
	}
	if in.EndsAtSet {
		row.EndsAt = cloneTime(in.EndsAt)
	}
	if err := validateAnnouncementWindow(row.StartsAt, row.EndsAt); err != nil {
		return nil, err
	}
	if err := applyAnnouncementSchedule(row, time.Now().UTC()); err != nil {
		return nil, err
	}
	if err := s.store.Update(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

// Delete 硬删除一条公告。
func (s *AnnouncementService) Delete(ctx context.Context, projectID, id uuid.UUID) error {
	if err := s.store.Delete(ctx, projectID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAnnouncementNotFound
		}
		return err
	}
	return nil
}

// Reorder 要求 ids 为项目内全部公告 ID 的排列，并重写 sort_order 为 0..n-1。
func (s *AnnouncementService) Reorder(ctx context.Context, projectID uuid.UUID, ids []uuid.UUID) error {
	list, err := s.store.ListIndexByProject(ctx, projectID)
	if err != nil {
		return err
	}
	if len(ids) != len(list) {
		return ErrInvalidRequest("reorder ids must be a permutation of all announcements")
	}
	have := make(map[uuid.UUID]struct{}, len(list))
	for i := range list {
		have[list[i].ID] = struct{}{}
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := have[id]; !ok {
			return ErrInvalidRequest("unknown announcement id")
		}
		if _, dup := seen[id]; dup {
			return ErrInvalidRequest("duplicate announcement id")
		}
		seen[id] = struct{}{}
	}
	if err := s.store.Reorder(ctx, projectID, ids); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAnnouncementNotFound
		}
		return err
	}
	return nil
}

// ListClient 返回当前可见且匹配的公告（已按 sort_order），并计算 ETag / s-maxage。
func (s *AnnouncementService) ListClient(ctx context.Context, project *model.Project, in ClientListInput) (*ClientListResult, error) {
	if project == nil {
		return nil, ErrProjectNotFound
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	list, err := s.store.ListByProject(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	s.flipDueScheduled(ctx, list, now)

	queryOS := platform.CanonicalOS(nil, in.OS)
	queryArch := platform.CanonicalArch(in.Arch)
	var matchVersionID *uuid.UUID
	if strings.TrimSpace(in.Version) != "" {
		id, err := s.lookupQueryVersionID(ctx, project.ID, in.Version)
		if err != nil {
			return nil, err
		}
		matchVersionID = id
	}

	candidates := make([]ClientAnnouncement, 0)
	for i := range list {
		row := &list[i]
		if !announcementVisible(row, now) {
			continue
		}
		if !announcementMatches(row, matchVersionID, queryOS, queryArch) {
			continue
		}
		if strings.TrimSpace(row.Title) == "" {
			continue
		}
		candidates = append(candidates, ClientAnnouncement{
			ID:        row.ID,
			Title:     row.Title,
			Subtitle:  row.Subtitle,
			Markdown:  row.Content,
			Locale:    row.Language,
			StartsAt:  cloneTime(row.StartsAt),
			EndsAt:    cloneTime(row.EndsAt),
			UpdatedAt: row.UpdatedAt,
		})
	}

	explicitLocale := strings.TrimSpace(in.Locale) != ""
	items := filterAnnouncementLanguage(candidates, explicitLocale, in.Locale, in.AcceptLanguage, project.DefaultLocale)

	vary := []string{"Accept-Encoding"}
	if project.RequireClientToken {
		vary = append(vary, "Authorization")
	}
	if !explicitLocale {
		vary = append(vary, "Accept-Language")
	}

	return &ClientListResult{
		Items:        items,
		ETag:         announcementListETag(project.ID, items),
		CacheControl: announcementCacheControl(project.CacheSMaxageSeconds, list, now),
		Vary:         vary,
	}, nil
}

type resolvedScope struct {
	versionID *uuid.UUID
	os        string
	arch      string
}

func (s *AnnouncementService) resolveScope(ctx context.Context, projectID uuid.UUID, versionID *uuid.UUID, os, arch string) (resolvedScope, error) {
	hasV := versionID != nil
	hasOS := os != ""
	hasArch := arch != ""
	legal := (!hasV && !hasOS && !hasArch) ||
		(hasV && !hasOS && !hasArch) ||
		(!hasV && hasOS && !hasArch) ||
		(!hasV && !hasOS && hasArch) ||
		(hasV && hasOS && !hasArch) ||
		(hasV && !hasOS && hasArch) ||
		(hasV && hasOS && hasArch)
	if !legal {
		return resolvedScope{}, ErrInvalidRequest("illegal announcement scope")
	}

	out := resolvedScope{}
	if hasOS {
		canon := platform.CanonicalOSWrite(os)
		if !platform.ValidOSArchSlug(canon) {
			return resolvedScope{}, ErrInvalidRequest("invalid os")
		}
		out.os = canon
	}
	if hasArch {
		canon := platform.CanonicalArch(arch)
		if !platform.ValidOSArchSlug(canon) {
			return resolvedScope{}, ErrInvalidRequest("invalid arch")
		}
		out.arch = canon
	}
	if hasV {
		id, err := s.resolveVersionID(ctx, projectID, *versionID)
		if err != nil {
			return resolvedScope{}, err
		}
		out.versionID = id
	}
	return out, nil
}

func (s *AnnouncementService) resolveVersionID(ctx context.Context, projectID, id uuid.UUID) (*uuid.UUID, error) {
	if s.versions == nil {
		return nil, ErrVersionNotFound
	}
	v, err := s.versions.GetVersionByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionNotFound
		}
		return nil, err
	}
	if v == nil || v.ProjectID != projectID {
		return nil, ErrVersionNotFound
	}
	out := v.ID
	return &out, nil
}

func (s *AnnouncementService) lookupQueryVersionID(ctx context.Context, projectID uuid.UUID, raw string) (*uuid.UUID, error) {
	parsed, err := ParseVersionRef(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid version", ErrInvalidQueryParam)
	}
	if s.versions == nil {
		return nil, nil
	}
	var v *model.Version
	if parsed.IsInteger {
		v, err = s.versions.GetVersionByInteger(ctx, projectID, parsed.Integer)
	} else {
		v, err = s.versions.GetVersionBySemverCanonical(ctx, projectID, parsed.Semver)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	id := v.ID
	return &id, nil
}

func validateAnnouncementFields(language, title, subtitle, content string) (string, string, string, string, error) {
	lang := strings.TrimSpace(language)
	if lang == "" || !model.ValidProjectLanguageCode(lang) {
		return "", "", "", "", ErrInvalidRequest("invalid language")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", "", "", ErrInvalidRequest("title is required")
	}
	if utf8.RuneCountInString(title) > model.AnnouncementMaxTitleRunes {
		return "", "", "", "", ErrInvalidRequest("title is too long")
	}
	if utf8.RuneCountInString(subtitle) > model.AnnouncementMaxSubtitleRunes {
		return "", "", "", "", ErrInvalidRequest("subtitle is too long")
	}
	if len(content) > model.AnnouncementMaxContentBytes {
		return "", "", "", "", ErrInvalidRequest("content is too long")
	}
	return lang, title, subtitle, content, nil
}

func validateAnnouncementWindow(startsAt, endsAt *time.Time) error {
	if startsAt != nil && endsAt != nil && endsAt.Before(*startsAt) {
		return ErrInvalidRequest("ends_at is before starts_at")
	}
	return nil
}

func applyAnnouncementSchedule(row *model.Announcement, now time.Time) error {
	if row == nil {
		return nil
	}
	if row.Status != model.AnnouncementStatusScheduled {
		return nil
	}
	if row.StartsAt == nil {
		return ErrInvalidRequest("scheduled announcements require starts_at")
	}
	if !now.Before(row.StartsAt.UTC()) {
		row.Status = model.AnnouncementStatusPublished
	}
	return nil
}

func shouldFlipScheduled(row *model.Announcement, now time.Time) bool {
	return row != nil &&
		row.Status == model.AnnouncementStatusScheduled &&
		row.StartsAt != nil &&
		!now.Before(row.StartsAt.UTC())
}

func (s *AnnouncementService) flipDueScheduled(ctx context.Context, rows []model.Announcement, now time.Time) {
	if s == nil || s.store == nil {
		for i := range rows {
			if shouldFlipScheduled(&rows[i], now) {
				rows[i].Status = model.AnnouncementStatusPublished
			}
		}
		return
	}
	for i := range rows {
		if !shouldFlipScheduled(&rows[i], now) {
			continue
		}
		rows[i].Status = model.AnnouncementStatusPublished
		_ = s.store.UpdateStatus(ctx, rows[i].ProjectID, rows[i].ID, model.AnnouncementStatusPublished)
	}
}

func announcementVisible(row *model.Announcement, now time.Time) bool {
	if row == nil {
		return false
	}
	status := row.Status
	if shouldFlipScheduled(row, now) {
		status = model.AnnouncementStatusPublished
	}
	if status != model.AnnouncementStatusPublished {
		return false
	}
	if row.StartsAt != nil && now.Before(row.StartsAt.UTC()) {
		return false
	}
	if row.EndsAt != nil && !now.Before(row.EndsAt.UTC()) {
		return false
	}
	return true
}

func announcementMatches(row *model.Announcement, versionID *uuid.UUID, os, arch string) bool {
	hasV := row.VersionID != nil
	hasOS := row.OS != ""
	hasArch := row.Arch != ""
	switch {
	case !hasV && !hasOS && !hasArch:
		return true
	case hasV && !hasOS && !hasArch:
		return uuidPtrEqual(versionID, row.VersionID)
	case !hasV && hasOS && !hasArch:
		return os != "" && os == row.OS
	case !hasV && !hasOS && hasArch:
		return arch != "" && arch == row.Arch
	case hasV && hasOS && !hasArch:
		return os != "" && uuidPtrEqual(versionID, row.VersionID) && os == row.OS
	case hasV && !hasOS && hasArch:
		return arch != "" && uuidPtrEqual(versionID, row.VersionID) && arch == row.Arch
	case hasV && hasOS && hasArch:
		return os != "" && arch != "" &&
			uuidPtrEqual(versionID, row.VersionID) && os == row.OS && arch == row.Arch
	default:
		return false
	}
}

func uuidPtrEqual(a, b *uuid.UUID) bool {
	return a != nil && b != nil && *a == *b
}

// filterAnnouncementLanguage 按 D5 从候选行中选出一种语言：
// 显式 locale 严格 EqualFold，不退回；省略 locale 时走 BuildLocaleChain，链全 miss 则 leftover 到字典序最小 language。
func filterAnnouncementLanguage(candidates []ClientAnnouncement, explicit bool, locale, acceptLanguage, defaultLocale string) []ClientAnnouncement {
	if len(candidates) == 0 {
		return candidates
	}
	if explicit {
		tag := announcementLocaleTag(locale)
		out := make([]ClientAnnouncement, 0)
		for i := range candidates {
			if strings.EqualFold(candidates[i].Locale, tag) {
				out = append(out, candidates[i])
			}
		}
		return out
	}
	chain := update.BuildLocaleChain("", "", acceptLanguage, defaultLocale)
	winning := ""
	for _, want := range chain {
		for i := range candidates {
			if strings.EqualFold(candidates[i].Locale, want) {
				winning = candidates[i].Locale
				break
			}
		}
		if winning != "" {
			break
		}
	}
	if winning == "" {
		for i := range candidates {
			lang := candidates[i].Locale
			if winning == "" || lang < winning {
				winning = lang
			}
		}
	}
	out := make([]ClientAnnouncement, 0)
	for i := range candidates {
		if strings.EqualFold(candidates[i].Locale, winning) {
			out = append(out, candidates[i])
		}
	}
	return out
}

func announcementLocaleTag(locale string) string {
	chain := update.BuildLocaleChain("", locale, "", "")
	if len(chain) == 0 {
		return ""
	}
	return chain[0]
}

func announcementCacheControl(projectSMaxage int, rows []model.Announcement, now time.Time) string {
	sMaxage := projectSMaxage
	if sMaxage <= 0 {
		sMaxage = model.DefaultCacheSMaxageSeconds
	}
	var next time.Time
	for i := range rows {
		row := &rows[i]
		if row.Status != model.AnnouncementStatusPublished && row.Status != model.AnnouncementStatusScheduled {
			continue
		}
		for _, ts := range []*time.Time{row.StartsAt, row.EndsAt} {
			if ts == nil {
				continue
			}
			t := ts.UTC()
			if t.After(now) && (next.IsZero() || t.Before(next)) {
				next = t
			}
		}
	}
	if !next.IsZero() {
		sec := int(next.Sub(now).Seconds())
		if sec < 1 {
			sec = 1
		}
		if sec < sMaxage {
			sMaxage = sec
		}
	}
	return fmt.Sprintf("public, s-maxage=%d, stale-while-revalidate=30", sMaxage)
}

func announcementListETag(projectID uuid.UUID, items []ClientAnnouncement) string {
	type etagItem struct {
		ID        string `json:"id"`
		UpdatedAt string `json:"updated_at"`
		Locale    string `json:"locale"`
		Title     string `json:"title"`
		Subtitle  string `json:"subtitle"`
		Markdown  string `json:"markdown"`
	}
	payload := struct {
		ProjectID string     `json:"project_id"`
		Items     []etagItem `json:"items"`
	}{
		ProjectID: projectID.String(),
		Items:     make([]etagItem, 0, len(items)),
	}
	for i := range items {
		payload.Items = append(payload.Items, etagItem{
			ID:        items[i].ID.String(),
			UpdatedAt: items[i].UpdatedAt.UTC().Format(time.RFC3339Nano),
			Locale:    items[i].Locale,
			Title:     items[i].Title,
			Subtitle:  items[i].Subtitle,
			Markdown:  items[i].Markdown,
		})
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := t.UTC()
	return &cp
}
