package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Webhook 投递状态常量（C14-4，§5.8 / §11.5）。
const (
	// WebhookDeliveryStatusPending 待投递（含等待退避重试）。
	WebhookDeliveryStatusPending = "pending"
	// WebhookDeliveryStatusDelivered 已成功投递（收到 2xx）。
	WebhookDeliveryStatusDelivered = "delivered"
	// WebhookDeliveryStatusFailed 终态失败：重试次数用尽（默认 3 次）仍未收到 2xx。
	WebhookDeliveryStatusFailed = "failed"
)

// Publish webhook 事件类型常量（C14-3，§5.8）。
const (
	// WebhookEventVersionPublished 版本发布成功（PublishVersion 幂等首次推进时触发）。
	WebhookEventVersionPublished = "version.published"
	// WebhookEventVersionLineReady 已发布版本的新平台切片就绪（补平台场景）。
	WebhookEventVersionLineReady = "version_line.ready"
	// WebhookEventVersionRevoked 版本吊销。
	WebhookEventVersionRevoked = "version.revoked"
)

// WebhookDeliveryMaxAttempts 单条事件的最大投递尝试次数（首投 + 2 次重试）。
const WebhookDeliveryMaxAttempts = 3

// WebhookDelivery 是一次 Publish webhook 事件的投递记录（C14-3, C14-4）。
//
// 用途：事件触发点（版本发布 / 平台切片就绪 / 吊销）只创建本行并入队
// webhook_deliver 任务（入队即返回，绝不阻塞 HTTP，§11.5）；worker 依据本行
// 携带的载荷向 Project.WebhookURL 发送签名 POST，并回写投递结果，
// 供管理端排障查询（GET /admin/projects/:ref/webhook/deliveries）。
//
// 关系：
//   - 归属 Project（ProjectID）；Project.WebhookSecret 用于 HMAC 签名。
//   - 可选关联 Version（VersionID）与 VersionLine（LineID）。
//   - 由 webhook_deliver 类型的 Job 驱动状态流转。
//
// 字段：
//   - ID：投递记录 UUID 主键，应用侧生成。
//   - ProjectID：所属项目 UUID（webhook URL 与签名密钥的持有方）。
//   - Event：事件类型（version.published | version_line.ready | version.revoked）。
//   - VersionID：关联版本 UUID（可空）。
//   - LineID：关联平台切片 UUID（仅 version_line.ready 事件非空）。
//   - Payload：事件数据 jsonb；投递时包装为
//     {event, project_id, version_id, line_id, data} 信封后整体签名发送。
//   - Status：pending | delivered | failed。
//   - Attempts：已执行的 HTTP POST 尝试次数（含失败）。
//   - LastStatusCode：最近一次尝试的 HTTP 响应码（网络错误为 0）。
//   - LastError：最近一次失败的错误描述；成功后清空。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
//   - DeliveredAt：进入 delivered 终态的时间。
type WebhookDelivery struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID      uuid.UUID  `gorm:"type:uuid;not null;index:idx_webhook_deliveries_project_created,priority:1" json:"project_id"`
	Event          string     `gorm:"type:text;not null" json:"event"`
	VersionID      *uuid.UUID `gorm:"type:uuid" json:"version_id,omitempty"`
	LineID         *uuid.UUID `gorm:"type:uuid" json:"line_id,omitempty"`
	Payload        []byte     `gorm:"type:jsonb" json:"payload,omitempty"`
	Status         string     `gorm:"type:text;not null;default:pending;index" json:"status"`
	Attempts       int        `gorm:"not null;default:0" json:"attempts"`
	LastStatusCode int        `gorm:"not null;default:0" json:"last_status_code"`
	LastError      string     `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt      time.Time  `gorm:"index:idx_webhook_deliveries_project_created,priority:2" json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
}

// TableName 固定表名 webhook_deliveries。
func (WebhookDelivery) TableName() string {
	return "webhook_deliveries"
}

// BeforeCreate 补 UUID 与默认 pending 状态。
func (d *WebhookDelivery) BeforeCreate(_ *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.Status == "" {
		d.Status = WebhookDeliveryStatusPending
	}
	return nil
}
