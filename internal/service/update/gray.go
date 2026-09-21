package update

import (
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// grayDevice 携带一次灰度判定所需的设备身份。
//
//   - RawDevice：明文原始 device_id；空串 = 匿名。匿名在未转全量时永不命中。
//   - DeviceHash：HTTP 层按 service.HashDeviceID 语义计算一次的设备哈希。
//     raw 策略下白名单比较改用原样 RawDevice（allowlistKey）。
type grayDevice struct {
	DeviceHash string
	RawDevice  string
}

// grayHitFor 灰度命中判定（D1/D2：版本白名单，不是 HMAC）。
//
// 优先级（强制路径由调用方短路——select 的 mandatoryFor 与 diff 的强制判定
// 都先于本函数执行）：
//
//  1. 版本 GrayIsComplete（gray_completed_at 非空或 is_critical）→ 恒命中
//     （匿名同口径，等同旧 100%）；
//  2. 匿名（无 device_id）→ 未转全量恒不命中；
//  3. 设备标识 ∈ 版本级白名单 → 命中；
//  4. 否则该候选对该设备不可见。
//
// line 保留签名以兼容 SelectTarget / diff 调用点，判定不再使用线级覆盖。
func grayHitFor(cat *Catalog, cand *VersionState, _ *LineState, dev grayDevice) bool {
	if cand == nil {
		return false
	}
	if cand.Version.GrayIsComplete() {
		return true
	}
	if strings.TrimSpace(dev.RawDevice) == "" {
		return false
	}
	return cand.Allowlist.contains(allowlistKey(cat, dev))
}

// allowlistKey 返回白名单比较键：raw 策略下白名单存原样、比较用原样；
// 其余策略（hashed；none 策略下无条目，比较值恒空）使用 HMAC 后的
// DeviceHash（service.HashDeviceID 同函数同输出）。
func allowlistKey(cat *Catalog, dev grayDevice) string {
	if cat != nil && cat.Project.DeviceIDPolicy == model.DeviceIDPolicyRaw {
		return strings.TrimSpace(dev.RawDevice)
	}
	return dev.DeviceHash
}
