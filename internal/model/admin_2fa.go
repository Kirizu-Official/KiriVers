package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AdminTOTP 是实例管理员的 TOTP 密钥与周期失败计数（每账号至多一行）。
//
// 用途：强制第二因素。未确认密钥不得签发完整管理会话。校验每次直连数据库，禁止写入缓存。
//
// 关系：1:1 归属 Admin（admin_id 即主键）。CLI clear-2fa 删除本行。
//
// 字段：
//   - AdminID：管理员 UUID，主键。
//   - Secret：已确认的 base32 密钥；未确认前为空。API/日志/缓存永不输出。
//   - PendingSecret：绑定期未确认密钥；确认成功后移入 Secret 并清空。
//   - FailCount / FailPeriod：当前 TOTP 时间步（unix/30）内的失败次数与周期索引。
//   - RecoveryFailCount / RecoveryFailPeriod：恢复码喷码防护，共用同一时间步。
//   - ConfirmedAt：首次确认 TOTP 的 UTC 时间；空表示尚未合格。
type AdminTOTP struct {
	AdminID            uuid.UUID  `gorm:"type:uuid;primaryKey" json:"-"`
	Secret             string     `gorm:"type:text" json:"-"`
	PendingSecret      string     `gorm:"type:text" json:"-"`
	FailCount          int        `gorm:"not null;default:0" json:"-"`
	FailPeriod         int64      `gorm:"not null;default:0" json:"-"`
	RecoveryFailCount  int        `gorm:"not null;default:0" json:"-"`
	RecoveryFailPeriod int64      `gorm:"not null;default:0" json:"-"`
	ConfirmedAt        *time.Time `json:"-"`
	CreatedAt          time.Time  `json:"-"`
	UpdatedAt          time.Time  `json:"-"`
}

// TableName 固定表名 admin_totp。
func (AdminTOTP) TableName() string {
	return "admin_totp"
}

// Confirmed 表示该账号已绑定可用 TOTP（合格 2FA）。
func (t *AdminTOTP) Confirmed() bool {
	return t != nil && t.Secret != "" && t.ConfirmedAt != nil
}

// AdminPasskey 是管理员的 WebAuthn / Passkey 凭证（每账号 0..N 把）。
//
// 用途：已确认 TOTP 之后的可选第二因素。只存公钥材料，私钥永不落库、不进缓存。
//
// 关系：N:1 归属 Admin。删除 Passkey 或 CLI clear-2fa 不影响已签发的完整会话。
//
// 字段：
//   - ID：UUID 主键，应用侧生成。
//   - AdminID：所属管理员。
//   - CredentialID：WebAuthn credential id，实例内唯一。
//   - PublicKey：COSE 公钥字节。
//   - SignCount：断言计数器，成功断言后更新。
//   - Name：用户可见显示名。
type AdminPasskey struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	AdminID      uuid.UUID `gorm:"type:uuid;not null;index" json:"admin_id"`
	CredentialID []byte    `gorm:"type:bytea;not null;uniqueIndex" json:"-"`
	PublicKey    []byte    `gorm:"type:bytea;not null" json:"-"`
	SignCount    uint32    `gorm:"not null;default:0" json:"-"`
	Name         string    `gorm:"type:text;not null" json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 固定表名 admin_passkeys。
func (AdminPasskey) TableName() string {
	return "admin_passkeys"
}

// BeforeCreate 补 UUID。
func (p *AdminPasskey) BeforeCreate(_ *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// AdminRecoveryCode 是 TOTP 确认后生成的一次性恢复码哈希。
//
// 用途：丢失验证器时作为第二因素。明文只在生成当次返回；入库仅存规范化后的 SHA-256 hex。
// 用后写入 UsedAt。重新生成会作废全部未用旧码。不能代替必须先绑定的 TOTP。
//
// 关系：N:1 归属 Admin。CLI clear-2fa 删除。
//
// 字段：
//   - ID：UUID 主键。
//   - AdminID：所属管理员。
//   - CodeHash：规范化恢复码的 SHA-256 hex（无连字符、大写）。
//   - UsedAt：使用时刻；空表示尚未使用。
type AdminRecoveryCode struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	AdminID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"admin_id"`
	CodeHash  string     `gorm:"type:text;not null;index" json:"-"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// TableName 固定表名 admin_recovery_codes。
func (AdminRecoveryCode) TableName() string {
	return "admin_recovery_codes"
}

// BeforeCreate 补 UUID。
func (c *AdminRecoveryCode) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
