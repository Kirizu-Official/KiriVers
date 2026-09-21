package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// MediaMaxBytes 是 Markdown 编辑器上传单文件上限（10 MiB）。
	MediaMaxBytes int64 = 10 * 1024 * 1024
	// MediaInlineJPEG 等用于 Content-Disposition: inline 的图片 MIME。
	MediaInlineJPEG = "image/jpeg"
	MediaInlinePNG  = "image/png"
	MediaInlineGIF  = "image/gif"
	MediaInlineWebP = "image/webp"
)

// ProjectMedia 是项目 Markdown 编辑器上传的图片/附件元数据（非 Artifact）。
//
// 用途：
//
//	运营在 changelog / 公告 Markdown 中插入图片与附件。字节走与产物相同的
//	storage.Backend，键为 media/{project_id}/{id}/{filename}。客户端 GET 按 UUID
//	公开读取（无需项目 Token、无需 urlsign）；知道 UUID 即可下载，不可猜测即不可见。
//
// 关系：
//   - 归属于 Project（ProjectID），无 Version / VersionLine FK。
//   - 不是 Artifact，不参与 TUS / PresignPut / 更新包。
//
// 字段含义：
//   - ID: 媒体对象 UUID（客户端 GET 路径段；应用侧 BeforeCreate 生成 v4）
//   - ProjectID: 所属项目；GET 必须同时匹配项目与 ID，否则 404
//   - FileName: 原始安全文件名（用于 Content-Disposition）
//   - StorageKey: storage.Backend 主键（media/{project_id}/{id}/{filename}）
//   - Size: 字节数
//   - ContentType: 存储与响应的 MIME
//   - SHA256 / MD5 / SHA512: 上传时写入的小写 hex 哈希（新上传必填；不回填历史行）
//   - CreatedAt / UpdatedAt: GORM 时间戳
type ProjectMedia struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID   uuid.UUID `gorm:"type:uuid;not null;index" json:"project_id"`
	FileName    string    `gorm:"type:text;not null" json:"file_name"`
	StorageKey  string    `gorm:"type:text;not null" json:"storage_key"`
	Size        int64     `gorm:"not null" json:"size"`
	ContentType string    `gorm:"type:text;not null;default:application/octet-stream" json:"content_type"`
	SHA256      string    `gorm:"type:text;not null;default:''" json:"sha256"`
	MD5         string    `gorm:"type:text;not null;default:''" json:"md5"`
	SHA512      string    `gorm:"type:text;not null;default:''" json:"sha512"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 固定表名 project_media。
func (ProjectMedia) TableName() string {
	return "project_media"
}

// BeforeCreate 补全 UUID v4 与默认 MIME。
func (m *ProjectMedia) BeforeCreate(_ *gorm.DB) error {
	if m.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		m.ID = id
	}
	if m.ContentType == "" {
		m.ContentType = "application/octet-stream"
	}
	return nil
}

// MediaInline 判断该 MIME 是否应作为图片在浏览器内联展示。
func MediaInline(contentType string) bool {
	switch contentType {
	case MediaInlineJPEG, MediaInlinePNG, MediaInlineGIF, MediaInlineWebP:
		return true
	default:
		return false
	}
}
