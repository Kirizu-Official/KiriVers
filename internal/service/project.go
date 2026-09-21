package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

var (
	ErrProjectNotFound        = errors.New("project not found")
	ErrInvalidSlug            = errors.New("invalid slug")
	ErrSlugTaken              = errors.New("slug already taken")
	ErrCompareEngineImmutable = errors.New("compare engine is immutable after a published version")
	ErrInvalidProjectSettings = errors.New("invalid project settings")
	ErrBootstrapTokenRejected = errors.New("bootstrap_admin_token is not supported")
	ErrTokenNotFound          = errors.New("project token not found")
	ErrInvalidTokenScopes     = errors.New("invalid token scopes")
	ErrInvalidTokenName       = errors.New("token name is required")
	ErrSystemChannel          = errors.New("system channel cannot be deleted")
	ErrPackageTypeImmutable   = errors.New("package type is immutable after a published version line")
	ErrHwRevUnknown           = errors.New("hw_rev is not registered")
	ErrChannelNotFound        = errors.New("channel not found")
	ErrMatrixNotFound         = errors.New("platform matrix row not found")
	ErrHwRevNotFound          = errors.New("hw_rev not found")
	ErrInvalidChannel         = errors.New("invalid channel")
	ErrInvalidMatrix          = errors.New("invalid platform matrix")
	ErrInvalidHwRev           = errors.New("invalid hw_rev")
	ErrMatrixExists           = errors.New("platform matrix row already exists")
	ErrLanguageTaken          = errors.New("language already taken")
	ErrLanguageNotFound       = errors.New("language not found")
	ErrInvalidLanguage        = errors.New("invalid language")
	ErrLanguageIsDefault      = errors.New("cannot delete the default language")
	ErrLanguageLast           = errors.New("cannot delete the last language")
	ErrChecksumMismatch       = errors.New("checksum mismatch")
	ErrArtifactImmutable      = errors.New("artifact is immutable after version published")
	ErrUploadIncomplete       = errors.New("upload is incomplete")
	ErrHwRevIncompatible      = errors.New("hw_rev is incompatible with artifact")
	ErrOffsetMismatch         = errors.New("upload offset mismatch")
	ErrUploadSessionNotFound  = errors.New("upload session not found")
	ErrArtifactNotFound       = errors.New("artifact not found")
	ErrStorageUnavailable     = errors.New("storage backend unavailable")
	// ErrReuseSourceRevoked 吊销版本上的产物禁止被复用（§5.1，C14-1）。
	ErrReuseSourceRevoked = errors.New("revoked version artifacts cannot be reused")
	// ErrReuseAmbiguousSHA256 按 sha256 引用源产物时项目内存在多个命中，要求改用 artifact_id。
	ErrReuseAmbiguousSHA256 = errors.New("ambiguous sha256: multiple artifacts match, specify artifact_id")
	// ErrReuseSourceRequired 复用请求必须提供 source.artifact_id 或 source.sha256 之一。
	ErrReuseSourceRequired = errors.New("source artifact_id or sha256 is required")
	// ErrWebhookDeliveryNotFound webhook 投递记录不存在。
	ErrWebhookDeliveryNotFound = errors.New("webhook delivery not found")
	// ErrStoreListingNotFound 商店 listing 不存在。
	ErrStoreListingNotFound = errors.New("store listing not found")
	// ErrStoreListingTaken (protocol, slug) 在项目内已存在 → HTTP 400。
	ErrStoreListingTaken = errors.New("store listing already exists")
	// ErrInvalidStoreListing listing 字段非法（slug / protocol / package_source / pin）。
	ErrInvalidStoreListing = errors.New("invalid store listing")
)

var slugRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,64}$`)

// ProjectService 处理项目 CRUD、slug alias、compare_engine 锁定与项目 Token。
// 本类型不依赖 gin.Context，也不决定「谁可以创建项目」（由中间件完成）。
type ProjectService struct {
	store    repository.ProjectStore
	storage  storage.Backend
	jobs     repository.JobStore
	webhooks repository.WebhookStore
	cache    cache.Store
	cacheLog zerolog.Logger
	// telemetry 供列表/详情 stats 的 24h 事件计数；nil 时遥测字段为 0。
	telemetry repository.TelemetryStore
	admins    *AdminService
	geoip     GeoipLookupFunc
	// dynamicPackMaxBytes 是客户端 fileset 入队前未压缩硬顶（D6）。
	dynamicPackMaxBytes int64
	// fileListMaxFiles 是原生 file_list 待下载条数的平台天花板。
	fileListMaxFiles int
	// changelogDefaultEntries / changelogMaxEntries 是实例 changelog 条数窗口。
	changelogDefaultEntries int
	changelogMaxEntries     int
	packEnqueueMu    sync.Mutex
	nodeID           uuid.UUID
	localRoot        string
	storageDriver    string
	clusterActive    bool
	clusterDownload  string
	nodes            repository.NodeStore
	installPolicy    repository.InstallPolicyRuleStore
}

// NewProjectService 构造项目领域服务。
func NewProjectService(store repository.ProjectStore, storageBackend ...storage.Backend) *ProjectService {
	s := &ProjectService{store: store}
	if len(storageBackend) > 0 {
		s.storage = storageBackend[0]
		if fs, ok := s.storage.(*storage.LocalFS); ok {
			s.localRoot = fs.Root()
		}
	}
	return s
}

// SetStorage 设置存储后端。
func (s *ProjectService) SetStorage(b storage.Backend) {
	s.storage = b
	if s.localRoot == "" {
		if fs, ok := b.(*storage.LocalFS); ok {
			s.localRoot = fs.Root()
		}
	}
}

// Storage 返回当前存储后端。
func (s *ProjectService) Storage() storage.Backend {
	return s.storage
}

// SetJobStore 设置异步任务仓储。
func (s *ProjectService) SetJobStore(js repository.JobStore) {
	s.jobs = js
}

// JobStore 返回异步任务仓储。
func (s *ProjectService) JobStore() repository.JobStore {
	return s.jobs
}

// SetCache 注入热路径身份缓存。nil 时 Resolve 与失效均为现有直连行为。
func (s *ProjectService) SetCache(store cache.Store) {
	s.cache = store
}

// SetCacheLogger 注入 mod=cache 子 logger；失效失败只记日志，不失败写路径。
func (s *ProjectService) SetCacheLogger(log zerolog.Logger) {
	s.cacheLog = log
}

// SetTelemetryStore 注入遥测仓储。nil 时 StatsFor 的 telemetry 计数保持 0。
func (s *ProjectService) SetTelemetryStore(store repository.TelemetryStore) {
	s.telemetry = store
}

// SetGeoipLookup 注入来源地查询；nil 时名册不写地理字段。
func (s *ProjectService) SetGeoipLookup(fn GeoipLookupFunc) {
	s.geoip = fn
}

// SetInstallPolicyStore 注入安装策略模板仓储。nil 时生效策略为空（全部 OVERWRITE）。
func (s *ProjectService) SetInstallPolicyStore(store repository.InstallPolicyRuleStore) {
	s.installPolicy = store
}

// SetNodeID 盖管理面 Job 的 owner_node_id。
func (s *ProjectService) SetNodeID(id uuid.UUID) {
	s.nodeID = id
}

// NodeID 返回本进程节点 UUID。
func (s *ProjectService) NodeID() uuid.UUID {
	if s == nil {
		return uuid.Nil
	}
	return s.nodeID
}

// SetLocalRoot 设置本机 temp / 副本根（storage.local.root）。
func (s *ProjectService) SetLocalRoot(root string) {
	s.localRoot = strings.TrimSpace(root)
}

// SetStorageDriver 记录进程级对象存储驱动（local / s3），供管理台 JSON 只读回显。
func (s *ProjectService) SetStorageDriver(driver string) {
	if s == nil {
		return
	}
	s.storageDriver = strings.ToLower(strings.TrimSpace(driver))
}

// StorageDriver 返回进程级存储驱动；未注入时按 local。
func (s *ProjectService) StorageDriver() string {
	if s == nil {
		return "local"
	}
	d := strings.ToLower(strings.TrimSpace(s.storageDriver))
	if d == "" {
		return "local"
	}
	return d
}

// SetCluster 注入多节点门闩与下载方式（仅 clusterActive 时分支）。
func (s *ProjectService) SetCluster(active bool, download string) {
	s.clusterActive = active
	s.clusterDownload = download
}

// ClusterActive 是否 s3+redis 集群模式（与节点行数无关）。
func (s *ProjectService) ClusterActive() bool {
	return s != nil && s.clusterActive
}

// SetNodeStore 注入节点表（管理查询与同步进度）。
func (s *ProjectService) SetNodeStore(store repository.NodeStore) {
	s.nodes = store
}

// ListNodes 返回集群节点行（未注入仓储时为空列表）。
func (s *ProjectService) ListNodes(ctx context.Context) ([]model.Node, error) {
	if s == nil || s.nodes == nil {
		return []model.Node{}, nil
	}
	return s.nodes.List(ctx)
}

// ListNodeSync 返回某项目各节点同步进度。
func (s *ProjectService) ListNodeSync(ctx context.Context, projectRef string) ([]model.NodeArtifactSync, error) {
	if s == nil || s.nodes == nil {
		return []model.NodeArtifactSync{}, nil
	}
	p, err := s.Resolve(ctx, projectRef)
	if err != nil {
		return nil, err
	}
	return s.nodes.ListSyncByProject(ctx, p.ID)
}

func (s *ProjectService) ownerNodePtr() *uuid.UUID {
	if s == nil || s.nodeID == uuid.Nil {
		return nil
	}
	id := s.nodeID
	return &id
}

// CreateProjectInput 是创建/PATCH 项目的可选设置。创建时 Slug 必填。
type CreateProjectInput struct {
	Slug *string
	// Name 显示名。创建时 nil/省略则等于 slug；显式空白非法。
	Name                     *string
	CompareEngine            *string
	MinimumSupportedVersion  *string
	DefaultLocale            *string
	RequireClientToken       *bool
	StoreToken               *string
	ForceHTTPS               *bool
	CORSOrigins              *[]string
	DeviceIDPolicy           *string
	GrayWeightTenureActivity *bool
	StorageVisibility        *string
	StoragePrefix            *string
	StorageBucket            *string
	WebhookURL               *string
	// WebhookSecretReset 置 true 时重新生成 WebhookSecret（可重置不可读，C14-3）。
	// 配置 WebhookURL 且项目尚无密钥时也会自动生成。
	WebhookSecretReset      *bool
	SigningAlgo             *string
	SigningPublicKey        *string
	SigningPrivateKey       *string
	ChangelogScope          *string
	ChangelogLayout         *string
	ChangelogClientOverride *bool
	ChangelogIncludeRevoked *bool
	ChangelogIncludeNotes   *bool
	RateLimit               *model.JSONObject
	CacheSMaxageSeconds     *int
	SlugAliasRetentionDays  *int
	// TelemetryRetentionDays 遥测事件留存天数；nil = 用默认 90（C11-4）。
	TelemetryRetentionDays *int
	// SignedURLTTLSeconds 私有存储短时签名 URL 项目级 TTL 秒数（§13.7 / C15-1）；
	// nil = 不改写，0 = 回退实例默认 3600。
	SignedURLTTLSeconds *int
	// FileListMaxFiles 原生 file_list 待下载条数项目覆盖；nil = 不改写。
	// 0 = 继承平台天花板；负值与超过天花板非法。
	FileListMaxFiles *int
	// ChangelogDefaultEntries 无 from_version 时的项目条数；nil = 不改写。
	// 必须 1…实例 default，且不超过生效 max。
	ChangelogDefaultEntries *int
	// ChangelogMaxEntries 有 from_version 时的截断上限；nil = 不改写。
	// 0 = 继承实例 max；超过实例天花板非法。
	ChangelogMaxEntries *int
	BootstrapAdminToken *string
	// SystemChannelNames 创建时三条系统渠道的显示名；空字段回退为 slug。
	SystemChannelNames model.SystemChannelNames
	// OwnerUsername 创建项目时可选指定拥有者（已有账号或配合 OwnerPassword 当场创建）。
	OwnerUsername *string
	OwnerPassword *string
}

// PatchProjectInput 使用指针表示「本次是否写入」。
type PatchProjectInput = CreateProjectInput

// IssuedToken 是创建 Token 后一次性返回明文的结果。
type IssuedToken struct {
	Token     *model.ProjectToken
	Plaintext string
}

// Create 分配 UUID v4、校验 slug、写入默认设置。
func (s *ProjectService) Create(ctx context.Context, in CreateProjectInput) (*model.Project, string, error) {
	if in.BootstrapAdminToken != nil {
		return nil, "", ErrBootstrapTokenRejected
	}
	if in.Slug == nil {
		return nil, "", ErrInvalidSlug
	}
	slug := strings.TrimSpace(*in.Slug)
	if err := validateSlug(slug); err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	taken, err := s.store.SlugTaken(ctx, slug, uuid.Nil, now)
	if err != nil {
		return nil, "", err
	}
	if taken {
		return nil, "", ErrSlugTaken
	}
	p := defaultProject(slug)
	storePlain, err := s.applyProjectInput(p, in, false)
	if err != nil {
		return nil, "", err
	}
	if err := s.store.Create(ctx, p); err != nil {
		if isUniqueViolation(err) {
			return nil, "", ErrSlugTaken
		}
		return nil, "", err
	}
	if err := s.store.SeedSystemChannels(ctx, p.ID, in.SystemChannelNames); err != nil {
		return nil, "", err
	}
	if _, err := s.SeedProjectLanguage(ctx, p.ID, p.DefaultLocale); err != nil {
		return nil, "", err
	}
	if err := s.attachCreateOwner(ctx, p.ID, in); err != nil {
		return nil, "", err
	}
	return p, storePlain, nil
}

// List 返回未软删项目。
func (s *ProjectService) List(ctx context.Context) ([]model.Project, error) {
	return s.store.List(ctx)
}

func (s *ProjectService) attachCreateOwner(ctx context.Context, projectID uuid.UUID, in CreateProjectInput) error {
	if in.OwnerUsername == nil || strings.TrimSpace(*in.OwnerUsername) == "" {
		return nil
	}
	if s.admins == nil {
		return fmt.Errorf("%w: cannot assign owner without admin store", ErrInvalidProjectSettings)
	}
	platform := &model.Admin{IsPlatformAdmin: true}
	_, err := s.AddMember(ctx, projectID, platform, MemberWrite{
		Username: *in.OwnerUsername,
		Password: in.OwnerPassword,
		Role:     model.ProjectMemberRoleOwner,
	})
	return err
}

// Resolve 将 :project_ref 解析为 UUID、live slug 或未过期 alias。过期 alias 与缺失均为 ErrProjectNotFound。
func (s *ProjectService) Resolve(ctx context.Context, ref string) (*model.Project, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, ErrProjectNotFound
	}
	now := time.Now().UTC()
	if id, err := uuid.Parse(ref); err == nil {
		if p, ok := s.cachedProjectByID(ctx, id); ok {
			return p, nil
		}
		p, err := s.store.GetByID(ctx, id)
		if err == nil {
			s.fillProjectIdentityCache(ctx, p, cache.ProjectIDKey(p.ID))
			return s.cloneResolvedProject(p), nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if p, ok := s.cachedProjectBySlug(ctx, ref); ok {
		return p, nil
	}
	p, err := s.store.GetBySlug(ctx, ref)
	if err == nil {
		s.fillProjectIdentityCache(ctx, p, cache.ProjectSlugKey(p.Slug), cache.ProjectIDKey(p.ID))
		return s.cloneResolvedProject(p), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if p, ok := s.cachedProjectByAlias(ctx, ref, now); ok {
		return p, nil
	}
	p, exp, err := s.store.GetByUnexpiredAlias(ctx, ref, now)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	s.fillAliasIdentityCache(ctx, p, ref, exp)
	return s.cloneResolvedProject(p), nil
}

// Patch 更新设置。改 slug 时写入旧名 alias；已有 Published Version 时改 compare_engine 返回锁定错误。
func (s *ProjectService) Patch(ctx context.Context, ref string, in PatchProjectInput) (*model.Project, string, error) {
	if in.BootstrapAdminToken != nil {
		return nil, "", ErrBootstrapTokenRejected
	}
	p, err := s.Resolve(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	if in.CompareEngine != nil {
		if err := requireEnum(*in.CompareEngine, model.CompareEngineSemver, model.CompareEngineInteger); err != nil {
			return nil, "", fmt.Errorf("%w: compare_engine", ErrInvalidProjectSettings)
		}
		if *in.CompareEngine != p.CompareEngine {
			published, err := s.store.HasPublishedVersion(ctx, p.ID)
			if err != nil {
				return nil, "", err
			}
			if published {
				return nil, "", ErrCompareEngineImmutable
			}
		}
	}
	oldSlug := p.Slug
	storePlain, err := s.applyProjectInput(p, in, true)
	if err != nil {
		return nil, "", err
	}
	if p.Slug != oldSlug {
		now := time.Now().UTC()
		taken, err := s.store.SlugTaken(ctx, p.Slug, p.ID, now)
		if err != nil {
			return nil, "", err
		}
		if taken {
			return nil, "", ErrSlugTaken
		}
		if err := s.store.DeleteAliasesBySlug(ctx, p.ID, p.Slug); err != nil {
			return nil, "", err
		}
		alias := &model.ProjectSlugAlias{
			ProjectID: p.ID,
			Slug:      oldSlug,
		}
		if p.SlugAliasRetentionDays > 0 {
			exp := now.Add(time.Duration(p.SlugAliasRetentionDays) * 24 * time.Hour)
			alias.ExpiresAt = &exp
		}
		if err := s.store.CreateAlias(ctx, alias); err != nil {
			return nil, "", err
		}
	}
	if err := s.store.Save(ctx, p); err != nil {
		if isUniqueViolation(err) {
			return nil, "", ErrSlugTaken
		}
		return nil, "", err
	}
	if in.DefaultLocale != nil {
		if _, err := s.SeedProjectLanguage(ctx, p.ID, p.DefaultLocale); err != nil {
			return nil, "", err
		}
	}
	s.invalidateProject(ctx, p.ID)
	return p, storePlain, nil
}

// SoftDelete 软删项目，之后 uuid/slug/alias 均不可解析。
func (s *ProjectService) SoftDelete(ctx context.Context, ref string) error {
	p, err := s.Resolve(ctx, ref)
	if err != nil {
		return err
	}
	if err := s.store.SoftDelete(ctx, p.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProjectNotFound
		}
		return err
	}
	s.invalidateProject(ctx, p.ID)
	return nil
}

// CreateToken 签发项目 Token，明文只返回一次。哈希为 SHA-256。
func (s *ProjectService) CreateToken(ctx context.Context, ref, name string, scopes []string, expiresAt *time.Time) (*IssuedToken, error) {
	p, err := s.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidTokenName
	}
	if err := validateScopes(scopes); err != nil {
		return nil, err
	}
	plain, err := newPlaintextToken()
	if err != nil {
		return nil, err
	}
	hash, err := hashToken(plain)
	if err != nil {
		return nil, err
	}
	fp := plain
	if len(fp) > 8 {
		fp = fp[:8]
	}
	tok := &model.ProjectToken{
		ProjectID:   p.ID,
		Name:        name,
		Scopes:      append(model.StringList(nil), scopes...),
		TokenHash:   hash,
		Fingerprint: fp,
		ExpiresAt:   expiresAt,
	}
	if err := s.store.CreateToken(ctx, tok); err != nil {
		return nil, err
	}
	return &IssuedToken{Token: tok, Plaintext: plain}, nil
}

// ListTokens 列出项目 Token（不含哈希与明文）。
func (s *ProjectService) ListTokens(ctx context.Context, ref string) ([]model.ProjectToken, error) {
	p, err := s.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.store.ListTokens(ctx, p.ID)
}

// DeleteToken 吊销（删除）指定项目 Token。
func (s *ProjectService) DeleteToken(ctx context.Context, ref string, tokenID uuid.UUID) error {
	p, err := s.Resolve(ctx, ref)
	if err != nil {
		return err
	}
	if err := s.store.DeleteToken(ctx, p.ID, tokenID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTokenNotFound
		}
		return err
	}
	return nil
}

// IssuedCIToken 包含新建的 CI Token 实体与仅展示一次的明文（C07-1）。
type IssuedCIToken struct {
	Token     *model.CIToken
	Plaintext string
}

func newPlaintextCIToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate ci token: %w", err)
	}
	return "kvc_" + hex.EncodeToString(b[:]), nil
}

// CreateCIToken 为项目签发专用于 CI/CD Agent 的 Token（C07-1）。
func (s *ProjectService) CreateCIToken(ctx context.Context, ref string, name string, scopes []string, expiresAt *time.Time) (*IssuedCIToken, error) {
	p, err := s.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidTokenScopes)
	}
	if err := validateScopes(scopes); err != nil {
		return nil, err
	}
	plain, err := newPlaintextCIToken()
	if err != nil {
		return nil, err
	}
	h, err := hashToken(plain)
	if err != nil {
		return nil, err
	}
	tok := &model.CIToken{
		ProjectID:   p.ID,
		Name:        name,
		Scopes:      model.StringList(scopes),
		TokenHash:   h,
		Fingerprint: plain[:8],
		ExpiresAt:   expiresAt,
	}
	if err := s.store.CreateCIToken(ctx, tok); err != nil {
		return nil, err
	}
	return &IssuedCIToken{Token: tok, Plaintext: plain}, nil
}

// ListCITokens 列出项目全部 CI Token（不含明文）。
func (s *ProjectService) ListCITokens(ctx context.Context, ref string) ([]model.CIToken, error) {
	p, err := s.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.store.ListCITokens(ctx, p.ID)
}

// DeleteCIToken 删除（吊销）指定 CI Token。
func (s *ProjectService) DeleteCIToken(ctx context.Context, ref string, tokenID uuid.UUID) error {
	p, err := s.Resolve(ctx, ref)
	if err != nil {
		return err
	}
	if err := s.store.DeleteCIToken(ctx, p.ID, tokenID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTokenNotFound
		}
		return err
	}
	return nil
}

// LookupProjectToken 按明文哈希查找项目 Token。找不到返回 ErrTokenNotFound。
func (s *ProjectService) LookupProjectToken(ctx context.Context, plaintext string) (*model.ProjectToken, error) {
	hash, err := hashToken(plaintext)
	if err != nil {
		return nil, err
	}
	tok, err := s.store.GetTokenByHash(ctx, hash)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	return tok, err
}

// LookupCIToken 按明文哈希查找 CI Token，供中间件拒绝实例级操作。
func (s *ProjectService) LookupCIToken(ctx context.Context, plaintext string) (*model.CIToken, error) {
	hash, err := hashToken(plaintext)
	if err != nil {
		return nil, err
	}
	tok, err := s.store.GetCITokenByHash(ctx, hash)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	return tok, err
}

// ValidClientToken 判断明文是否为本项目未过期的项目 Token。
func (s *ProjectService) ValidClientToken(ctx context.Context, projectID uuid.UUID, plaintext string) bool {
	tok, err := s.LookupProjectToken(ctx, plaintext)
	if err != nil || tok.ProjectID != projectID {
		return false
	}
	return !model.TokenExpired(tok.ExpiresAt, time.Now().UTC())
}

// ValidStoreToken 判断明文是否匹配项目已配置的 feed Token。
func (s *ProjectService) ValidStoreToken(project *model.Project, plaintext string) bool {
	if project == nil || project.StoreTokenHash == "" || plaintext == "" {
		return false
	}
	hash, err := hashToken(plaintext)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hash), []byte(project.StoreTokenHash)) == 1
}

// SeedCIToken 写入 CI Token 行，仅供测试与后续任务；无对应 HTTP。
func (s *ProjectService) SeedCIToken(ctx context.Context, token *model.CIToken) error {
	if token.TokenHash == "" && token.Name != "" {
		return fmt.Errorf("ci token hash required")
	}
	return s.store.CreateCIToken(ctx, token)
}

// CreatePublishedVersion 插入一条 published 瘦 Version，供锁定测试与后续任务。
func (s *ProjectService) CreatePublishedVersion(ctx context.Context, projectID uuid.UUID) (*model.Version, error) {
	v := &model.Version{
		ProjectID: projectID,
		Status:    model.VersionStatusPublished,
	}
	if err := s.store.CreateVersion(ctx, v); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return v, nil
}

// CreateVersionLine 挂一条瘦 Version Line，供 PACKAGE_TYPE_IMMUTABLE 测试与后续任务。
// os/arch 与矩阵写入相同：darwin→macos、amd64→x86_64；ipados 原样保存。
func (s *ProjectService) CreateVersionLine(ctx context.Context, versionID uuid.UUID, os, arch string) error {
	osSlug, archSlug, err := normalizeMatrixOSArch(os, arch)
	if err != nil {
		return err
	}
	if err := s.store.CreateVersionLine(ctx, &model.VersionLine{
		VersionID: versionID,
		OS:        osSlug,
		Arch:      archSlug,
	}); err != nil {
		return err
	}
	if v, err := s.store.GetVersionByID(ctx, versionID); err == nil {
		s.invalidateProject(ctx, v.ProjectID)
	}
	return nil
}

func defaultProject(slug string) *model.Project {
	// DeviceSecret：hashed 策略的 HMAC 密钥（32 字节 hex），创建即生成；
	// 永不经任何 API 返回（BeforeCreate 仍有兜底生成，防遗漏）。
	secret, err := model.NewDeviceSecret()
	if err != nil {
		// 随机源不可用属进程级故障；留空交给 BeforeCreate 兜底再试。
		secret = ""
	}
	return &model.Project{
		Slug:                     slug,
		Name:                     slug,
		DeviceSecret:             secret,
		TelemetryRetentionDays:   model.DefaultTelemetryRetentionDays,
		CompareEngine:            model.CompareEngineSemver,
		DefaultLocale:            "en",
		RequireClientToken:       false,
		ForceHTTPS:               false,
		CORSOrigins:              model.StringList{},
		DeviceIDPolicy:           model.DeviceIDPolicyHashed,
		GrayWeightTenureActivity: true,
		StorageVisibility:        model.StorageVisibilityPublic,
		SigningAlgo:              model.SigningAlgoEd25519,
		ChangelogScope:           model.ChangelogScopeRangeAll,
		ChangelogLayout:          model.ChangelogLayoutBoth,
		ChangelogClientOverride:  true,
		ChangelogIncludeRevoked:  true,
		ChangelogIncludeNotes:    true,
		ChangelogDefaultEntries:  model.DefaultChangelogDefaultEntries,
		RateLimit:                model.DefaultRateLimit(),
		CacheSMaxageSeconds:      model.DefaultCacheSMaxageSeconds,
		SlugAliasRetentionDays:   model.DefaultSlugAliasRetentionDays,
	}
}

func (s *ProjectService) applyProjectInput(p *model.Project, in CreateProjectInput, patch bool) (string, error) {
	if patch && in.Slug != nil {
		slug := strings.TrimSpace(*in.Slug)
		if err := validateSlug(slug); err != nil {
			return "", err
		}
		p.Slug = slug
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || utf8.RuneCountInString(name) > model.MaxProjectNameRunes {
			return "", fmt.Errorf("%w: name", ErrInvalidProjectSettings)
		}
		p.Name = name
	} else if !patch && p.Name == "" {
		p.Name = p.Slug
	}
	if in.CompareEngine != nil {
		if err := requireEnum(*in.CompareEngine, model.CompareEngineSemver, model.CompareEngineInteger); err != nil {
			return "", fmt.Errorf("%w: compare_engine", ErrInvalidProjectSettings)
		}
		p.CompareEngine = *in.CompareEngine
	}
	if in.MinimumSupportedVersion != nil {
		v := strings.TrimSpace(*in.MinimumSupportedVersion)
		if v == "" {
			p.MinimumSupportedVersion = nil
		} else {
			p.MinimumSupportedVersion = &v
		}
	}
	if !patch {
		if in.DefaultLocale == nil || strings.TrimSpace(*in.DefaultLocale) == "" {
			return "", fmt.Errorf("%w: default_locale", ErrInvalidProjectSettings)
		}
	}
	if in.DefaultLocale != nil {
		loc := strings.TrimSpace(*in.DefaultLocale)
		if loc == "" {
			return "", fmt.Errorf("%w: default_locale", ErrInvalidProjectSettings)
		}
		code, err := normalizeLanguageCode(loc)
		if err != nil {
			return "", fmt.Errorf("%w: default_locale", ErrInvalidProjectSettings)
		}
		p.DefaultLocale = code
	}
	if in.RequireClientToken != nil {
		p.RequireClientToken = *in.RequireClientToken
	}
	var storePlain string
	if in.StoreToken != nil {
		plain := strings.TrimSpace(*in.StoreToken)
		if plain == "" {
			p.StoreTokenHash = ""
		} else {
			hash, err := hashToken(plain)
			if err != nil {
				return "", err
			}
			p.StoreTokenHash = hash
			storePlain = plain
		}
	}
	if in.ForceHTTPS != nil {
		p.ForceHTTPS = *in.ForceHTTPS
	}
	if in.CORSOrigins != nil {
		p.CORSOrigins = append(model.StringList(nil), *in.CORSOrigins...)
	}
	if in.DeviceIDPolicy != nil {
		if err := requireEnum(*in.DeviceIDPolicy, model.DeviceIDPolicyHashed, model.DeviceIDPolicyRaw, model.DeviceIDPolicyNone); err != nil {
			return "", fmt.Errorf("%w: device_id_policy", ErrInvalidProjectSettings)
		}
		p.DeviceIDPolicy = *in.DeviceIDPolicy
	}
	if in.GrayWeightTenureActivity != nil {
		p.GrayWeightTenureActivity = *in.GrayWeightTenureActivity
	}
	if in.StorageVisibility != nil {
		vis := strings.TrimSpace(*in.StorageVisibility)
		if vis == model.StorageVisibilityPrivate {
			return "", fmt.Errorf("%w: storage_visibility must be public", ErrInvalidProjectSettings)
		}
		if err := requireEnum(vis, model.StorageVisibilityPublic); err != nil {
			return "", fmt.Errorf("%w: storage_visibility", ErrInvalidProjectSettings)
		}
		p.StorageVisibility = vis
	}
	if in.StoragePrefix != nil {
		p.StoragePrefix = optionalString(*in.StoragePrefix)
	}
	if in.StorageBucket != nil {
		p.StorageBucket = optionalString(*in.StorageBucket)
	}
	if storagePathConflictsSlug(p.Slug, p.StoragePrefix, p.StorageBucket) {
		return "", fmt.Errorf("%w: storage_prefix or storage_bucket overlaps project slug", ErrInvalidProjectSettings)
	}
	if in.WebhookURL != nil {
		p.WebhookURL = optionalString(*in.WebhookURL)
	}
	// WebhookSecret（C14-3）：首次配置 webhook URL 时自动生成；WebhookSecretReset
	// 为 true 时重新生成（换密钥使旧签名失效）。密钥永不回显（json:"-"）。
	if (in.WebhookSecretReset != nil && *in.WebhookSecretReset) ||
		(p.WebhookURL != nil && *p.WebhookURL != "" && p.WebhookSecret == "") {
		if p.WebhookURL != nil && *p.WebhookURL != "" {
			secret, serr := model.NewWebhookSecret()
			if serr != nil {
				return "", serr
			}
			p.WebhookSecret = secret
		}
	}
	if in.SigningAlgo != nil {
		if err := requireEnum(*in.SigningAlgo, model.SigningAlgoEd25519, model.SigningAlgoRSASHA256); err != nil {
			return "", fmt.Errorf("%w: signing_algo", ErrInvalidProjectSettings)
		}
		p.SigningAlgo = *in.SigningAlgo
	}
	if in.SigningPublicKey != nil {
		p.SigningPublicKey = *in.SigningPublicKey
	}
	if in.SigningPrivateKey != nil {
		p.SigningPrivateKey = *in.SigningPrivateKey
	}
	if in.ChangelogScope != nil {
		if err := requireEnum(*in.ChangelogScope, model.ChangelogScopeRangeAll, model.ChangelogScopeRangePlatform, model.ChangelogScopeTargetOnly); err != nil {
			return "", fmt.Errorf("%w: changelog_scope", ErrInvalidProjectSettings)
		}
		p.ChangelogScope = *in.ChangelogScope
	}
	if in.ChangelogLayout != nil {
		if err := requireEnum(*in.ChangelogLayout, model.ChangelogLayoutAggregated, model.ChangelogLayoutStructured, model.ChangelogLayoutBoth); err != nil {
			return "", fmt.Errorf("%w: changelog_layout", ErrInvalidProjectSettings)
		}
		p.ChangelogLayout = *in.ChangelogLayout
	}
	if in.ChangelogClientOverride != nil {
		p.ChangelogClientOverride = *in.ChangelogClientOverride
	}
	if in.ChangelogIncludeRevoked != nil {
		p.ChangelogIncludeRevoked = *in.ChangelogIncludeRevoked
	}
	if in.ChangelogIncludeNotes != nil {
		p.ChangelogIncludeNotes = *in.ChangelogIncludeNotes
	}
	if in.RateLimit != nil {
		p.RateLimit = cloneJSONObject(*in.RateLimit)
	}
	if in.CacheSMaxageSeconds != nil {
		if *in.CacheSMaxageSeconds < 0 {
			return "", fmt.Errorf("%w: cache_s_maxage_seconds", ErrInvalidProjectSettings)
		}
		p.CacheSMaxageSeconds = *in.CacheSMaxageSeconds
	}
	if in.SlugAliasRetentionDays != nil {
		if *in.SlugAliasRetentionDays < 0 {
			return "", fmt.Errorf("%w: slug_alias_retention_days", ErrInvalidProjectSettings)
		}
		p.SlugAliasRetentionDays = *in.SlugAliasRetentionDays
	}
	if in.TelemetryRetentionDays != nil {
		// 负值非法；0 视为默认 90（§13.11 可配留存）。
		if *in.TelemetryRetentionDays < 0 {
			return "", fmt.Errorf("%w: telemetry_retention_days", ErrInvalidProjectSettings)
		}
		p.TelemetryRetentionDays = *in.TelemetryRetentionDays
	}
	if in.SignedURLTTLSeconds != nil {
		// 私有存储短时签名 URL 项目级 TTL（§13.7 / C15-1）：负值非法；
		// 0 = 回退实例默认 3600s（签名器与目录装配侧兜底）。
		if *in.SignedURLTTLSeconds < 0 {
			return "", fmt.Errorf("%w: signed_url_ttl_seconds", ErrInvalidProjectSettings)
		}
		p.SignedURLTTLSeconds = *in.SignedURLTTLSeconds
	}
	if in.FileListMaxFiles != nil {
		if *in.FileListMaxFiles < 0 {
			return "", fmt.Errorf("%w: file_list_max_files", ErrInvalidProjectSettings)
		}
		ceiling := model.DefaultFileListMaxFiles
		if s != nil {
			ceiling = s.FileListMaxFilesLimit()
		}
		if *in.FileListMaxFiles > ceiling {
			return "", fmt.Errorf("%w: file_list_max_files", ErrInvalidProjectSettings)
		}
		p.FileListMaxFiles = *in.FileListMaxFiles
	}
	if in.ChangelogMaxEntries != nil {
		if *in.ChangelogMaxEntries < 0 {
			return "", fmt.Errorf("%w: changelog_max_entries", ErrInvalidProjectSettings)
		}
		ceiling := model.DefaultChangelogMaxEntries
		if s != nil {
			ceiling = s.ChangelogMaxEntriesLimit()
		}
		if *in.ChangelogMaxEntries > ceiling {
			return "", fmt.Errorf("%w: changelog_max_entries", ErrInvalidProjectSettings)
		}
		p.ChangelogMaxEntries = *in.ChangelogMaxEntries
	}
	if in.ChangelogDefaultEntries != nil {
		if *in.ChangelogDefaultEntries < 1 {
			return "", fmt.Errorf("%w: changelog_default_entries", ErrInvalidProjectSettings)
		}
		ceiling := model.DefaultChangelogDefaultEntries
		if s != nil {
			ceiling = s.ChangelogDefaultEntriesLimit()
		}
		if *in.ChangelogDefaultEntries > ceiling {
			return "", fmt.Errorf("%w: changelog_default_entries", ErrInvalidProjectSettings)
		}
		p.ChangelogDefaultEntries = *in.ChangelogDefaultEntries
	}
	if in.ChangelogMaxEntries != nil || in.ChangelogDefaultEntries != nil {
		instDef := model.DefaultChangelogDefaultEntries
		instMax := model.DefaultChangelogMaxEntries
		if s != nil {
			instDef = s.ChangelogDefaultEntriesLimit()
			instMax = s.ChangelogMaxEntriesLimit()
		}
		effMax := model.EffectiveChangelogMax(instMax, p.ChangelogMaxEntries)
		def := p.ChangelogDefaultEntries
		if def < 1 {
			def = instDef
		}
		if def > effMax {
			return "", fmt.Errorf("%w: changelog_default_entries", ErrInvalidProjectSettings)
		}
	}
	return storePlain, nil
}

func validateSlug(slug string) error {
	if !slugRE.MatchString(slug) {
		return ErrInvalidSlug
	}
	return nil
}

func validateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return ErrInvalidTokenScopes
	}
	for _, s := range scopes {
		if _, ok := model.ValidProjectScopes[s]; !ok {
			return ErrInvalidTokenScopes
		}
	}
	return nil
}

func requireEnum(v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return errors.New("invalid enum")
}

func optionalString(v string) *string {
	s := strings.TrimSpace(v)
	if s == "" {
		return nil
	}
	return &s
}

// storagePathConflictsSlug 当前对象键未使用 prefix/bucket，但仍禁止它们与 slug 同名以免配置歧义。
// prefix：trim、去首尾 `/` 后按 `/` 分段，任一段与 slug 大小写不敏感相等即冲突。
// bucket：同样 trim / 去斜杠，但整段比较（不拆路径）。空值跳过。
func storagePathConflictsSlug(slug string, prefix, bucket *string) bool {
	s := strings.TrimSpace(slug)
	if s == "" {
		return false
	}
	return storageValueConflictsSlug(s, prefix, true) || storageValueConflictsSlug(s, bucket, false)
}

func storageValueConflictsSlug(slug string, raw *string, split bool) bool {
	if raw == nil {
		return false
	}
	v := strings.Trim(strings.TrimSpace(*raw), "/")
	if v == "" {
		return false
	}
	if !split {
		return strings.EqualFold(v, slug)
	}
	for _, part := range strings.Split(v, "/") {
		if part != "" && strings.EqualFold(part, slug) {
			return true
		}
	}
	return false
}

func newPlaintextToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return "kv_" + hex.EncodeToString(b[:]), nil
}

func hashToken(plaintext string) (string, error) {
	return hashutil.SHA256Hex(strings.NewReader(plaintext))
}

func cloneJSONObject(in model.JSONObject) model.JSONObject {
	out := model.JSONObject{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
