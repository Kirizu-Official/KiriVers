package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// AnnouncementStatusDraft 草稿：仅管理端可见，客户端 GET 不返回。
	AnnouncementStatusDraft = "draft"
	// AnnouncementStatusScheduled 定时发布：到达 starts_at 后视为并持久化为 published。
	AnnouncementStatusScheduled = "scheduled"
	// AnnouncementStatusPublished 已发布：在时间窗内对匹配客户端可见。
	AnnouncementStatusPublished = "published"

	// AnnouncementMaxTitleRunes 单条 title 最大 rune 数。
	AnnouncementMaxTitleRunes = 256
	// AnnouncementMaxSubtitleRunes 单条 subtitle 最大 rune 数。
	AnnouncementMaxSubtitleRunes = 256
	// AnnouncementMaxContentBytes 正文（Markdown）最大字节数（64 KiB）。
	AnnouncementMaxContentBytes = 64 * 1024
	// AnnouncementMaxLocales 项目语言行上限的历史名（公告本身一行一种语言）。
	AnnouncementMaxLocales = 16
	// AnnouncementMaxPerProject 单项目公告条数上限。
	AnnouncementMaxPerProject = 500
)

// Announcement 是项目内一条有序公告（维护通知、已知问题、政策说明等）。
// 一行一种语言：language / title / subtitle / content 均为独立列，便于列表索引而不必读取正文。
//
// 用途：运营在管理端维护独立于 Version / changelog / update/check 的公告列表；
// 软件客户端按当前身份（version / os / arch）拉取当前可见且匹配的子集。
// 不嵌入 Version，也不参与 SelectTarget。
//
// 关系：
//   - 归属 Project（ProjectID）；无实例级目录。
//   - VersionID 可空且**无外键**（Job/Audit 模式）：草稿 Version 删除后公告行保留；
//     客户端按查询 version 解析到的 Version UUID 匹配，找不到则跳过版本作用域行。
//     不随 Version 级联删除。
//
// 作用域由 VersionID / OS / Arch 是否为空编码（七种合法组合）：
//   - 项目级：三者皆空；
//   - 仅版本 / 仅系统 / 仅架构；
//   - 版本+系统 / 版本+架构 / 版本+平台矩阵（三者皆有值）。
//     os+arch 且无版本非法。
//
// 字段：
//   - ID：UUID v4 主键，应用侧 BeforeCreate 生成。
//   - ProjectID：所属项目；列表/唯一性均按项目隔离。
//   - Status：draft | scheduled | published；创建默认为 draft；scheduled 必须有 starts_at。
//   - SortOrder：项目内展示序（非唯一）；创建时追加 max+1，reorder 重写 0..n-1。
//   - VersionID：绑定的 Version UUID（可空，无 FK）；写入时必须属于本项目。
//   - OS / Arch：写入时规范化的平台 slug；空表示该维未约束。
//   - Language：该条文案的语言代码（如 zh-CN）；删除 project_languages 行不改写本列。
//   - Title：必填纯文本标题。
//   - Subtitle：可选纯文本副标题。
//   - Content：可选 Markdown 正文；管理端列表查询 Omit 本列。
//   - StartsAt / EndsAt：可选 UTC 窗口；皆空表示发布后立即可见直至取消发布。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
type Announcement struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID uuid.UUID  `gorm:"type:uuid;not null;index:idx_announcements_project_sort,priority:1;index:idx_announcements_project_lang,priority:1" json:"project_id"`
	Status    string     `gorm:"type:text;not null" json:"status"`
	SortOrder int        `gorm:"not null;index:idx_announcements_project_sort,priority:2" json:"sort_order"`
	VersionID *uuid.UUID `gorm:"type:uuid" json:"version_id,omitempty"`
	OS        string     `gorm:"type:text;not null;default:''" json:"os"`
	Arch      string     `gorm:"type:text;not null;default:''" json:"arch"`
	Language  string     `gorm:"type:text;not null;default:'';index:idx_announcements_project_lang,priority:2" json:"language"`
	Title     string     `gorm:"type:text;not null;default:''" json:"title"`
	Subtitle  string     `gorm:"type:text;not null;default:''" json:"subtitle"`
	Content   string     `gorm:"type:text;not null;default:''" json:"content"`
	StartsAt  *time.Time `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TableName 固定表名 announcements。
func (Announcement) TableName() string {
	return "announcements"
}

// BeforeCreate 补 UUID v4 与默认草稿状态。
func (a *Announcement) BeforeCreate(_ *gorm.DB) error {
	if a.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		a.ID = id
	}
	if a.Status == "" {
		a.Status = AnnouncementStatusDraft
	}
	return nil
}
