package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// projectRecord 是身份缓存信封：model.Project 若干字段为 json:"-"（密钥），
// 必须显式带上，否则命中缓存后 hashed HMAC / feed / 签名会丢材料。
type projectRecord struct {
	Project           model.Project `json:"project"`
	DeviceSecret      string        `json:"device_secret"`
	SigningPrivateKey string        `json:"signing_private_key"`
	StoreTokenHash     string        `json:"store_token_hash"`
	WebhookSecret     string        `json:"webhook_secret"`
}

type aliasRecord struct {
	projectRecord
	ExpiresAt *time.Time `json:"expires_at"`
}

func packProject(p *model.Project) projectRecord {
	return projectRecord{
		Project:           *p,
		DeviceSecret:      p.DeviceSecret,
		SigningPrivateKey: p.SigningPrivateKey,
		StoreTokenHash:     p.StoreTokenHash,
		WebhookSecret:     p.WebhookSecret,
	}
}

func unpackProject(rec projectRecord) *model.Project {
	p := rec.Project
	p.DeviceSecret = rec.DeviceSecret
	p.SigningPrivateKey = rec.SigningPrivateKey
	p.StoreTokenHash = rec.StoreTokenHash
	p.WebhookSecret = rec.WebhookSecret
	return &p
}

// clonePackedProject 通过信封 JSON 深拷贝，避免调用方改写缓存或仓储中的同一指针。
func clonePackedProject(p *model.Project) *model.Project {
	if p == nil {
		return nil
	}
	raw, err := json.Marshal(packProject(p))
	if err != nil {
		cp := *p
		return &cp
	}
	var rec projectRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		cp := *p
		return &cp
	}
	return unpackProject(rec)
}

func (s *ProjectService) cloneResolvedProject(p *model.Project) *model.Project {
	if s == nil || s.cache == nil {
		return p
	}
	return clonePackedProject(p)
}

func (s *ProjectService) cachedProjectByID(ctx context.Context, id uuid.UUID) (*model.Project, bool) {
	return s.readProjectCache(ctx, cache.ProjectIDKey(id), false, time.Time{})
}

func (s *ProjectService) cachedProjectBySlug(ctx context.Context, slug string) (*model.Project, bool) {
	return s.readProjectCache(ctx, cache.ProjectSlugKey(slug), false, time.Time{})
}

func (s *ProjectService) cachedProjectByAlias(ctx context.Context, slug string, now time.Time) (*model.Project, bool) {
	return s.readProjectCache(ctx, cache.ProjectAliasKey(slug), true, now)
}

func (s *ProjectService) readProjectCache(ctx context.Context, key string, alias bool, now time.Time) (*model.Project, bool) {
	if s == nil || s.cache == nil || key == "" {
		return nil, false
	}
	raw, ok, err := s.cache.Get(ctx, key)
	if err != nil || !ok {
		return nil, false
	}
	if alias {
		var rec aliasRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			_ = s.cache.Delete(ctx, key)
			return nil, false
		}
		if rec.ExpiresAt != nil && !rec.ExpiresAt.After(now) {
			_ = s.cache.Delete(ctx, key)
			return nil, false
		}
		return unpackProject(rec.projectRecord), true
	}
	var rec projectRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		_ = s.cache.Delete(ctx, key)
		return nil, false
	}
	return unpackProject(rec), true
}

func (s *ProjectService) fillProjectIdentityCache(ctx context.Context, p *model.Project, keys ...string) {
	if s == nil || s.cache == nil || p == nil {
		return
	}
	raw, err := json.Marshal(packProject(p))
	if err != nil {
		return
	}
	for _, key := range keys {
		_ = s.cache.Set(ctx, key, raw)
	}
	s.recordLookups(ctx, p.ID, keys...)
}

func (s *ProjectService) fillAliasIdentityCache(ctx context.Context, p *model.Project, slug string, exp *time.Time) {
	if s == nil || s.cache == nil || p == nil {
		return
	}
	rec := aliasRecord{projectRecord: packProject(p), ExpiresAt: exp}
	raw, err := json.Marshal(rec)
	if err != nil {
		return
	}
	aliasKey := cache.ProjectAliasKey(slug)
	idKey := cache.ProjectIDKey(p.ID)
	_ = s.cache.Set(ctx, aliasKey, raw)
	idRaw, err := json.Marshal(packProject(p))
	if err == nil {
		_ = s.cache.Set(ctx, idKey, idRaw)
	}
	s.recordLookups(ctx, p.ID, aliasKey, idKey)
}

func (s *ProjectService) recordLookups(ctx context.Context, id uuid.UUID, keys ...string) {
	if s.cache == nil || id == uuid.Nil {
		return
	}
	lookupsKey := cache.ProjectLookupsKey(id)
	var existing []string
	if raw, ok, err := s.cache.Get(ctx, lookupsKey); err == nil && ok {
		_ = json.Unmarshal(raw, &existing)
	}
	seen := make(map[string]struct{}, len(existing)+len(keys))
	for _, k := range existing {
		seen[k] = struct{}{}
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		existing = append(existing, k)
		seen[k] = struct{}{}
	}
	raw, err := json.Marshal(existing)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, lookupsKey, raw)
}

func (s *ProjectService) invalidateProject(ctx context.Context, id uuid.UUID) {
	if s == nil || s.cache == nil || id == uuid.Nil {
		return
	}
	if err := cache.InvalidateProject(ctx, s.cache, id); err != nil {
		s.cacheLog.Error().Err(err).Str("project_id", id.String()).Msg("invalidate project cache")
	}
}
