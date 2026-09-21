package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// ProjectStore 项目及其 alias / token / 瘦 Version 的持久化。不含鉴权决策。
type ProjectStore interface {
	Create(ctx context.Context, project *model.Project) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Project, error)
	GetBySlug(ctx context.Context, slug string) (*model.Project, error)
	GetByUnexpiredAlias(ctx context.Context, slug string, now time.Time) (*model.Project, *time.Time, error)
	List(ctx context.Context) ([]model.Project, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Project, error)
	Save(ctx context.Context, project *model.Project) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	SlugTaken(ctx context.Context, slug string, excludeProjectID uuid.UUID, now time.Time) (bool, error)

	CreateAlias(ctx context.Context, alias *model.ProjectSlugAlias) error
	DeleteAliasesBySlug(ctx context.Context, projectID uuid.UUID, slug string) error

	CreateToken(ctx context.Context, token *model.ProjectToken) error
	ListTokens(ctx context.Context, projectID uuid.UUID) ([]model.ProjectToken, error)
	GetTokenByID(ctx context.Context, projectID, tokenID uuid.UUID) (*model.ProjectToken, error)
	GetTokenByHash(ctx context.Context, hash string) (*model.ProjectToken, error)
	DeleteToken(ctx context.Context, projectID, tokenID uuid.UUID) error

	CreateCIToken(ctx context.Context, token *model.CIToken) error
	GetCITokenByHash(ctx context.Context, hash string) (*model.CIToken, error)
	ListCITokens(ctx context.Context, projectID uuid.UUID) ([]model.CIToken, error)
	GetCITokenByID(ctx context.Context, projectID, tokenID uuid.UUID) (*model.CIToken, error)
	DeleteCIToken(ctx context.Context, projectID, tokenID uuid.UUID) error

	HasPublishedVersion(ctx context.Context, projectID uuid.UUID) (bool, error)
	CreateVersion(ctx context.Context, version *model.Version) error
	GetVersionByID(ctx context.Context, id uuid.UUID) (*model.Version, error)
	GetVersionByInteger(ctx context.Context, projectID uuid.UUID, versionInt int64) (*model.Version, error)
	GetVersionBySemverCanonical(ctx context.Context, projectID uuid.UUID, canonical string) (*model.Version, error)
	GetMaxVersionInteger(ctx context.Context, projectID uuid.UUID) (int64, error)
	SaveVersion(ctx context.Context, version *model.Version) error
	DeleteVersion(ctx context.Context, id uuid.UUID) error
	ListVersions(ctx context.Context, projectID uuid.UUID) ([]model.Version, error)
	// ListPublishedReadyVersionsForPlatform 列出 status=published 且该 (os,arch) 线 status=ready 的版本（os/arch 已是 CanonicalOSWrite / CanonicalArch）。
	ListPublishedReadyVersionsForPlatform(ctx context.Context, projectID uuid.UUID, os, arch string) ([]model.Version, error)

	CreateVersionLine(ctx context.Context, line *model.VersionLine) error
	HasPublishedVersionLine(ctx context.Context, projectID uuid.UUID, os, arch string) (bool, error)
	ListVersionLines(ctx context.Context, versionID uuid.UUID) ([]model.VersionLine, error)
	GetVersionLine(ctx context.Context, versionID uuid.UUID, os, arch string) (*model.VersionLine, error)
	SaveVersionLine(ctx context.Context, line *model.VersionLine) error
	CountReadyVersionLines(ctx context.Context, versionID uuid.UUID) (int64, error)

	// 灰度白名单（版本级）。DeviceID 已由 service 按项目策略处理。
	ListVersionAllowlist(ctx context.Context, projectID uuid.UUID) ([]model.GrayAllowlist, error)
	ListAllowlist(ctx context.Context, projectID, versionID uuid.UUID) ([]model.GrayAllowlist, error)
	InsertAllowlist(ctx context.Context, entries []model.GrayAllowlist) error
	DeleteAllowlist(ctx context.Context, projectID, versionID uuid.UUID, deviceIDs []string) (int64, error)
	DeleteAllowlistByVersion(ctx context.Context, versionID uuid.UUID) error
	DeleteAllowlistByDeviceID(ctx context.Context, projectID uuid.UUID, deviceID string) (int64, error)
	CountAllowlist(ctx context.Context, versionID uuid.UUID) (int64, error)
	AdmitGray(ctx context.Context, project *model.Project, version *model.Version, now time.Time) (*GrayAdmitResult, error)

	UpsertClientLogin(ctx context.Context, row *model.Client, custom model.JSONObject, now time.Time) (*model.Client, bool, error)
	UpsertClientCheck(ctx context.Context, row *model.Client, now time.Time) (*model.Client, bool, error)
	GetClientByID(ctx context.Context, projectID, id uuid.UUID) (*model.Client, error)
	GetClientByDeviceHash(ctx context.Context, projectID uuid.UUID, hash string) (*model.Client, error)
	DeleteClient(ctx context.Context, projectID, id uuid.UUID) error
	ListClients(ctx context.Context, projectID uuid.UUID, filter ClientListFilter) ([]model.Client, int64, error)
	CountClients(ctx context.Context, projectID uuid.UUID) (int64, error)
	ListClientsByIDs(ctx context.Context, projectID uuid.UUID, ids []uuid.UUID) ([]model.Client, error)
	ClientBuckets(ctx context.Context, projectID uuid.UUID, now time.Time) (ClientBuckets, error)
	CountActiveClients(ctx context.Context, projectID uuid.UUID, since time.Time) (int64, error)
	ListClientDailyStats(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]model.ClientDailyStats, error)
	InsertGraySnapshot(ctx context.Context, snap *model.GrayRolloutSnapshot) error
	ListGraySnapshots(ctx context.Context, versionID uuid.UUID, from, to time.Time) ([]model.GrayRolloutSnapshot, error)

	SeedSystemChannels(ctx context.Context, projectID uuid.UUID, names model.SystemChannelNames) error
	ListChannels(ctx context.Context, projectID uuid.UUID) ([]model.Channel, error)
	GetChannel(ctx context.Context, projectID uuid.UUID, slug string) (*model.Channel, error)
	CreateChannel(ctx context.Context, channel *model.Channel) error
	SaveChannel(ctx context.Context, channel *model.Channel) error
	DeleteChannel(ctx context.Context, projectID uuid.UUID, slug string) error

	ListMatrix(ctx context.Context, projectID uuid.UUID) ([]model.PlatformMatrix, error)
	GetMatrix(ctx context.Context, projectID uuid.UUID, os, arch string) (*model.PlatformMatrix, error)
	CreateMatrix(ctx context.Context, row *model.PlatformMatrix) error
	SaveMatrix(ctx context.Context, row *model.PlatformMatrix) error

	ListHwRevs(ctx context.Context, projectID uuid.UUID) ([]model.HwRev, error)
	GetHwRev(ctx context.Context, projectID uuid.UUID, slug string) (*model.HwRev, error)
	CreateHwRev(ctx context.Context, hw *model.HwRev) error
	SaveHwRev(ctx context.Context, hw *model.HwRev) error
	DeleteHwRev(ctx context.Context, projectID uuid.UUID, slug string) error

	ListLanguages(ctx context.Context, projectID uuid.UUID) ([]model.ProjectLanguage, error)
	GetLanguage(ctx context.Context, projectID uuid.UUID, code string) (*model.ProjectLanguage, error)
	CountLanguages(ctx context.Context, projectID uuid.UUID) (int64, error)
	CreateLanguage(ctx context.Context, lang *model.ProjectLanguage) error
	SaveLanguage(ctx context.Context, lang *model.ProjectLanguage) error
	DeleteLanguage(ctx context.Context, projectID uuid.UUID, code string) error

	ListStoreListings(ctx context.Context, projectID uuid.UUID) ([]model.StoreListing, error)
	GetStoreListing(ctx context.Context, projectID uuid.UUID, protocol, slug string) (*model.StoreListing, error)
	GetStoreListingByID(ctx context.Context, projectID, id uuid.UUID) (*model.StoreListing, error)
	CreateStoreListing(ctx context.Context, listing *model.StoreListing) error
	SaveStoreListing(ctx context.Context, listing *model.StoreListing) error
	DeleteStoreListing(ctx context.Context, projectID, id uuid.UUID) error
	HasEnabledStoreListing(ctx context.Context, projectID uuid.UUID, protocol string) (bool, error)

	CreateMember(ctx context.Context, member *model.ProjectMember) error
	GetMember(ctx context.Context, projectID, adminID uuid.UUID) (*model.ProjectMember, error)
	ListMembers(ctx context.Context, projectID uuid.UUID) ([]model.ProjectMember, error)
	ListProjectIDsForAdmin(ctx context.Context, adminID uuid.UUID) ([]uuid.UUID, error)
	CountOwners(ctx context.Context, projectID uuid.UUID) (int64, error)
	SaveMember(ctx context.Context, member *model.ProjectMember) error
	DeleteMember(ctx context.Context, projectID, adminID uuid.UUID) error

	CreateArtifact(ctx context.Context, artifact *model.Artifact) error
	SaveArtifact(ctx context.Context, artifact *model.Artifact) error
	GetArtifactByID(ctx context.Context, id uuid.UUID) (*model.Artifact, error)
	// GetArtifactByLineID 返回该线的 kind=full 产物（单文件切片语义上「该线的全量包」）。
	GetArtifactByLineID(ctx context.Context, lineID uuid.UUID) (*model.Artifact, error)
	// GetArtifactByFileName 返回项目内指定稳定文件名的产物（全 kind）。
	GetArtifactByFileName(ctx context.Context, projectID uuid.UUID, fileName string) (*model.Artifact, error)
	// ListArtifactsByLineAndKind 按线与种类列出产物（delta_generate 幂等判定与
	// 全量包定位使用；kind 传 full/delta/file）。
	ListArtifactsByLineAndKind(ctx context.Context, lineID uuid.UUID, kind string) ([]model.Artifact, error)
	// ListArtifactsByVersionID 列出版本下全部产物（created_at 升序）。
	ListArtifactsByVersionID(ctx context.Context, versionID uuid.UUID) ([]model.Artifact, error)
	// ListArtifactsBySHA256 列出项目内同 SHA-256 的 kind=full 产物（created_at 升序），
	// 供 artifacts/reuse 按 sha256 引用源对象（多个命中 → 要求改用 artifact_id）。
	ListArtifactsBySHA256(ctx context.Context, projectID uuid.UUID, sha256 string) ([]model.Artifact, error)
	// ListArtifactsByContentSHA256 列出项目内同 SHA-256 的全部 kind 产物（created_at 升序，
	// 并列时 id 升序由调用方再排），供哈希下载。比较大小写不敏感。
	ListArtifactsByContentSHA256(ctx context.Context, projectID uuid.UUID, sha256 string) ([]model.Artifact, error)
	DeleteArtifact(ctx context.Context, id uuid.UUID) error

	CreateUploadSession(ctx context.Context, session *model.UploadSession) error
	GetUploadSessionByID(ctx context.Context, id uuid.UUID) (*model.UploadSession, error)
	GetUploadSessionByIdempotencyKey(ctx context.Context, projectID uuid.UUID, key string) (*model.UploadSession, error)
	UpdateUploadSession(ctx context.Context, session *model.UploadSession) error
	DeleteUploadSession(ctx context.Context, id uuid.UUID) error
	ListActiveUploadSessionsByLineID(ctx context.Context, lineID uuid.UUID) ([]model.UploadSession, error)
	ListExpiredUploadSessions(ctx context.Context, projectID uuid.UUID, before time.Time) ([]model.UploadSession, error)

	SaveManifestEntries(ctx context.Context, lineID uuid.UUID, entries []model.ManifestEntry) error
	ListManifestEntries(ctx context.Context, lineID uuid.UUID) ([]model.ManifestEntry, error)
	GetManifestEntryByPath(ctx context.Context, lineID uuid.UUID, path string) (*model.ManifestEntry, error)
	CountManifestEntries(ctx context.Context, lineID uuid.UUID) (int64, error)
	DeleteManifestEntriesByLineID(ctx context.Context, lineID uuid.UUID) error

	// 项目列表卡片统计：按 project_id 批量聚合，禁止在 list handler 里逐项目 List*。
	CountVersionsByStatus(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]VersionStatusCounts, error)
	ListReleasedVersions(ctx context.Context, projectIDs []uuid.UUID) ([]model.Version, error)
	CountChannelsByProject(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error)
	CountMatrixByProject(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error)
	CountHwRevsByProject(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error)
	CountProjectTokens(ctx context.Context, projectIDs []uuid.UUID, now time.Time) (map[uuid.UUID]TokenCountRow, error)
	CountCITokens(ctx context.Context, projectIDs []uuid.UUID, now time.Time) (map[uuid.UUID]TokenCountRow, error)
	ArtifactStorageStats(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]ArtifactStorageRow, error)
}

// ProjectRepo 是 PostgreSQL 实现。
type ProjectRepo struct {
	db *gorm.DB
}

// NewProjectRepo 构造仓储。
func NewProjectRepo(db *gorm.DB) *ProjectRepo {
	return &ProjectRepo{db: db}
}

func (r *ProjectRepo) Create(ctx context.Context, project *model.Project) error {
	if err := r.db.WithContext(ctx).Create(project).Error; err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

func (r *ProjectRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Project, error) {
	var project model.Project
	err := r.db.WithContext(ctx).First(&project, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get project by id: %w", err)
	}
	return &project, nil
}

func (r *ProjectRepo) GetBySlug(ctx context.Context, slug string) (*model.Project, error) {
	var project model.Project
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get project by slug: %w", err)
	}
	return &project, nil
}

func (r *ProjectRepo) GetByUnexpiredAlias(ctx context.Context, slug string, now time.Time) (*model.Project, *time.Time, error) {
	var alias model.ProjectSlugAlias
	err := r.db.WithContext(ctx).
		Where("slug = ? AND (expires_at IS NULL OR expires_at > ?)", slug, now).
		First(&alias).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get project alias: %w", err)
	}
	p, err := r.GetByID(ctx, alias.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	var exp *time.Time
	if alias.ExpiresAt != nil {
		t := alias.ExpiresAt.UTC()
		exp = &t
	}
	return p, exp, nil
}

func (r *ProjectRepo) List(ctx context.Context) ([]model.Project, error) {
	var list []model.Project
	if err := r.db.WithContext(ctx).Order("created_at DESC, id DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) Save(ctx context.Context, project *model.Project) error {
	if err := r.db.WithContext(ctx).Save(project).Error; err != nil {
		return fmt.Errorf("save project: %w", err)
	}
	return nil
}

func (r *ProjectRepo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&model.Project{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("soft delete project: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) SlugTaken(ctx context.Context, slug string, excludeProjectID uuid.UUID, now time.Time) (bool, error) {
	q := r.db.WithContext(ctx).Unscoped().Model(&model.Project{}).Where("slug = ?", slug)
	if excludeProjectID != uuid.Nil {
		q = q.Where("id <> ?", excludeProjectID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, fmt.Errorf("count project slug: %w", err)
	}
	if n > 0 {
		return true, nil
	}
	aq := r.db.WithContext(ctx).Model(&model.ProjectSlugAlias{}).
		Where("slug = ? AND (expires_at IS NULL OR expires_at > ?)", slug, now)
	if excludeProjectID != uuid.Nil {
		aq = aq.Where("project_id <> ?", excludeProjectID)
	}
	if err := aq.Count(&n).Error; err != nil {
		return false, fmt.Errorf("count alias slug: %w", err)
	}
	return n > 0, nil
}

func (r *ProjectRepo) CreateAlias(ctx context.Context, alias *model.ProjectSlugAlias) error {
	if err := r.db.WithContext(ctx).Create(alias).Error; err != nil {
		return fmt.Errorf("create slug alias: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteAliasesBySlug(ctx context.Context, projectID uuid.UUID, slug string) error {
	if err := r.db.WithContext(ctx).
		Where("project_id = ? AND slug = ?", projectID, slug).
		Delete(&model.ProjectSlugAlias{}).Error; err != nil {
		return fmt.Errorf("delete slug aliases: %w", err)
	}
	return nil
}

func (r *ProjectRepo) CreateToken(ctx context.Context, token *model.ProjectToken) error {
	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("create project token: %w", err)
	}
	return nil
}

func (r *ProjectRepo) ListTokens(ctx context.Context, projectID uuid.UUID) ([]model.ProjectToken, error) {
	var list []model.ProjectToken
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list project tokens: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetTokenByID(ctx context.Context, projectID, tokenID uuid.UUID) (*model.ProjectToken, error) {
	var token model.ProjectToken
	err := r.db.WithContext(ctx).Where("id = ? AND project_id = ?", tokenID, projectID).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get project token: %w", err)
	}
	return &token, nil
}

func (r *ProjectRepo) GetTokenByHash(ctx context.Context, hash string) (*model.ProjectToken, error) {
	var token model.ProjectToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get project token by hash: %w", err)
	}
	return &token, nil
}

func (r *ProjectRepo) DeleteToken(ctx context.Context, projectID, tokenID uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ? AND project_id = ?", tokenID, projectID).Delete(&model.ProjectToken{})
	if res.Error != nil {
		return fmt.Errorf("delete project token: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) CreateCIToken(ctx context.Context, token *model.CIToken) error {
	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("create ci token: %w", err)
	}
	return nil
}

func (r *ProjectRepo) GetCITokenByHash(ctx context.Context, hash string) (*model.CIToken, error) {
	var token model.CIToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get ci token by hash: %w", err)
	}
	return &token, nil
}

func (r *ProjectRepo) ListCITokens(ctx context.Context, projectID uuid.UUID) ([]model.CIToken, error) {
	var list []model.CIToken
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list ci tokens: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetCITokenByID(ctx context.Context, projectID, tokenID uuid.UUID) (*model.CIToken, error) {
	var tok model.CIToken
	err := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, tokenID).First(&tok).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get ci token by id: %w", err)
	}
	return &tok, nil
}

func (r *ProjectRepo) DeleteCIToken(ctx context.Context, projectID, tokenID uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, tokenID).Delete(&model.CIToken{})
	if res.Error != nil {
		return fmt.Errorf("delete ci token: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) HasPublishedVersion(ctx context.Context, projectID uuid.UUID) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.Version{}).
		Where("project_id = ? AND status = ?", projectID, model.VersionStatusPublished).
		Count(&n).Error; err != nil {
		return false, fmt.Errorf("count published versions: %w", err)
	}
	return n > 0, nil
}

func (r *ProjectRepo) CreateVersion(ctx context.Context, version *model.Version) error {
	if err := r.db.WithContext(ctx).Create(version).Error; err != nil {
		return fmt.Errorf("create version: %w", err)
	}
	return nil
}

func (r *ProjectRepo) GetVersionByID(ctx context.Context, id uuid.UUID) (*model.Version, error) {
	var v model.Version
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *ProjectRepo) GetVersionByInteger(ctx context.Context, projectID uuid.UUID, versionInt int64) (*model.Version, error) {
	var v model.Version
	if err := r.db.WithContext(ctx).Where("project_id = ? AND version_integer = ?", projectID, versionInt).First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *ProjectRepo) GetVersionBySemverCanonical(ctx context.Context, projectID uuid.UUID, canonical string) (*model.Version, error) {
	var v model.Version
	if err := r.db.WithContext(ctx).Where("project_id = ? AND version_semver_canonical = ?", projectID, canonical).First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *ProjectRepo) GetMaxVersionInteger(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var maxVal *int64
	err := r.db.WithContext(ctx).Model(&model.Version{}).
		Where("project_id = ? AND version_integer IS NOT NULL", projectID).
		Select("COALESCE(MAX(version_integer), 0)").
		Scan(&maxVal).Error
	if err != nil {
		return 0, fmt.Errorf("get max version integer: %w", err)
	}
	if maxVal == nil {
		return 0, nil
	}
	return *maxVal, nil
}

func (r *ProjectRepo) SaveVersion(ctx context.Context, version *model.Version) error {
	if err := r.db.WithContext(ctx).Save(version).Error; err != nil {
		return fmt.Errorf("save version: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteVersion(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("version_id = ?", id).Delete(&model.VersionLine{}).Error; err != nil {
			return fmt.Errorf("delete version lines: %w", err)
		}
		// 灰度白名单随版本级联删除（C12；不留孤儿条目）。
		if err := tx.Where("version_id = ?", id).Delete(&model.GrayAllowlist{}).Error; err != nil {
			return fmt.Errorf("delete gray allowlist: %w", err)
		}
		if err := tx.Where("id = ?", id).Delete(&model.Version{}).Error; err != nil {
			return fmt.Errorf("delete version: %w", err)
		}
		return nil
	})
}

func (r *ProjectRepo) ListVersions(ctx context.Context, projectID uuid.UUID) ([]model.Version, error) {
	var list []model.Version
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return list, nil
}

// ListPublishedReadyVersionsForPlatform 一次 JOIN 查出已发布且该平台切片就绪的版本，禁止逐版本 GetVersion。
func (r *ProjectRepo) ListPublishedReadyVersionsForPlatform(ctx context.Context, projectID uuid.UUID, os, arch string) ([]model.Version, error) {
	var list []model.Version
	sub := r.db.WithContext(ctx).Model(&model.VersionLine{}).
		Select("DISTINCT version_id").
		Where("status = ? AND os = ? AND arch = ?", model.VersionLineStatusReady, os, arch)
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND status = ? AND id IN (?)", projectID, model.VersionStatusPublished, sub).
		Order("created_at DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list published ready versions for platform: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) CreateVersionLine(ctx context.Context, line *model.VersionLine) error {
	if err := r.db.WithContext(ctx).Create(line).Error; err != nil {
		return fmt.Errorf("create version line: %w", err)
	}
	return nil
}

func (r *ProjectRepo) ListVersionLines(ctx context.Context, versionID uuid.UUID) ([]model.VersionLine, error) {
	var list []model.VersionLine
	if err := r.db.WithContext(ctx).Where("version_id = ?", versionID).Order("os ASC, arch ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list version lines: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetVersionLine(ctx context.Context, versionID uuid.UUID, os, arch string) (*model.VersionLine, error) {
	var line model.VersionLine
	if err := r.db.WithContext(ctx).Where("version_id = ? AND os = ? AND arch = ?", versionID, os, arch).First(&line).Error; err != nil {
		return nil, err
	}
	return &line, nil
}

func (r *ProjectRepo) SaveVersionLine(ctx context.Context, line *model.VersionLine) error {
	if err := r.db.WithContext(ctx).Save(line).Error; err != nil {
		return fmt.Errorf("save version line: %w", err)
	}
	return nil
}

func (r *ProjectRepo) CountReadyVersionLines(ctx context.Context, versionID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.VersionLine{}).
		Where("version_id = ? AND status = ?", versionID, model.VersionLineStatusReady).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("count ready version lines: %w", err)
	}
	return n, nil
}

// ---------- 灰度白名单（版本级）----------

func (r *ProjectRepo) ListVersionAllowlist(ctx context.Context, projectID uuid.UUID) ([]model.GrayAllowlist, error) {
	var list []model.GrayAllowlist
	if err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("device_id ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list version allowlist: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) ListAllowlist(ctx context.Context, projectID, versionID uuid.UUID) ([]model.GrayAllowlist, error) {
	var list []model.GrayAllowlist
	if err := r.db.WithContext(ctx).
		Where("project_id = ? AND version_id = ?", projectID, versionID).
		Order("device_id ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list allowlist: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) InsertAllowlist(ctx context.Context, entries []model.GrayAllowlist) error {
	if len(entries) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&entries).Error; err != nil {
		return fmt.Errorf("insert allowlist: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteAllowlist(ctx context.Context, projectID, versionID uuid.UUID, deviceIDs []string) (int64, error) {
	if len(deviceIDs) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("project_id = ? AND version_id = ? AND device_id IN ?", projectID, versionID, deviceIDs).
		Delete(&model.GrayAllowlist{})
	if res.Error != nil {
		return 0, fmt.Errorf("delete allowlist: %w", res.Error)
	}
	return res.RowsAffected, nil
}

func (r *ProjectRepo) DeleteAllowlistByVersion(ctx context.Context, versionID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Where("version_id = ?", versionID).Delete(&model.GrayAllowlist{}).Error; err != nil {
		return fmt.Errorf("delete allowlist by version: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteAllowlistByDeviceID(ctx context.Context, projectID uuid.UUID, deviceID string) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("project_id = ? AND device_id = ?", projectID, deviceID).
		Delete(&model.GrayAllowlist{})
	if res.Error != nil {
		return 0, fmt.Errorf("delete allowlist by device id: %w", res.Error)
	}
	return res.RowsAffected, nil
}

func (r *ProjectRepo) CountAllowlist(ctx context.Context, versionID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.GrayAllowlist{}).Where("version_id = ?", versionID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count allowlist: %w", err)
	}
	return n, nil
}

func (r *ProjectRepo) HasPublishedVersionLine(ctx context.Context, projectID uuid.UUID, os, arch string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.VersionLine{}).
		Joins("JOIN versions ON versions.id = version_lines.version_id").
		Where("versions.project_id = ? AND versions.status = ? AND version_lines.os = ? AND version_lines.arch = ?",
			projectID, model.VersionStatusPublished, os, arch).
		Count(&n).Error
	if err != nil {
		return false, fmt.Errorf("count published version lines: %w", err)
	}
	return n > 0, nil
}

func seedSystemChannels(db *gorm.DB, projectID uuid.UUID, names model.SystemChannelNames) error {
	for _, seed := range model.SystemChannelSeeds(projectID, names) {
		ch := seed
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "project_id"}, {Name: "slug"}},
			DoNothing: true,
		}).Create(&ch).Error; err != nil {
			return fmt.Errorf("seed channel %s: %w", ch.Slug, err)
		}
	}
	return nil
}

func (r *ProjectRepo) SeedSystemChannels(ctx context.Context, projectID uuid.UUID, names model.SystemChannelNames) error {
	return seedSystemChannels(r.db.WithContext(ctx), projectID, names)
}

func (r *ProjectRepo) ListChannels(ctx context.Context, projectID uuid.UUID) ([]model.Channel, error) {
	var list []model.Channel
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).
		Order("stability_rank ASC, slug ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetChannel(ctx context.Context, projectID uuid.UUID, slug string) (*model.Channel, error) {
	var ch model.Channel
	err := r.db.WithContext(ctx).Where("project_id = ? AND slug = ?", projectID, slug).First(&ch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	return &ch, nil
}

func (r *ProjectRepo) CreateChannel(ctx context.Context, channel *model.Channel) error {
	if err := r.db.WithContext(ctx).Create(channel).Error; err != nil {
		return fmt.Errorf("create channel: %w", err)
	}
	return nil
}

func (r *ProjectRepo) SaveChannel(ctx context.Context, channel *model.Channel) error {
	if err := r.db.WithContext(ctx).Save(channel).Error; err != nil {
		return fmt.Errorf("save channel: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteChannel(ctx context.Context, projectID uuid.UUID, slug string) error {
	res := r.db.WithContext(ctx).Where("project_id = ? AND slug = ?", projectID, slug).Delete(&model.Channel{})
	if res.Error != nil {
		return fmt.Errorf("delete channel: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) ListMatrix(ctx context.Context, projectID uuid.UUID) ([]model.PlatformMatrix, error) {
	var list []model.PlatformMatrix
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).
		Order("os ASC, arch ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list matrix: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetMatrix(ctx context.Context, projectID uuid.UUID, os, arch string) (*model.PlatformMatrix, error) {
	var row model.PlatformMatrix
	err := r.db.WithContext(ctx).Where("project_id = ? AND os = ? AND arch = ?", projectID, os, arch).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get matrix: %w", err)
	}
	return &row, nil
}

func (r *ProjectRepo) CreateMatrix(ctx context.Context, row *model.PlatformMatrix) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create matrix: %w", err)
	}
	return nil
}

func (r *ProjectRepo) SaveMatrix(ctx context.Context, row *model.PlatformMatrix) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return fmt.Errorf("save matrix: %w", err)
	}
	return nil
}

func (r *ProjectRepo) ListHwRevs(ctx context.Context, projectID uuid.UUID) ([]model.HwRev, error) {
	var list []model.HwRev
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).
		Order("rank ASC, slug ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list hw revs: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetHwRev(ctx context.Context, projectID uuid.UUID, slug string) (*model.HwRev, error) {
	var hw model.HwRev
	err := r.db.WithContext(ctx).Where("project_id = ? AND slug = ?", projectID, slug).First(&hw).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get hw rev: %w", err)
	}
	return &hw, nil
}

func (r *ProjectRepo) CreateHwRev(ctx context.Context, hw *model.HwRev) error {
	if err := r.db.WithContext(ctx).Create(hw).Error; err != nil {
		return fmt.Errorf("create hw rev: %w", err)
	}
	return nil
}

func (r *ProjectRepo) SaveHwRev(ctx context.Context, hw *model.HwRev) error {
	if err := r.db.WithContext(ctx).Save(hw).Error; err != nil {
		return fmt.Errorf("save hw rev: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteHwRev(ctx context.Context, projectID uuid.UUID, slug string) error {
	res := r.db.WithContext(ctx).Where("project_id = ? AND slug = ?", projectID, slug).Delete(&model.HwRev{})
	if res.Error != nil {
		return fmt.Errorf("delete hw rev: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) CreateArtifact(ctx context.Context, artifact *model.Artifact) error {
	if err := r.db.WithContext(ctx).Create(artifact).Error; err != nil {
		return fmt.Errorf("create artifact: %w", err)
	}
	return nil
}

func (r *ProjectRepo) SaveArtifact(ctx context.Context, artifact *model.Artifact) error {
	if err := r.db.WithContext(ctx).Save(artifact).Error; err != nil {
		return fmt.Errorf("save artifact: %w", err)
	}
	return nil
}

func (r *ProjectRepo) GetArtifactByID(ctx context.Context, id uuid.UUID) (*model.Artifact, error) {
	var a model.Artifact
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get artifact by id: %w", err)
	}
	return &a, nil
}

// GetArtifactByLineID 返回该线的 kind=full 产物。切片上可能并存多个 delta 产物，
// 语义上「该线的全量包」恒指 kind=full（上传幂等/发布不可变判定依赖此约定）。
func (r *ProjectRepo) GetArtifactByLineID(ctx context.Context, lineID uuid.UUID) (*model.Artifact, error) {
	var a model.Artifact
	err := r.db.WithContext(ctx).
		Where("version_line_id = ? AND kind = ?", lineID, model.ArtifactKindFull).
		Order("created_at DESC").First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get artifact by line id: %w", err)
	}
	return &a, nil
}

func (r *ProjectRepo) GetArtifactByFileName(ctx context.Context, projectID uuid.UUID, fileName string) (*model.Artifact, error) {
	var a model.Artifact
	err := r.db.WithContext(ctx).Where("project_id = ? AND file_name = ?", projectID, fileName).Order("created_at DESC").First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get artifact by file name: %w", err)
	}
	return &a, nil
}

// ListArtifactsByLineAndKind 按线与种类列出产物（created_at 升序，确定性）。
func (r *ProjectRepo) ListArtifactsByLineAndKind(ctx context.Context, lineID uuid.UUID, kind string) ([]model.Artifact, error) {
	var list []model.Artifact
	if err := r.db.WithContext(ctx).
		Where("version_line_id = ? AND kind = ?", lineID, kind).
		Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list artifacts by line and kind: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) ListArtifactsByVersionID(ctx context.Context, versionID uuid.UUID) ([]model.Artifact, error) {
	var list []model.Artifact
	if err := r.db.WithContext(ctx).Where("version_id = ?", versionID).Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list artifacts by version id: %w", err)
	}
	return list, nil
}

// ListArtifactsBySHA256 列出项目内同 SHA-256 的 kind=full 产物（created_at 升序，
// 确定性）。仅 full：产物复用面向全量包/归档对象，delta/patch 差量产物不可作为复用源。
func (r *ProjectRepo) ListArtifactsBySHA256(ctx context.Context, projectID uuid.UUID, sha256 string) ([]model.Artifact, error) {
	var list []model.Artifact
	if err := r.db.WithContext(ctx).
		Where("project_id = ? AND sha256 = ? AND kind = ?", projectID, strings.ToLower(sha256), model.ArtifactKindFull).
		Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list artifacts by sha256: %w", err)
	}
	return list, nil
}

// ListArtifactsByContentSHA256 列出项目内同 SHA-256 的全部 kind 产物（created_at 升序）。
func (r *ProjectRepo) ListArtifactsByContentSHA256(ctx context.Context, projectID uuid.UUID, sha256 string) ([]model.Artifact, error) {
	var list []model.Artifact
	if err := r.db.WithContext(ctx).
		Where("project_id = ? AND LOWER(sha256) = ?", projectID, strings.ToLower(sha256)).
		Order("created_at ASC, id ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list artifacts by content sha256: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) DeleteArtifact(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Artifact{})
	if res.Error != nil {
		return fmt.Errorf("delete artifact: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) CreateUploadSession(ctx context.Context, session *model.UploadSession) error {
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		return fmt.Errorf("create upload session: %w", err)
	}
	return nil
}

func (r *ProjectRepo) GetUploadSessionByID(ctx context.Context, id uuid.UUID) (*model.UploadSession, error) {
	var s model.UploadSession
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get upload session by id: %w", err)
	}
	return &s, nil
}

func (r *ProjectRepo) GetUploadSessionByIdempotencyKey(ctx context.Context, projectID uuid.UUID, key string) (*model.UploadSession, error) {
	var s model.UploadSession
	err := r.db.WithContext(ctx).Where("project_id = ? AND idempotency_key = ?", projectID, key).Order("created_at DESC").First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get upload session by idempotency key: %w", err)
	}
	return &s, nil
}

func (r *ProjectRepo) UpdateUploadSession(ctx context.Context, session *model.UploadSession) error {
	if err := r.db.WithContext(ctx).Save(session).Error; err != nil {
		return fmt.Errorf("update upload session: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteUploadSession(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.UploadSession{})
	if res.Error != nil {
		return fmt.Errorf("delete upload session: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) ListActiveUploadSessionsByLineID(ctx context.Context, lineID uuid.UUID) ([]model.UploadSession, error) {
	var list []model.UploadSession
	if err := r.db.WithContext(ctx).Where("version_line_id = ? AND status = ?", lineID, model.UploadSessionStatusUploading).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list active upload sessions: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) ListExpiredUploadSessions(ctx context.Context, projectID uuid.UUID, before time.Time) ([]model.UploadSession, error) {
	var list []model.UploadSession
	query := r.db.WithContext(ctx).Where("(expires_at < ? OR status IN (?, ?))", before, model.UploadSessionStatusAborted, model.UploadSessionStatusExpired)
	if projectID != uuid.Nil {
		query = query.Where("project_id = ?", projectID)
	}
	if err := query.Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list expired upload sessions: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) SaveManifestEntries(ctx context.Context, lineID uuid.UUID, entries []model.ManifestEntry) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("version_line_id = ?", lineID).Delete(&model.ManifestEntry{}).Error; err != nil {
			return fmt.Errorf("delete old manifest entries: %w", err)
		}
		if len(entries) > 0 {
			if err := tx.Create(&entries).Error; err != nil {
				return fmt.Errorf("insert manifest entries: %w", err)
			}
		}
		return nil
	})
}

func (r *ProjectRepo) ListManifestEntries(ctx context.Context, lineID uuid.UUID) ([]model.ManifestEntry, error) {
	var list []model.ManifestEntry
	if err := r.db.WithContext(ctx).Where("version_line_id = ?", lineID).Order("path ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list manifest entries: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetManifestEntryByPath(ctx context.Context, lineID uuid.UUID, path string) (*model.ManifestEntry, error) {
	var entry model.ManifestEntry
	err := r.db.WithContext(ctx).Where("version_line_id = ? AND path = ?", lineID, path).First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get manifest entry by path: %w", err)
	}
	return &entry, nil
}

func (r *ProjectRepo) CountManifestEntries(ctx context.Context, lineID uuid.UUID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.ManifestEntry{}).Where("version_line_id = ?", lineID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count manifest entries: %w", err)
	}
	return count, nil
}

func (r *ProjectRepo) DeleteManifestEntriesByLineID(ctx context.Context, lineID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Where("version_line_id = ?", lineID).Delete(&model.ManifestEntry{}).Error; err != nil {
		return fmt.Errorf("delete manifest entries by line: %w", err)
	}
	return nil
}
