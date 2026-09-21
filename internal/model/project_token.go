package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// ScopeProjectRead 可读项目配置与后续客户端 check。
	ScopeProjectRead = "project:read"
	// ScopeArtifactWrite 可上传产物。
	ScopeArtifactWrite = "artifact:write"
	// ScopeReleasePublish 可发版。
	ScopeReleasePublish = "release:publish"
	// ScopeProjectAdmin 可改本项目设置并签发本项目 Token。
	ScopeProjectAdmin = "project:admin"
)

// ValidProjectScopes 是本任务定义的全部项目/CI Token scope。
var ValidProjectScopes = map[string]struct{}{
	ScopeProjectRead:    {},
	ScopeArtifactWrite:  {},
	ScopeReleasePublish: {},
	ScopeProjectAdmin:   {},
}

// ProjectToken 是项目级 API Token（明文 kv_… 只在创建响应出现一次）。
//
// 用途：客户端 Bearer / X-Project-Token，以及拥有 project:admin 时管理本项目。
// 不能创建项目、改实例配置、签发其它项目的 Token。
//
// 关系：归属 Project。与 CIToken 同形但分表，避免与后续 CI 创建 API 混用。
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：所属项目。
//   - Name：人类可读名称。
//   - Scopes：四个 scope 的 jsonb 数组。
//   - TokenHash：明文 SHA-256 hex。
//   - Fingerprint：明文前 8 个字符，便于审计，不含完整密钥。
//   - ExpiresAt：可空；过期后不得再当有效凭证。
type ProjectToken struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"project_id"`
	Name        string     `gorm:"type:text;not null" json:"name"`
	Scopes      StringList `gorm:"type:jsonb" json:"scopes"`
	TokenHash   string     `gorm:"type:text;uniqueIndex;not null" json:"-"`
	Fingerprint string     `gorm:"type:text;not null" json:"fingerprint"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 固定表名 project_tokens。
func (ProjectToken) TableName() string {
	return "project_tokens"
}

// BeforeCreate 补 UUID。
func (t *ProjectToken) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.Scopes == nil {
		t.Scopes = StringList{}
	}
	return nil
}

// CIToken 与 ProjectToken 同形，专给后续 CI Agent 发版使用。
//
// 用途：本任务只建表与哈希查找，使中间件能把 CI Token 识别为「非 instance:admin」。
// 创建 HTTP 不在本任务。
//
// 关系：归属 Project；不得用于创建项目或签发跨项目 Token。
type CIToken struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"project_id"`
	Name        string     `gorm:"type:text;not null" json:"name"`
	Scopes      StringList `gorm:"type:jsonb" json:"scopes"`
	TokenHash   string     `gorm:"type:text;uniqueIndex;not null" json:"-"`
	Fingerprint string     `gorm:"type:text;not null" json:"fingerprint"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 固定表名 ci_tokens。
func (CIToken) TableName() string {
	return "ci_tokens"
}

// BeforeCreate 补 UUID。
func (t *CIToken) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.Scopes == nil {
		t.Scopes = StringList{}
	}
	return nil
}

// TokenExpired 在 now 是否已过期。ExpiresAt 为空表示不过期。
func TokenExpired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !expiresAt.After(now)
}

// ScopesInclude 判断 scopes 是否满足 want。project:admin 视为拥有全部项目 scope。
func ScopesInclude(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == ScopeProjectAdmin || s == want {
			return true
		}
	}
	return false
}
