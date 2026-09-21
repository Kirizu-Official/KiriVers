// Package webhook 提供 Publish webhook 的请求体签名与验签（C14-3，docs/app-init.md §5.8）。
//
// 签名格式（与投递端一致）：
//
//	X-KiriVers-Signature: sha256=<hex(HMAC-SHA256(secret, rawBody))>
//
// 服务端投递时用 Sign 生成请求头；外部消费方（或测试）用 Verify 校验原始请求体，
// 防止传输途中被篡改。secret 为项目 WebhookSecret（仅服务端持有，永不回显）。
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// HeaderName 是 webhook 签名请求头名称。
const HeaderName = "X-KiriVers-Signature"

// Prefix 是签名值的算法前缀；当前仅支持 HMAC-SHA256。
const Prefix = "sha256="

// ErrInvalidSignature 签名缺失、格式不合法或校验不通过。
var ErrInvalidSignature = errors.New("webhook signature invalid")

// Sign 计算 webhook 请求体的 HMAC-SHA256 签名，返回完整请求头值
// （形如 "sha256=<hex>"）。secret 为项目 WebhookSecret，body 为原始请求体字节。
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return Prefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify 校验签名头与原始请求体是否匹配。
//
// 使用 hmac.Equal 做常数时间比较，避免时序侧信道；
// 篡改过的 body、错误 secret 或格式不合法的头一律返回 false。
func Verify(secret string, body []byte, header string) bool {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, Prefix) {
		return false
	}
	want := Sign(secret, body)
	return hmac.Equal([]byte(want), []byte(header))
}

// VerifyErr 与 Verify 相同，但返回可读错误，便于消费方区分「头缺失/格式错」与「不匹配」。
func VerifyErr(secret string, body []byte, header string) error {
	if !Verify(secret, body, header) {
		return fmt.Errorf("%w: header %q mismatch", ErrInvalidSignature, HeaderName)
	}
	return nil
}
