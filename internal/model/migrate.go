// Package model 放置 GORM 实体。每个实体必须注释用途、字段含义与关系。
// 新增实体后加入 AutoMigrateModels，由 database.AutoMigrate 在启动时同步表结构。
package model

// AutoMigrateModels 返回需要 GORM AutoMigrate 的全部实体，顺序应满足外键依赖（被引用表在前）。
func AutoMigrateModels() []any {
	return []any{
		&Admin{},               // 后台账号（平台/项目），无外键依赖，必须先于后续业务表
		&ProjectMember{},       // 项目成员，软引用 admins/projects（无 FK）
		&GeoipDatabase{},       // 平台 GeoIP MMDB 元数据，无项目 FK
		&AdminTOTP{},           // 管理员 TOTP 密钥与周期失败计数，依赖 admins
		&AdminPasskey{},        // 管理员 Passkey 公钥凭证，依赖 admins
		&AdminRecoveryCode{},   // 管理员一次性恢复码哈希，依赖 admins
		&Project{},             // 项目隔离根；Job/Version 引用其 UUID
		&StoreListing{},        // 商店上架 listing（替代 store_protocols jsonb）
		&ProjectLanguage{},     // 项目语言列表，依赖 projects（无 FK）
		&ProjectSlugAlias{},    // 旧 slug alias，依赖 projects
		&ProjectToken{},        // 项目 Token
		&CIToken{},             // CI Token 空表（创建 HTTP 不在本任务）
		&Version{},             // 瘦版本表，供 compare_engine 锁定
		&VersionLine{},         // 瘦 Version Line，供 PACKAGE_TYPE_IMMUTABLE；version-lifecycle 加列
		&Announcement{},        // 项目公告（无 Version FK；按 VersionID UUID 绑定）
		&ProjectMedia{},        // Markdown 媒体（无 Version FK；UUID 公开 GET）
		&Channel{},             // 项目渠道（系统 alpha/beta/stable + 自定义）
		&PlatformMatrix{},      // 平台矩阵 (os,arch) 模板
		&InstallPolicyRule{},   // 项目/渠道 × 平台路径安装策略模板（无 FK；Nil channel_id = 项目作用域）
		&HwRev{},               // 硬件代号有序列表
		&Job{},                 // 异步任务空壳；ProjectID / OwnerNodeID 仍不加 FK，保持可空
		&Node{},                // 集群节点身份；无 FK
		&NodeArtifactSync{},    // 本机代拉进度；软引用 nodes/versions（无 FK）
		&Artifact{},            // 实体产物表，依赖 Project/Version/VersionLine
		&UploadSession{},       // 分片上传与 TUS 会话表，依赖 Project/Version/VersionLine
		&ManifestEntry{},       // 多文件 Manifest 清单条目表，依赖 Project/Version/VersionLine
		&TelemetryEvent{},      // 客户端遥测事件表，依赖 Project（§10.4 / 连续失败降级）
		&Client{},              // 项目客户端名册，依赖 Project（无 FK）
		&ClientDailyStats{},    // 名册按日聚合，依赖 Project（无 FK）
		&GrayAllowlist{},       // 版本级灰度白名单，依赖 Project/Version（无 line）
		&GrayRolloutSnapshot{}, // 灰度覆盖快照（只追加）
		&WebhookDelivery{},     // Publish webhook 投递记录，依赖 Project（C14-3/C14-4，§5.8）
		&AuditEvent{},          // 管理动作审计事件，软引用 Project（无外键；§11.2 / C15-3）
	}
}
