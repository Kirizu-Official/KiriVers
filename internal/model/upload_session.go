package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// UploadSessionStatusUploading 分片上传进行中。
	UploadSessionStatusUploading = "uploading"
	// UploadSessionStatusCompleted 分片上传已完成并转为正式产物。
	UploadSessionStatusCompleted = "completed"
	// UploadSessionStatusAborted 客户端或管理端主动中止。
	UploadSessionStatusAborted = "aborted"
	// UploadSessionStatusExpired 会话已超时失效。
	UploadSessionStatusExpired = "expired"
)

// UploadSession 记录 TUS 1.0 或大文件分片上传会话状态。
//
// 用途：
//   跟踪分片上传的当前偏移量（Upload-Offset）、预期总大小（Upload-Length）、
//   临时存储对象键、会话状态（uploading/completed/aborted/expired）、过期时间，
//   并在未完成时阻塞版本发布（UPLOAD_INCOMPLETE）。
//
// 关系：
//   - 关联 Project、Version、VersionLine
//
// 字段含义：
//   - ID: 会话唯一 UUID（作为 TUS URL 中的 upload_id）
//   - ProjectID: 所属项目 UUID
//   - VersionID: 所属版本 UUID
//   - VersionLineID: 所属平台切片 UUID
//   - StorageKey: 临时分片/组装文件在 storage.Backend 的键
//   - Offset: 当前已写入字节数（TUS Upload-Offset）
//   - Size: 预期上传总字节数（TUS Upload-Length）
//   - Status: 当前状态（uploading | completed | aborted | expired）
//   - IdempotencyKey: 客户端上传关联的幂等键（可选，窗口 24h）
//   - Metadata: 客户端初始上传元数据（文件名、hw_rev、哈希、签名等）
//   - ExpiresAt: 会话过期时间（默认 24 小时）
//   - CreatedAt / UpdatedAt: 时间戳
type UploadSession struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"project_id"`
	VersionID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"version_id"`
	VersionLineID  uuid.UUID  `gorm:"type:uuid;not null;index:idx_upload_sessions_line_status,priority:1" json:"version_line_id"`
	StorageKey     string     `gorm:"type:text;not null" json:"storage_key"`
	Offset         int64      `gorm:"not null;default:0" json:"offset"`
	Size           int64      `gorm:"not null" json:"size"`
	Status         string     `gorm:"type:text;not null;default:uploading;index:idx_upload_sessions_line_status,priority:2" json:"status"`
	// IdempotencyKey 查找走 extras 的 (project_id, idempotency_key) partial btree。
	IdempotencyKey *string    `gorm:"type:text" json:"idempotency_key,omitempty"`
	Metadata       JSONObject `gorm:"type:jsonb" json:"metadata,omitempty"`
	ExpiresAt      time.Time  `gorm:"index" json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName 固定表名 upload_sessions。
func (UploadSession) TableName() string {
	return "upload_sessions"
}

// BeforeCreate 补全 UUID 与默认状态。
func (u *UploadSession) BeforeCreate(_ *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	if u.Status == "" {
		u.Status = UploadSessionStatusUploading
	}
	if u.ExpiresAt.IsZero() {
		u.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	return nil
}
