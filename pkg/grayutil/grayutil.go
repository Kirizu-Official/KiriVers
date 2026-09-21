// Package grayutil 提供灰度发布的一致性哈希纯函数（docs/app-init.md §5.4、§13.11）。
//
// 设计要点：
//   - 以「版本级 GraySalt」为 HMAC 密钥、device_id 为消息，保证同一设备在同一版本上
//     的灰度判定恒定（salt 永不轮换），且不同版本之间判定互相独立。
//   - 原始 device_id 不落库、不进日志：本包只做内存内计算，不产生任何 IO 与日志输出。
//   - telemetry 子任务后续可复用 HMAC 原语实现 HMAC(project_device_secret, raw_id)。
package grayutil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// bucketsTotal 是桶空间大小：百分比按 0.01% 精度映射到 [0, 10000)。
const bucketsTotal = uint32(10000)

// Bucket 返回 device_id 在版本级 salt 下的一致性桶，范围 [0, 10000)。
//
// 计算：HMAC-SHA256(key=salt, msg=device_id) 取前 4 字节按大端序转 uint32 后对 10000 取模。
// saltHex 非法十六进制时按原始字节作为密钥，保证判定仍然确定（不 panic、不随机）。
func Bucket(saltHex, deviceID string) uint32 {
	key, err := hex.DecodeString(saltHex)
	if err != nil {
		key = []byte(saltHex)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(deviceID))
	sum := mac.Sum(nil)
	return binary.BigEndian.Uint32(sum[:4]) % bucketsTotal
}

// Hit 判定 device_id 是否命中灰度放量。
//
// 规则（§5.4）：
//   - percent >= 100：恒命中（匿名与关键版本同口径，读路径不区分）；
//   - percent <= 0：恒不命中；
//   - 其余：device_id 为空（匿名）视为未命中；否则桶值落入 [0, percent*100) 即命中。
func Hit(saltHex, deviceID string, percent int) bool {
	switch {
	case percent >= 100:
		return true
	case percent <= 0:
		return false
	case deviceID == "":
		return false
	}
	threshold := uint32(percent) * (bucketsTotal / 100)
	return Bucket(saltHex, deviceID) < threshold
}
