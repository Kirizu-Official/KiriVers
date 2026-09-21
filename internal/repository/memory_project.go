package repository

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// MemoryProjectStore 是进程内实现，供单测使用，不需要 PostgreSQL。
type MemoryProjectStore struct {
	mu              sync.Mutex
	projects        map[uuid.UUID]*model.Project
	aliases         []*model.ProjectSlugAlias
	tokens          map[uuid.UUID]*model.ProjectToken
	ciTokens        map[uuid.UUID]*model.CIToken
	versions        map[uuid.UUID]*model.Version
	channels        map[uuid.UUID]*model.Channel
	matrix          map[uuid.UUID]*model.PlatformMatrix
	hwRevs          map[uuid.UUID]*model.HwRev
	languages       map[uuid.UUID]*model.ProjectLanguage
	versionLines    map[uuid.UUID]*model.VersionLine
	artifacts       map[uuid.UUID]*model.Artifact
	uploadSessions  map[uuid.UUID]*model.UploadSession
	manifestEntries map[uuid.UUID]*model.ManifestEntry
	grayAllowlist   map[string]*model.GrayAllowlist
	clients         map[uuid.UUID]*model.Client
	clientByHash    map[string]uuid.UUID // projectID|hash → client id
	dailyStats      map[string]*model.ClientDailyStats
	graySnapshots   []*model.GrayRolloutSnapshot
	members         map[string]*model.ProjectMember // projectID|adminID
	listings        map[uuid.UUID]*model.StoreListing
}

// NewMemoryProjectStore 构造空的内存仓储。
func NewMemoryProjectStore() *MemoryProjectStore {
	return &MemoryProjectStore{
		projects:        map[uuid.UUID]*model.Project{},
		tokens:          map[uuid.UUID]*model.ProjectToken{},
		ciTokens:        map[uuid.UUID]*model.CIToken{},
		versions:        map[uuid.UUID]*model.Version{},
		channels:        map[uuid.UUID]*model.Channel{},
		matrix:          map[uuid.UUID]*model.PlatformMatrix{},
		hwRevs:          map[uuid.UUID]*model.HwRev{},
		languages:       map[uuid.UUID]*model.ProjectLanguage{},
		versionLines:    map[uuid.UUID]*model.VersionLine{},
		artifacts:       map[uuid.UUID]*model.Artifact{},
		uploadSessions:  map[uuid.UUID]*model.UploadSession{},
		manifestEntries: map[uuid.UUID]*model.ManifestEntry{},
		grayAllowlist:   map[string]*model.GrayAllowlist{},
		clients:         map[uuid.UUID]*model.Client{},
		clientByHash:    map[string]uuid.UUID{},
		dailyStats:      map[string]*model.ClientDailyStats{},
		members:         map[string]*model.ProjectMember{},
		listings:        map[uuid.UUID]*model.StoreListing{},
	}
}

func (m *MemoryProjectStore) Create(_ context.Context, project *model.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := project.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if project.CreatedAt.IsZero() {
		project.CreatedAt = now
	}
	project.UpdatedAt = now
	for _, p := range m.projects {
		if p.Slug == project.Slug {
			return gorm.ErrDuplicatedKey
		}
	}
	m.projects[project.ID] = cloneProject(project)
	return nil
}

func (m *MemoryProjectStore) GetByID(_ context.Context, id uuid.UUID) (*model.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok || p.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneProject(p), nil
}

func (m *MemoryProjectStore) GetBySlug(_ context.Context, slug string) (*model.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.projects {
		if p.Slug == slug && !p.DeletedAt.Valid {
			return cloneProject(p), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) GetByUnexpiredAlias(_ context.Context, slug string, now time.Time) (*model.Project, *time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.aliases {
		if a.Slug != slug || !a.Unexpired(now) {
			continue
		}
		p, ok := m.projects[a.ProjectID]
		if !ok || p.DeletedAt.Valid {
			continue
		}
		var exp *time.Time
		if a.ExpiresAt != nil {
			t := a.ExpiresAt.UTC()
			exp = &t
		}
		return cloneProject(p), exp, nil
	}
	return nil, nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) List(_ context.Context) ([]model.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.Project, 0, len(m.projects))
	for _, p := range m.projects {
		if p.DeletedAt.Valid {
			continue
		}
		out = append(out, *cloneProject(p))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return bytes.Compare(out[i].ID[:], out[j].ID[:]) > 0
	})
	return out, nil
}

func (m *MemoryProjectStore) ListByIDs(_ context.Context, ids []uuid.UUID) ([]model.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[uuid.UUID]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]model.Project, 0, len(want))
	for id := range want {
		p, ok := m.projects[id]
		if !ok || p.DeletedAt.Valid {
			continue
		}
		out = append(out, *cloneProject(p))
	}
	return out, nil
}

func (m *MemoryProjectStore) Save(_ context.Context, project *model.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.projects[project.ID]
	if !ok || old.DeletedAt.Valid {
		return gorm.ErrRecordNotFound
	}
	project.UpdatedAt = time.Now().UTC()
	project.DeletedAt = old.DeletedAt
	if project.CreatedAt.IsZero() {
		project.CreatedAt = old.CreatedAt
	}
	m.projects[project.ID] = cloneProject(project)
	return nil
}

func (m *MemoryProjectStore) SoftDelete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok || p.DeletedAt.Valid {
		return gorm.ErrRecordNotFound
	}
	now := time.Now().UTC()
	p.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}
	p.UpdatedAt = now
	return nil
}

func (m *MemoryProjectStore) SlugTaken(_ context.Context, slug string, excludeProjectID uuid.UUID, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.projects {
		if p.Slug == slug && p.ID != excludeProjectID {
			return true, nil
		}
	}
	for _, a := range m.aliases {
		if a.Slug == slug && a.Unexpired(now) && a.ProjectID != excludeProjectID {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryProjectStore) CreateAlias(_ context.Context, alias *model.ProjectSlugAlias) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if alias.ID == uuid.Nil {
		alias.ID = uuid.New()
	}
	if alias.CreatedAt.IsZero() {
		alias.CreatedAt = time.Now().UTC()
	}
	cp := *alias
	m.aliases = append(m.aliases, &cp)
	return nil
}

func (m *MemoryProjectStore) DeleteAliasesBySlug(_ context.Context, projectID uuid.UUID, slug string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.aliases[:0]
	for _, a := range m.aliases {
		if a.ProjectID == projectID && a.Slug == slug {
			continue
		}
		kept = append(kept, a)
	}
	m.aliases = kept
	return nil
}

func (m *MemoryProjectStore) CreateToken(_ context.Context, token *model.ProjectToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := token.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = now
	}
	token.UpdatedAt = now
	for _, t := range m.tokens {
		if t.TokenHash == token.TokenHash {
			return gorm.ErrDuplicatedKey
		}
	}
	m.tokens[token.ID] = cloneProjectToken(token)
	return nil
}

func (m *MemoryProjectStore) ListTokens(_ context.Context, projectID uuid.UUID) ([]model.ProjectToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.ProjectToken, 0)
	for _, t := range m.tokens {
		if t.ProjectID == projectID {
			out = append(out, *cloneProjectToken(t))
		}
	}
	return out, nil
}

func (m *MemoryProjectStore) GetTokenByID(_ context.Context, projectID, tokenID uuid.UUID) (*model.ProjectToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[tokenID]
	if !ok || t.ProjectID != projectID {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneProjectToken(t), nil
}

func (m *MemoryProjectStore) GetTokenByHash(_ context.Context, hash string) (*model.ProjectToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tokens {
		if t.TokenHash == hash {
			return cloneProjectToken(t), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) DeleteToken(_ context.Context, projectID, tokenID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[tokenID]
	if !ok || t.ProjectID != projectID {
		return gorm.ErrRecordNotFound
	}
	delete(m.tokens, tokenID)
	return nil
}

func (m *MemoryProjectStore) CreateCIToken(_ context.Context, token *model.CIToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := token.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = now
	}
	token.UpdatedAt = now
	m.ciTokens[token.ID] = cloneCIToken(token)
	return nil
}

func (m *MemoryProjectStore) GetCITokenByHash(_ context.Context, hash string) (*model.CIToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.ciTokens {
		if t.TokenHash == hash {
			return cloneCIToken(t), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) ListCITokens(_ context.Context, projectID uuid.UUID) ([]model.CIToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.CIToken
	for _, t := range m.ciTokens {
		if t.ProjectID == projectID {
			list = append(list, *cloneCIToken(t))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (m *MemoryProjectStore) GetCITokenByID(_ context.Context, projectID, tokenID uuid.UUID) (*model.CIToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.ciTokens[tokenID]
	if !ok || t.ProjectID != projectID {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneCIToken(t), nil
}

func (m *MemoryProjectStore) DeleteCIToken(_ context.Context, projectID, tokenID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.ciTokens[tokenID]
	if !ok || t.ProjectID != projectID {
		return gorm.ErrRecordNotFound
	}
	delete(m.ciTokens, tokenID)
	return nil
}

func (m *MemoryProjectStore) HasPublishedVersion(_ context.Context, projectID uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.versions {
		if v.ProjectID == projectID && v.Status == model.VersionStatusPublished {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryProjectStore) CreateVersion(_ context.Context, version *model.Version) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := version.BeforeCreate(nil); err != nil {
		return err
	}
	for _, v := range m.versions {
		if v.ProjectID == version.ProjectID && v.ID != version.ID {
			if version.VersionInteger != nil && v.VersionInteger != nil && *v.VersionInteger == *version.VersionInteger {
				return gorm.ErrDuplicatedKey
			}
			if version.VersionSemverCanonical != nil && v.VersionSemverCanonical != nil && *v.VersionSemverCanonical == *version.VersionSemverCanonical {
				return gorm.ErrDuplicatedKey
			}
		}
	}
	now := time.Now().UTC()
	if version.CreatedAt.IsZero() {
		version.CreatedAt = now
	}
	version.UpdatedAt = now
	m.versions[version.ID] = cloneVersion(version)
	return nil
}

func (m *MemoryProjectStore) GetVersionByID(_ context.Context, id uuid.UUID) (*model.Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.versions[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneVersion(v), nil
}

func (m *MemoryProjectStore) GetVersionByInteger(_ context.Context, projectID uuid.UUID, versionInt int64) (*model.Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.versions {
		if v.ProjectID == projectID && v.VersionInteger != nil && *v.VersionInteger == versionInt {
			return cloneVersion(v), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) GetVersionBySemverCanonical(_ context.Context, projectID uuid.UUID, canonical string) (*model.Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.versions {
		if v.ProjectID == projectID && v.VersionSemverCanonical != nil && *v.VersionSemverCanonical == canonical {
			return cloneVersion(v), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) GetMaxVersionInteger(_ context.Context, projectID uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var max int64
	for _, v := range m.versions {
		if v.ProjectID == projectID && v.VersionInteger != nil && *v.VersionInteger > max {
			max = *v.VersionInteger
		}
	}
	return max, nil
}

func (m *MemoryProjectStore) SaveVersion(_ context.Context, version *model.Version) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.versions {
		if v.ProjectID == version.ProjectID && v.ID != version.ID {
			if version.VersionInteger != nil && v.VersionInteger != nil && *v.VersionInteger == *version.VersionInteger {
				return gorm.ErrDuplicatedKey
			}
			if version.VersionSemverCanonical != nil && v.VersionSemverCanonical != nil && *v.VersionSemverCanonical == *version.VersionSemverCanonical {
				return gorm.ErrDuplicatedKey
			}
		}
	}
	now := time.Now().UTC()
	version.UpdatedAt = now
	m.versions[version.ID] = cloneVersion(version)
	return nil
}

func (m *MemoryProjectStore) DeleteVersion(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.versions, id)
	for lineID, l := range m.versionLines {
		if l.VersionID == id {
			delete(m.versionLines, lineID)
		}
	}
	// 灰度白名单随版本级联删除（C12；不留孤儿条目）。
	for key, e := range m.grayAllowlist {
		if e.VersionID == id {
			delete(m.grayAllowlist, key)
		}
	}
	return nil
}

func (m *MemoryProjectStore) ListVersions(_ context.Context, projectID uuid.UUID) ([]model.Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.Version
	for _, v := range m.versions {
		if v.ProjectID == projectID {
			list = append(list, *cloneVersion(v))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

// ListPublishedReadyVersionsForPlatform 内存实现：published 版本 ∩ 该 os/arch 的 ready 线。
func (m *MemoryProjectStore) ListPublishedReadyVersionsForPlatform(_ context.Context, projectID uuid.UUID, os, arch string) ([]model.Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ready := map[uuid.UUID]struct{}{}
	for _, l := range m.versionLines {
		if l.Status == model.VersionLineStatusReady && l.OS == os && l.Arch == arch {
			ready[l.VersionID] = struct{}{}
		}
	}
	var list []model.Version
	for _, v := range m.versions {
		if v.ProjectID != projectID || v.Status != model.VersionStatusPublished {
			continue
		}
		if _, ok := ready[v.ID]; !ok {
			continue
		}
		list = append(list, *cloneVersion(v))
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

// ExpireAlias 把指定 slug 的 alias 标为已过期，仅供测试断言 PROJECT_NOT_FOUND。
func (m *MemoryProjectStore) ExpireAlias(slug string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp := at
	for _, a := range m.aliases {
		if a.Slug == slug {
			a.ExpiresAt = &exp
		}
	}
}

func (m *MemoryProjectStore) CreateVersionLine(_ context.Context, line *model.VersionLine) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := line.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if line.CreatedAt.IsZero() {
		line.CreatedAt = now
	}
	line.UpdatedAt = now
	for _, existing := range m.versionLines {
		if existing.VersionID == line.VersionID && existing.OS == line.OS && existing.Arch == line.Arch {
			return gorm.ErrDuplicatedKey
		}
	}
	m.versionLines[line.ID] = cloneVersionLine(line)
	return nil
}

func (m *MemoryProjectStore) ListVersionLines(_ context.Context, versionID uuid.UUID) ([]model.VersionLine, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.VersionLine
	for _, l := range m.versionLines {
		if l.VersionID == versionID {
			list = append(list, *cloneVersionLine(l))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].OS != list[j].OS {
			return list[i].OS < list[j].OS
		}
		return list[i].Arch < list[j].Arch
	})
	return list, nil
}

func (m *MemoryProjectStore) GetVersionLine(_ context.Context, versionID uuid.UUID, os, arch string) (*model.VersionLine, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.versionLines {
		if l.VersionID == versionID && l.OS == os && l.Arch == arch {
			return cloneVersionLine(l), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) SaveVersionLine(_ context.Context, line *model.VersionLine) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	line.UpdatedAt = now
	m.versionLines[line.ID] = cloneVersionLine(line)
	return nil
}

func (m *MemoryProjectStore) CountReadyVersionLines(_ context.Context, versionID uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, l := range m.versionLines {
		if l.VersionID == versionID && l.Status == model.VersionLineStatusReady {
			n++
		}
	}
	return n, nil
}

func (m *MemoryProjectStore) HasPublishedVersionLine(_ context.Context, projectID uuid.UUID, os, arch string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, line := range m.versionLines {
		if line.OS != os || line.Arch != arch {
			continue
		}
		v, ok := m.versions[line.VersionID]
		if ok && v.ProjectID == projectID && v.Status == model.VersionStatusPublished {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryProjectStore) SeedSystemChannels(_ context.Context, projectID uuid.UUID, names model.SystemChannelNames) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for _, seed := range model.SystemChannelSeeds(projectID, names) {
		exists := false
		for _, ch := range m.channels {
			if ch.ProjectID == projectID && ch.Slug == seed.Slug {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		ch := seed
		if err := ch.BeforeCreate(nil); err != nil {
			return err
		}
		ch.CreatedAt = now
		ch.UpdatedAt = now
		cp := ch
		m.channels[ch.ID] = &cp
	}
	return nil
}

func (m *MemoryProjectStore) ListChannels(_ context.Context, projectID uuid.UUID) ([]model.Channel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.Channel, 0)
	for _, ch := range m.channels {
		if ch.ProjectID == projectID {
			cp := *ch
			out = append(out, cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StabilityRank != out[j].StabilityRank {
			return out[i].StabilityRank < out[j].StabilityRank
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

func (m *MemoryProjectStore) GetChannel(_ context.Context, projectID uuid.UUID, slug string) (*model.Channel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.channels {
		if ch.ProjectID == projectID && ch.Slug == slug {
			cp := *ch
			return &cp, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) CreateChannel(_ context.Context, channel *model.Channel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := channel.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if channel.CreatedAt.IsZero() {
		channel.CreatedAt = now
	}
	channel.UpdatedAt = now
	for _, ch := range m.channels {
		if ch.ProjectID == channel.ProjectID && ch.Slug == channel.Slug {
			return gorm.ErrDuplicatedKey
		}
	}
	cp := *channel
	m.channels[channel.ID] = &cp
	return nil
}

func (m *MemoryProjectStore) SaveChannel(_ context.Context, channel *model.Channel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.channels[channel.ID]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	channel.UpdatedAt = time.Now().UTC()
	if channel.CreatedAt.IsZero() {
		channel.CreatedAt = old.CreatedAt
	}
	cp := *channel
	m.channels[channel.ID] = &cp
	return nil
}

func (m *MemoryProjectStore) DeleteChannel(_ context.Context, projectID uuid.UUID, slug string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, ch := range m.channels {
		if ch.ProjectID == projectID && ch.Slug == slug {
			delete(m.channels, id)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) ListMatrix(_ context.Context, projectID uuid.UUID) ([]model.PlatformMatrix, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.PlatformMatrix, 0)
	for _, row := range m.matrix {
		if row.ProjectID == projectID {
			out = append(out, *cloneMatrix(row))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OS != out[j].OS {
			return out[i].OS < out[j].OS
		}
		return out[i].Arch < out[j].Arch
	})
	return out, nil
}

func (m *MemoryProjectStore) GetMatrix(_ context.Context, projectID uuid.UUID, os, arch string) (*model.PlatformMatrix, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.matrix {
		if row.ProjectID == projectID && row.OS == os && row.Arch == arch {
			return cloneMatrix(row), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) CreateMatrix(_ context.Context, row *model.PlatformMatrix) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := row.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	for _, existing := range m.matrix {
		if existing.ProjectID == row.ProjectID && existing.OS == row.OS && existing.Arch == row.Arch {
			return gorm.ErrDuplicatedKey
		}
	}
	m.matrix[row.ID] = cloneMatrix(row)
	return nil
}

func (m *MemoryProjectStore) SaveMatrix(_ context.Context, row *model.PlatformMatrix) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.matrix[row.ID]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	row.UpdatedAt = time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = old.CreatedAt
	}
	m.matrix[row.ID] = cloneMatrix(row)
	return nil
}

func (m *MemoryProjectStore) ListHwRevs(_ context.Context, projectID uuid.UUID) ([]model.HwRev, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.HwRev, 0)
	for _, hw := range m.hwRevs {
		if hw.ProjectID == projectID {
			cp := *hw
			out = append(out, cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

func (m *MemoryProjectStore) GetHwRev(_ context.Context, projectID uuid.UUID, slug string) (*model.HwRev, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, hw := range m.hwRevs {
		if hw.ProjectID == projectID && hw.Slug == slug {
			cp := *hw
			return &cp, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) CreateHwRev(_ context.Context, hw *model.HwRev) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := hw.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if hw.CreatedAt.IsZero() {
		hw.CreatedAt = now
	}
	hw.UpdatedAt = now
	for _, existing := range m.hwRevs {
		if existing.ProjectID == hw.ProjectID && existing.Slug == hw.Slug {
			return gorm.ErrDuplicatedKey
		}
	}
	cp := *hw
	m.hwRevs[hw.ID] = &cp
	return nil
}

func (m *MemoryProjectStore) SaveHwRev(_ context.Context, hw *model.HwRev) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.hwRevs[hw.ID]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	hw.UpdatedAt = time.Now().UTC()
	if hw.CreatedAt.IsZero() {
		hw.CreatedAt = old.CreatedAt
	}
	cp := *hw
	m.hwRevs[hw.ID] = &cp
	return nil
}

func (m *MemoryProjectStore) DeleteHwRev(_ context.Context, projectID uuid.UUID, slug string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, hw := range m.hwRevs {
		if hw.ProjectID == projectID && hw.Slug == slug {
			delete(m.hwRevs, id)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func cloneMatrix(row *model.PlatformMatrix) *model.PlatformMatrix {
	if row == nil {
		return nil
	}
	cp := *row
	if row.MinimumSupportedVersion != nil {
		s := *row.MinimumSupportedVersion
		cp.MinimumSupportedVersion = &s
	}
	return &cp
}

func cloneProject(p *model.Project) *model.Project {
	if p == nil {
		return nil
	}
	cp := *p
	if p.CORSOrigins != nil {
		cp.CORSOrigins = append(model.StringList(nil), p.CORSOrigins...)
	} else {
		cp.CORSOrigins = model.StringList{}
	}
	if p.RateLimit != nil {
		cp.RateLimit = model.JSONObject{}
		for k, v := range p.RateLimit {
			cp.RateLimit[k] = v
		}
	} else {
		cp.RateLimit = model.JSONObject{}
	}
	if p.MinimumSupportedVersion != nil {
		s := *p.MinimumSupportedVersion
		cp.MinimumSupportedVersion = &s
	}
	if p.StoragePrefix != nil {
		s := *p.StoragePrefix
		cp.StoragePrefix = &s
	}
	if p.StorageBucket != nil {
		s := *p.StorageBucket
		cp.StorageBucket = &s
	}
	if p.WebhookURL != nil {
		s := *p.WebhookURL
		cp.WebhookURL = &s
	}
	return &cp
}

func cloneProjectToken(t *model.ProjectToken) *model.ProjectToken {
	if t == nil {
		return nil
	}
	cp := *t
	if t.Scopes != nil {
		cp.Scopes = append(model.StringList(nil), t.Scopes...)
	}
	if t.ExpiresAt != nil {
		exp := *t.ExpiresAt
		cp.ExpiresAt = &exp
	}
	return &cp
}

func cloneCIToken(t *model.CIToken) *model.CIToken {
	if t == nil {
		return nil
	}
	cp := *t
	if t.Scopes != nil {
		cp.Scopes = append(model.StringList(nil), t.Scopes...)
	}
	if t.ExpiresAt != nil {
		exp := *t.ExpiresAt
		cp.ExpiresAt = &exp
	}
	return &cp
}

func cloneVersion(v *model.Version) *model.Version {
	if v == nil {
		return nil
	}
	cp := *v
	if v.VersionInteger != nil {
		val := *v.VersionInteger
		cp.VersionInteger = &val
	}
	if v.VersionSemver != nil {
		val := *v.VersionSemver
		cp.VersionSemver = &val
	}
	if v.VersionSemverCanonical != nil {
		val := *v.VersionSemverCanonical
		cp.VersionSemverCanonical = &val
	}
	if v.GitTag != nil {
		val := *v.GitTag
		cp.GitTag = &val
	}
	if v.GitCommit != nil {
		val := *v.GitCommit
		cp.GitCommit = &val
	}
	if v.GitLogFrom != nil {
		val := *v.GitLogFrom
		cp.GitLogFrom = &val
	}
	if v.GitLogTo != nil {
		val := *v.GitLogTo
		cp.GitLogTo = &val
	}
	if v.MinSourceVersion != nil {
		val := *v.MinSourceVersion
		cp.MinSourceVersion = &val
	}
	if v.ChannelID != nil {
		val := *v.ChannelID
		cp.ChannelID = &val
	}
	if v.Changelog != nil {
		cp.Changelog = make(model.ChangelogMap, len(v.Changelog))
		for k, val := range v.Changelog {
			cp.Changelog[k] = val
		}
	} else {
		cp.Changelog = model.ChangelogMap{}
	}
	if v.AutoPublishWhen != nil {
		rule := *v.AutoPublishWhen
		if v.AutoPublishWhen.RequiredLines != nil {
			rule.RequiredLines = make(model.StringList, len(v.AutoPublishWhen.RequiredLines))
			copy(rule.RequiredLines, v.AutoPublishWhen.RequiredLines)
		}
		cp.AutoPublishWhen = &rule
	}
	if v.PublishTime != nil {
		val := *v.PublishTime
		cp.PublishTime = &val
	}
	if v.GrayStartedAt != nil {
		val := *v.GrayStartedAt
		cp.GrayStartedAt = &val
	}
	if v.GrayCompletedAt != nil {
		val := *v.GrayCompletedAt
		cp.GrayCompletedAt = &val
	}
	return &cp
}

func cloneVersionLine(l *model.VersionLine) *model.VersionLine {
	if l == nil {
		return nil
	}
	cp := *l
	if l.MinOS != nil {
		val := *l.MinOS
		cp.MinOS = &val
	}
	if l.MinAPILevel != nil {
		val := *l.MinAPILevel
		cp.MinAPILevel = &val
	}
	if l.PacksReadyAt != nil {
		val := *l.PacksReadyAt
		cp.PacksReadyAt = &val
	}
	return &cp
}

func (m *MemoryProjectStore) CreateArtifact(_ context.Context, artifact *model.Artifact) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := artifact.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = now
	}
	artifact.UpdatedAt = now
	m.artifacts[artifact.ID] = cloneArtifact(artifact)
	return nil
}

func (m *MemoryProjectStore) SaveArtifact(_ context.Context, artifact *model.Artifact) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.artifacts[artifact.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	artifact.UpdatedAt = time.Now().UTC()
	m.artifacts[artifact.ID] = cloneArtifact(artifact)
	return nil
}

func (m *MemoryProjectStore) GetArtifactByID(_ context.Context, id uuid.UUID) (*model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.artifacts[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneArtifact(a), nil
}

// GetArtifactByLineID 返回该线的 kind=full 产物（切片上可能并存 delta 产物，
// 「该线的全量包」恒指 kind=full，与 GORM 实现约定一致）。
func (m *MemoryProjectStore) GetArtifactByLineID(_ context.Context, lineID uuid.UUID) (*model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var found *model.Artifact
	for _, a := range m.artifacts {
		if a.VersionLineID == lineID && a.Kind == model.ArtifactKindFull {
			if found == nil || a.CreatedAt.After(found.CreatedAt) {
				found = a
			}
		}
	}
	if found == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneArtifact(found), nil
}

// ListArtifactsByLineAndKind 按线与种类列出产物（created_at 升序，确定性）。
func (m *MemoryProjectStore) ListArtifactsByLineAndKind(_ context.Context, lineID uuid.UUID, kind string) ([]model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.Artifact
	for _, a := range m.artifacts {
		if a.VersionLineID == lineID && a.Kind == kind {
			list = append(list, *cloneArtifact(a))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list, nil
}

func (m *MemoryProjectStore) GetArtifactByFileName(_ context.Context, projectID uuid.UUID, fileName string) (*model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var found *model.Artifact
	for _, a := range m.artifacts {
		if a.ProjectID == projectID && a.FileName == fileName {
			if found == nil || a.CreatedAt.After(found.CreatedAt) {
				found = a
			}
		}
	}
	if found == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneArtifact(found), nil
}

func (m *MemoryProjectStore) ListArtifactsByVersionID(_ context.Context, versionID uuid.UUID) ([]model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.Artifact
	for _, a := range m.artifacts {
		if a.VersionID == versionID {
			list = append(list, *cloneArtifact(a))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list, nil
}

// ListArtifactsBySHA256 列出项目内同 SHA-256 的 kind=full 产物（created_at 升序，
// 与 GORM 实现约定一致；仅 full，delta/patch 不可作为复用源）。
func (m *MemoryProjectStore) ListArtifactsBySHA256(_ context.Context, projectID uuid.UUID, sha256 string) ([]model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.Artifact
	for _, a := range m.artifacts {
		if a.ProjectID == projectID && a.Kind == model.ArtifactKindFull && strings.EqualFold(a.SHA256, sha256) {
			list = append(list, *cloneArtifact(a))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list, nil
}

// ListArtifactsByContentSHA256 列出项目内同 SHA-256 的全部 kind 产物。
func (m *MemoryProjectStore) ListArtifactsByContentSHA256(_ context.Context, projectID uuid.UUID, sha256 string) ([]model.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.Artifact
	for _, a := range m.artifacts {
		if a.ProjectID == projectID && strings.EqualFold(a.SHA256, sha256) {
			list = append(list, *cloneArtifact(a))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].ID.String() < list[j].ID.String()
		}
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list, nil
}

func (m *MemoryProjectStore) DeleteArtifact(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.artifacts[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.artifacts, id)
	return nil
}

func (m *MemoryProjectStore) CreateUploadSession(_ context.Context, session *model.UploadSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := session.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	session.UpdatedAt = now
	m.uploadSessions[session.ID] = cloneUploadSession(session)
	return nil
}

func (m *MemoryProjectStore) GetUploadSessionByID(_ context.Context, id uuid.UUID) (*model.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.uploadSessions[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneUploadSession(s), nil
}

func (m *MemoryProjectStore) GetUploadSessionByIdempotencyKey(_ context.Context, projectID uuid.UUID, key string) (*model.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var found *model.UploadSession
	for _, s := range m.uploadSessions {
		if s.ProjectID == projectID && s.IdempotencyKey != nil && *s.IdempotencyKey == key {
			if found == nil || s.CreatedAt.After(found.CreatedAt) {
				found = s
			}
		}
	}
	if found == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneUploadSession(found), nil
}

func (m *MemoryProjectStore) UpdateUploadSession(_ context.Context, session *model.UploadSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session.UpdatedAt = time.Now().UTC()
	m.uploadSessions[session.ID] = cloneUploadSession(session)
	return nil
}

func (m *MemoryProjectStore) DeleteUploadSession(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.uploadSessions[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.uploadSessions, id)
	return nil
}

func (m *MemoryProjectStore) ListActiveUploadSessionsByLineID(_ context.Context, lineID uuid.UUID) ([]model.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.UploadSession
	for _, s := range m.uploadSessions {
		if s.VersionLineID == lineID && s.Status == model.UploadSessionStatusUploading {
			list = append(list, *cloneUploadSession(s))
		}
	}
	return list, nil
}

func (m *MemoryProjectStore) ListExpiredUploadSessions(_ context.Context, projectID uuid.UUID, before time.Time) ([]model.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.UploadSession
	for _, s := range m.uploadSessions {
		if projectID != uuid.Nil && s.ProjectID != projectID {
			continue
		}
		if s.ExpiresAt.Before(before) || s.Status == model.UploadSessionStatusAborted || s.Status == model.UploadSessionStatusExpired {
			list = append(list, *cloneUploadSession(s))
		}
	}
	return list, nil
}

func cloneArtifact(a *model.Artifact) *model.Artifact {
	if a == nil {
		return nil
	}
	cp := *a
	if a.HwRev != nil {
		v := *a.HwRev
		cp.HwRev = &v
	}
	if a.MinHwRev != nil {
		v := *a.MinHwRev
		cp.MinHwRev = &v
	}
	if a.MaxHwRev != nil {
		v := *a.MaxHwRev
		cp.MaxHwRev = &v
	}
	if a.CompatibleHwRevs != nil {
		cp.CompatibleHwRevs = make(model.StringList, len(a.CompatibleHwRevs))
		copy(cp.CompatibleHwRevs, a.CompatibleHwRevs)
	}
	return &cp
}

func cloneUploadSession(s *model.UploadSession) *model.UploadSession {
	if s == nil {
		return nil
	}
	cp := *s
	if s.IdempotencyKey != nil {
		v := *s.IdempotencyKey
		cp.IdempotencyKey = &v
	}
	if s.Metadata != nil {
		cp.Metadata = make(model.JSONObject, len(s.Metadata))
		for k, v := range s.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

func cloneManifestEntry(e *model.ManifestEntry) *model.ManifestEntry {
	if e == nil {
		return nil
	}
	cp := *e
	return &cp
}

func (m *MemoryProjectStore) SaveManifestEntries(_ context.Context, lineID uuid.UUID, entries []model.ManifestEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.manifestEntries {
		if e.VersionLineID == lineID {
			delete(m.manifestEntries, id)
		}
	}
	now := time.Now().UTC()
	for i := range entries {
		e := &entries[i]
		if err := e.BeforeCreate(nil); err != nil {
			return err
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
		e.UpdatedAt = now
		m.manifestEntries[e.ID] = cloneManifestEntry(e)
	}
	return nil
}

func (m *MemoryProjectStore) ListManifestEntries(_ context.Context, lineID uuid.UUID) ([]model.ManifestEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.ManifestEntry
	for _, e := range m.manifestEntries {
		if e.VersionLineID == lineID {
			list = append(list, *cloneManifestEntry(e))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Path < list[j].Path
	})
	return list, nil
}

func (m *MemoryProjectStore) GetManifestEntryByPath(_ context.Context, lineID uuid.UUID, path string) (*model.ManifestEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.manifestEntries {
		if e.VersionLineID == lineID && e.Path == path {
			return cloneManifestEntry(e), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) CountManifestEntries(_ context.Context, lineID uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	for _, e := range m.manifestEntries {
		if e.VersionLineID == lineID {
			count++
		}
	}
	return count, nil
}

func (m *MemoryProjectStore) DeleteManifestEntriesByLineID(_ context.Context, lineID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.manifestEntries {
		if e.VersionLineID == lineID {
			delete(m.manifestEntries, id)
		}
	}
	return nil
}

// LineDetails 实现 update.LineDetailSource：返回该线 Manifest 条目（按 Path
// 升序）与 kind ∈ {delta, patch, file} 的产物快照，供 integrity/diff 端点按需读取。
// delta 产物映射为 binary_delta（§7.2）、patch 产物映射为 patch_package（§7.5）：
// Algo 与源/目标 SHA-256 来自生成 Job 写入的 artifacts 列
// （DeltaAlgo / DeltaSourceSHA256 / DeltaTargetSHA256）。
func (m *MemoryProjectStore) LineDetails(_ context.Context, lineID uuid.UUID) (*update.LineDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var entries []model.ManifestEntry
	for _, e := range m.manifestEntries {
		if e.VersionLineID == lineID {
			entries = append(entries, *cloneManifestEntry(e))
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	detail := &update.LineDetail{Manifest: entries}
	for _, a := range m.artifacts {
		if a.VersionLineID != lineID {
			continue
		}
		switch a.Kind {
		case model.ArtifactKindDelta:
			detail.Deltas = append(detail.Deltas, update.DeltaArtifactInfo{
				Kind:              update.DeltaKindBinaryDelta,
				FileName:          a.FileName,
				Size:              a.Size,
				SHA256:            a.SHA256,
				Algo:              a.DeltaAlgo,
				SourceSHA256:      a.DeltaSourceSHA256,
				TargetSHA256:      a.DeltaTargetSHA256,
				HwRev:             a.HwRev,
				ArtifactSignature: a.ArtifactSignature,
				StorageKey:        a.StorageKey,
			})
		case model.ArtifactKindPatch:
			detail.Patches = append(detail.Patches, update.DeltaArtifactInfo{
				Kind:              update.DeltaKindPatchPackage,
				FileName:          a.FileName,
				Size:              a.Size,
				SHA256:            a.SHA256,
				SourceSHA256:      a.DeltaSourceSHA256,
				TargetSHA256:      a.DeltaTargetSHA256,
				HwRev:             a.HwRev,
				ArtifactSignature: a.ArtifactSignature,
				StorageKey:        a.StorageKey,
			})
		case model.ArtifactKindFile:
			detail.Files = append(detail.Files, update.FileArtifactInfo{
				FileName:   a.FileName,
				StorageKey: a.StorageKey,
				Size:       a.Size,
				SHA256:     a.SHA256,
				MD5:        a.MD5,
				HwRev:      a.HwRev,
			})
		}
	}
	return detail, nil
}

func memberKey(projectID, adminID uuid.UUID) string {
	return projectID.String() + "|" + adminID.String()
}

func (m *MemoryProjectStore) CreateMember(_ context.Context, member *model.ProjectMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := member.BeforeCreate(nil); err != nil {
		return err
	}
	key := memberKey(member.ProjectID, member.AdminID)
	if _, ok := m.members[key]; ok {
		return gorm.ErrDuplicatedKey
	}
	now := time.Now().UTC()
	if member.CreatedAt.IsZero() {
		member.CreatedAt = now
	}
	member.UpdatedAt = now
	cp := *member
	m.members[key] = &cp
	return nil
}

func (m *MemoryProjectStore) GetMember(_ context.Context, projectID, adminID uuid.UUID) (*model.ProjectMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.members[memberKey(projectID, adminID)]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *row
	return &cp, nil
}

func (m *MemoryProjectStore) ListMembers(_ context.Context, projectID uuid.UUID) ([]model.ProjectMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.ProjectMember
	for _, row := range m.members {
		if row.ProjectID == projectID {
			list = append(list, *row)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	return list, nil
}

func (m *MemoryProjectStore) ListProjectIDsForAdmin(_ context.Context, adminID uuid.UUID) ([]uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []uuid.UUID
	for _, row := range m.members {
		if row.AdminID == adminID {
			ids = append(ids, row.ProjectID)
		}
	}
	return ids, nil
}

func (m *MemoryProjectStore) CountOwners(_ context.Context, projectID uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, row := range m.members {
		if row.ProjectID == projectID && row.Role == model.ProjectMemberRoleOwner {
			n++
		}
	}
	return n, nil
}

func (m *MemoryProjectStore) SaveMember(_ context.Context, member *model.ProjectMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := memberKey(member.ProjectID, member.AdminID)
	if _, ok := m.members[key]; !ok {
		return gorm.ErrRecordNotFound
	}
	member.UpdatedAt = time.Now().UTC()
	cp := *member
	m.members[key] = &cp
	return nil
}

func (m *MemoryProjectStore) DeleteMember(_ context.Context, projectID, adminID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := memberKey(projectID, adminID)
	if _, ok := m.members[key]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.members, key)
	return nil
}

func cloneStoreListing(row *model.StoreListing) *model.StoreListing {
	if row == nil {
		return nil
	}
	cp := *row
	if row.Identifiers != nil {
		cp.Identifiers = model.IdentifierMap{}
		for k, v := range row.Identifiers {
			cp.Identifiers[k] = v
		}
	} else {
		cp.Identifiers = model.IdentifierMap{}
	}
	if row.OS != nil {
		s := *row.OS
		cp.OS = &s
	}
	if row.Arch != nil {
		s := *row.Arch
		cp.Arch = &s
	}
	if row.Channel != nil {
		s := *row.Channel
		cp.Channel = &s
	}
	return &cp
}

func (m *MemoryProjectStore) ListStoreListings(_ context.Context, projectID uuid.UUID) ([]model.StoreListing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.StoreListing
	for _, row := range m.listings {
		if row.ProjectID == projectID {
			list = append(list, *cloneStoreListing(row))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Protocol == list[j].Protocol {
			return list[i].Slug < list[j].Slug
		}
		return list[i].Protocol < list[j].Protocol
	})
	return list, nil
}

func (m *MemoryProjectStore) GetStoreListing(_ context.Context, projectID uuid.UUID, protocol, slug string) (*model.StoreListing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.listings {
		if row.ProjectID == projectID && row.Protocol == protocol && row.Slug == slug {
			return cloneStoreListing(row), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *MemoryProjectStore) GetStoreListingByID(_ context.Context, projectID, id uuid.UUID) (*model.StoreListing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.listings[id]
	if !ok || row.ProjectID != projectID {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneStoreListing(row), nil
}

func (m *MemoryProjectStore) CreateStoreListing(_ context.Context, listing *model.StoreListing) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := listing.BeforeCreate(nil); err != nil {
		return err
	}
	for _, row := range m.listings {
		if row.ProjectID == listing.ProjectID && row.Protocol == listing.Protocol && row.Slug == listing.Slug {
			return gorm.ErrDuplicatedKey
		}
	}
	now := time.Now().UTC()
	if listing.CreatedAt.IsZero() {
		listing.CreatedAt = now
	}
	listing.UpdatedAt = now
	m.listings[listing.ID] = cloneStoreListing(listing)
	return nil
}

func (m *MemoryProjectStore) SaveStoreListing(_ context.Context, listing *model.StoreListing) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.listings[listing.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	for _, row := range m.listings {
		if row.ID == listing.ID {
			continue
		}
		if row.ProjectID == listing.ProjectID && row.Protocol == listing.Protocol && row.Slug == listing.Slug {
			return gorm.ErrDuplicatedKey
		}
	}
	listing.UpdatedAt = time.Now().UTC()
	m.listings[listing.ID] = cloneStoreListing(listing)
	return nil
}

func (m *MemoryProjectStore) DeleteStoreListing(_ context.Context, projectID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.listings[id]
	if !ok || row.ProjectID != projectID {
		return gorm.ErrRecordNotFound
	}
	delete(m.listings, id)
	return nil
}

func (m *MemoryProjectStore) HasEnabledStoreListing(_ context.Context, projectID uuid.UUID, protocol string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.listings {
		if row.ProjectID == projectID && row.Protocol == protocol && row.Enabled {
			return true, nil
		}
	}
	return false, nil
}
