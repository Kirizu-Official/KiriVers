package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

// 遥测域阈值常量（docs/app-init.md §10.4 / §13.11 / §15.2）。
const (
	// TelemetryDowngradeWindow 连续失败降级的统计窗口：24 小时。
	TelemetryDowngradeWindow = 24 * time.Hour
	// TelemetryDowngradeFailedThreshold 窗口内 failed 事件达到该数量即触发强制全量。
	TelemetryDowngradeFailedThreshold = 3
	// TelemetryRetentionSampleN 惰性留存清理的采样分母：约 1/50 次插入触发一次
	// DELETE（无状态、免定时器；高峰下清理滞后可接受，设计 §7 trade-off）。
	TelemetryRetentionSampleN = 50
	// DeviceHashFingerprintLen 日志中允许出现的设备指纹长度（哈希前 8 字符）。
	DeviceHashFingerprintLen = 8
)

// ErrInvalidTelemetryReport 遥测上报参数非法（缺必填字段 / status 枚举外）
// → HTTP 400。
var ErrInvalidTelemetryReport = errors.New("invalid telemetry report")

// ReportInput 是 POST telemetry/report 的请求字段（§10.4）。
//
// device_id 可选（缺失按空哈希接受）；os/arch/channel/from_version/to_version/status
// 必填；error_code/error_message/diff_mode 可选。
type ReportInput struct {
	DeviceID     string
	OS           string
	Arch         string
	Channel      string
	FromVersion  string
	ToVersion    string
	Status       string
	ErrorCode    string
	ErrorMessage string
	DiffMode     string
}

// TelemetryService 承载遥测上报、设备哈希、留存清理与连续失败降级读模型。
//
// 隐私边界（C11-1/C11-2）：本服务是唯一允许接触原始 device_id 的位置；
// 原始值只做内存内 HMAC 或原样落库（raw 策略），永不写入日志——需要日志
// 关联时只允许使用 Fingerprint（哈希前 8 字符）。
type TelemetryService struct {
	store repository.TelemetryStore
	// allowlistDeleter 是可选的灰度白名单按设备删除源（§11.2 / C15-4）：
	// 注入后按哈希隐私删除同时清除 Version 级与 per-line 白名单行；nil 时
	// 只删遥测（测试装配省略）。生产装配 repository.ProjectRepo（DeleteAllowlistByDeviceID）。
	allowlistDeleter AllowlistDeviceDeleter
	cache            cache.Store
	cacheLog         zerolog.Logger
	// now 可注入的时钟（测试窗口边界用）；nil 时用 time.Now。
	now func() time.Time
	// retentionHit 控制惰性留存清理是否触发；nil 时按 1/50 随机采样。
	retentionHit func() bool
}

// AllowlistDeviceDeleter 按设备标识删除项目内全部灰度白名单行
// （隐私删除 §11.2 / C15-4）。deviceID 为存储形态（hashed 下 = HMAC hex，
// 与遥测 DeviceHash 同函数；raw 下 = 原样），与删除请求中的 device_hash 同值。
type AllowlistDeviceDeleter interface {
	DeleteAllowlistByDeviceID(ctx context.Context, projectID uuid.UUID, deviceID string) (int64, error)
}

// NewTelemetryService 构造遥测服务。
func NewTelemetryService(store repository.TelemetryStore) *TelemetryService {
	return &TelemetryService{store: store}
}

// SetAllowlistDeleter 注入灰度白名单按设备删除源（隐私删除补全，C15-4）。
func (s *TelemetryService) SetAllowlistDeleter(d AllowlistDeviceDeleter) {
	s.allowlistDeleter = d
}

// SetCache 注入目录缓存，供隐私删除白名单后失效（deleter 绕过 ProjectService）。
func (s *TelemetryService) SetCache(store cache.Store) {
	s.cache = store
}

// SetCacheLogger 注入 mod=cache 子 logger。
func (s *TelemetryService) SetCacheLogger(log zerolog.Logger) {
	s.cacheLog = log
}

// SetClock 注入时钟（测试用）。
func (s *TelemetryService) SetClock(now func() time.Time) { s.now = now }

// SetRetentionSampler 注入留存清理触发器（测试用）；生产为 1/50 随机采样。
func (s *TelemetryService) SetRetentionSampler(fn func() bool) { s.retentionHit = fn }

func (s *TelemetryService) currentTime() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

// HashDeviceID 按项目 DeviceSecret 计算 hashed 策略设备哈希：
// hex(HMAC-SHA256(key=DeviceSecret, msg=raw))。供遥测、名册与灰度白名单共用。
func HashDeviceID(project *model.Project, rawDeviceID string) string {
	key, err := hex.DecodeString(project.DeviceSecret)
	if err != nil {
		key = []byte(project.DeviceSecret)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(rawDeviceID))
	return hex.EncodeToString(mac.Sum(nil))
}

// HashDeviceID（方法形态）委托包级 HashDeviceID，保持既有调用点不变。
func (s *TelemetryService) HashDeviceID(project *model.Project, rawDeviceID string) string {
	return HashDeviceID(project, rawDeviceID)
}

// Fingerprint 返回设备哈希的前 8 字符。任何日志输出只允许使用本指纹，
// 严禁输出原始 device_id 或完整哈希（C11-2，由 log buffer 测试锁定）。
func Fingerprint(deviceHash string) string {
	if len(deviceHash) > DeviceHashFingerprintLen {
		return deviceHash[:DeviceHashFingerprintLen]
	}
	return deviceHash
}

// resolveDeviceHash 按项目 DeviceIDPolicy 决定落库的 DeviceHash：
// hashed = HMAC；raw = 明文原样（文档警告，不推荐）；none = 空串
// （同时意味着不参与任何基于设备的降级判定，C11-7）。
func (s *TelemetryService) resolveDeviceHash(project *model.Project, rawDeviceID string) string {
	rawDeviceID = trimDeviceID(rawDeviceID)
	switch project.DeviceIDPolicy {
	case model.DeviceIDPolicyRaw:
		return rawDeviceID
	case model.DeviceIDPolicyNone:
		return ""
	default: // hashed（默认）
		if rawDeviceID == "" {
			return ""
		}
		return s.HashDeviceID(project, rawDeviceID)
	}
}

// trimDeviceID 规范化原始 device_id：去首尾空白；中间内容原样保留
// （hashed 策略下它只作为 HMAC 消息，不落库）。
func trimDeviceID(raw string) string {
	return strings.TrimSpace(raw)
}

// Report 校验并落库一条遥测事件。除参数非法（ErrInvalidTelemetryReport）与
// 存储故障外不产生任何错误语义——遥测只收不挡，更新协议不因它阻塞（C11-3）。
// 插入后按约 1/50 概率触发惰性留存清理（DELETE created_at < now-retention）。
func (s *TelemetryService) Report(ctx context.Context, project *model.Project, in ReportInput) error {
	if project == nil {
		return ErrProjectNotFound
	}
	// 必填字段（§10.4 / design §3）：os、arch、channel、from_version、to_version、status。
	if in.OS == "" || in.Arch == "" || in.Channel == "" || in.FromVersion == "" || in.ToVersion == "" {
		return fmt.Errorf("%w: os, arch, channel, from_version and to_version are required", ErrInvalidTelemetryReport)
	}
	if _, ok := model.ValidTelemetryStatuses[in.Status]; !ok {
		return fmt.Errorf("%w: unknown status %q", ErrInvalidTelemetryReport, in.Status)
	}

	now := s.currentTime()
	event := &model.TelemetryEvent{
		ProjectID:    project.ID,
		DeviceHash:   s.resolveDeviceHash(project, in.DeviceID),
		OS:           in.OS,
		Arch:         in.Arch,
		ChannelSlug:  in.Channel,
		FromVersion:  in.FromVersion,
		ToVersion:    in.ToVersion,
		Status:       in.Status,
		ErrorCode:    in.ErrorCode,
		ErrorMessage: in.ErrorMessage,
		DiffMode:     in.DiffMode,
		CreatedAt:    now,
	}
	if err := s.store.Insert(ctx, event); err != nil {
		return err
	}
	s.maybeCleanupRetention(ctx, project, now)
	return nil
}

// maybeCleanupRetention 惰性留存清理：按采样命中执行一次按项目 DELETE。
// 清理失败只吞掉——留存是尽力而为的后台语义，绝不阻塞上报（C11-4）。
func (s *TelemetryService) maybeCleanupRetention(ctx context.Context, project *model.Project, now time.Time) {
	if s.retentionHit != nil {
		if !s.retentionHit() {
			return
		}
	} else if !retentionSampleHit() {
		return
	}
	days := project.TelemetryRetentionDays
	if days <= 0 {
		days = model.DefaultTelemetryRetentionDays
	}
	cutoff := now.AddDate(0, 0, -days)
	_, _ = s.store.DeleteOlderThan(ctx, project.ID, cutoff)
}

// retentionSampleHit 生产采样：约 1/50 的插入触发清理。
func retentionSampleHit() bool {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return false
	}
	// 取模避免浮点；50 非整除 256，偏差可忽略（采样语义）。
	return b[0]%TelemetryRetentionSampleN == 0
}

// DowngradeActive 判定「连续失败降级」读模型（C11-8 / §15.2）：
// 同一 (project, os, arch, deviceHash) 在 24h 窗口内 failed ≥ 3，
// 且（无 installed 事件，或最近一次 installed 早于第 3 新的 failed）→ true。
// 键不含 channel（设备可能已跨渠道）。deviceHash 为空（匿名 / none 策略）
// 恒 false——none 策略不做基于设备的遥测降级（C11-7）。
//
// 实现为一条索引查询（idx_telemetry_downgrade）后内存判定，无状态、无物化表。
func (s *TelemetryService) DowngradeActive(ctx context.Context, projectID uuid.UUID, os, arch, deviceHash string) (bool, error) {
	if deviceHash == "" {
		return false, nil
	}
	since := s.currentTime().Add(-TelemetryDowngradeWindow)
	events, err := s.store.ListWindow(ctx, projectID, os, arch, deviceHash, since)
	if err != nil {
		return false, err
	}
	// 事件已按 created_at 倒序（新→旧；同一时刻按插入先后倒序）。判定规则：
	// 从新到旧扫描，failed 计数先于任何 installed 达到 3 → 降级生效；
	// 一旦在 failed 计满前遇到 installed（即最近一次 installed 比第 3 新的
	// failed 更新）→ 已恢复。
	failedSeen := 0
	for _, e := range events {
		switch e.Status {
		case model.TelemetryStatusFailed:
			failedSeen++
			if failedSeen >= TelemetryDowngradeFailedThreshold {
				return true, nil
			}
		case model.TelemetryStatusInstalled:
			if failedSeen < TelemetryDowngradeFailedThreshold {
				return false, nil
			}
		}
	}
	return false, nil
}

// DeleteDeviceEvents 按哈希删除某设备在本项目内的全部遥测事件（隐私删除
// §13.11）。未知哈希是幂等成功（0 条）。返回删除条数供审计日志使用。
func (s *TelemetryService) DeleteDeviceEvents(ctx context.Context, projectID uuid.UUID, deviceHash string) (int64, error) {
	return s.store.DeleteByDeviceHash(ctx, projectID, deviceHash)
}

// DeleteDeviceData 隐私删除补全（§11.2 / C15-4）：同一删除请求内同时清除
// 该设备在本项目内的遥测事件与灰度白名单行（Version 级 + per-line，
// 单条 WHERE 按 device_id 同时覆盖两个层级）。两个存储抽象相互独立，
// 以「先白名单、后遥测」的顺序执行；两步均幂等，重试安全（design §5）。
// 返回各自删除条数供响应体与审计使用；未知哈希幂等成功（0, 0）。
func (s *TelemetryService) DeleteDeviceData(ctx context.Context, projectID uuid.UUID, deviceHash string) (telemetryDeleted, allowlistDeleted int64, err error) {
	if s.allowlistDeleter != nil {
		n, derr := s.allowlistDeleter.DeleteAllowlistByDeviceID(ctx, projectID, deviceHash)
		if derr != nil {
			return 0, 0, derr
		}
		allowlistDeleted = n
		if s.cache != nil {
			if err := cache.InvalidateProject(ctx, s.cache, projectID); err != nil {
				s.cacheLog.Error().Err(err).Str("project_id", projectID.String()).Msg("invalidate project cache")
			}
		}
	}
	telemetryDeleted, err = s.store.DeleteByDeviceHash(ctx, projectID, deviceHash)
	if err != nil {
		return 0, allowlistDeleted, err
	}
	return telemetryDeleted, allowlistDeleted, nil
}
