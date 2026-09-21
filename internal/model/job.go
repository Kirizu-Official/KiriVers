package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// JobStatusQueued 等待 worker 抢占。
	JobStatusQueued = "queued"
	// JobStatusRunning 已被某个进程的 worker 锁定执行。
	JobStatusRunning = "running"
	// JobStatusSucceeded 处理成功。
	JobStatusSucceeded = "succeeded"
	// JobStatusFailed 处理失败（可按 Attempts 决定是否重试，本任务不实现重试策略）。
	JobStatusFailed = "failed"
)

// Job 是进程内异步任务行，供多副本用 PostgreSQL SKIP LOCKED 抢占。
//
// 用途：解压 zip、哈希、差量生成、feed 刷新等长任务不阻塞 HTTP。本任务只建立表与空转 worker，
// 不注册具体 job type handler。
//
// 关系：可选归属某个 Project（ProjectID）。projects 表已存在，但本列仍不加 FK，保持实例级任务可空。
//
// 字段：
//   - ID：UUID 主键，应用侧生成。
//   - Type：任务类型字符串（如 bundle_unpack）。
//   - Status：queued | running | succeeded | failed。
//   - Payload：JSON 载荷（jsonb），handler 自行解码。
//   - Result：处理产出 JSON 结果（jsonb），成功或部分失败时记录详细拆线清单。
//   - Progress：0–100 可选进度。
//   - ErrorMessage：失败原因，成功时为空。
//   - Attempts：被 worker 抢占次数。
//   - NextAttemptAt：最早可再次被抢占的时间（可空）。webhook_deliver 等任务
//     失败重试时写入退避时间点（1m/5m），Claim 只取已到期的 queued 行。
//   - ProjectID：可空，实例级任务为 NULL。
//   - OwnerNodeID：可空。管理面任务盖接收该请求的节点 UUID，其它节点不得 SKIP LOCKED 抢走；
//     dynamic_pack 保持 NULL，任意注册了该 handler 的节点可抢。
//   - IdempotencyKey：可选 24 小时幂等键，避免重复解压与拆线。
//   - StartedAt / FinishedAt：进入 running / 终态的时间。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
type Job struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Type           string     `gorm:"type:text;not null;index" json:"type"`
	// Status 的 queued 抢占走 SQL extras 的 partial idx_jobs_claim，不在此建全表复合索引。
	Status         string     `gorm:"type:text;not null;default:queued" json:"status"`
	Payload        []byte     `gorm:"type:jsonb" json:"payload,omitempty"`
	Result         []byte     `gorm:"type:jsonb" json:"result,omitempty"`
	Progress       int        `gorm:"not null;default:0" json:"progress"`
	ErrorMessage   string     `gorm:"type:text" json:"error_message,omitempty"`
	Attempts       int        `gorm:"not null;default:0" json:"attempts"`
	NextAttemptAt  *time.Time `json:"next_attempt_at,omitempty"`
	ProjectID      *uuid.UUID `gorm:"type:uuid;index" json:"project_id,omitempty"`
	OwnerNodeID    *uuid.UUID `gorm:"type:uuid;index" json:"owner_node_id,omitempty"`
	// IdempotencyKey 查找走 extras 的 (project_id, idempotency_key, created_at) btree，非终身 UNIQUE。
	IdempotencyKey *string    `gorm:"type:text" json:"idempotency_key,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName 固定表名 jobs。
func (Job) TableName() string {
	return "jobs"
}

const (
	// JobTypeBundleUnpack 管理面拆包。
	JobTypeBundleUnpack = "bundle_unpack"
	// JobTypeDeltaGenerate 管理面单文件差量。
	JobTypeDeltaGenerate = "delta_generate"
	// JobTypeAutoDelta 发布后系统预热。
	JobTypeAutoDelta = "auto_delta"
	// JobTypeDynamicPack 客户端 fileset 动态打包（owner 为空）。
	JobTypeDynamicPack = "dynamic_pack"
	// JobTypeWebhookDeliver Publish webhook。
	JobTypeWebhookDeliver = "webhook_deliver"
)

// AdminJobTypes 仅管理平面开启的节点应注册的任务类型。
func AdminJobTypes() []string {
	return []string{JobTypeBundleUnpack, JobTypeDeltaGenerate, JobTypeAutoDelta, JobTypeWebhookDeliver}
}

// BeforeCreate 在插入前补 UUID，并把空状态写成 queued。
func (j *Job) BeforeCreate(_ *gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	if j.Status == "" {
		j.Status = JobStatusQueued
	}
	return nil
}
