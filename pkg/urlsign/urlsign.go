// Package urlsign 实现私有存储的短时签名下载 URL（docs/app-init.md §13.7）。
//
// 语义：签名在本服务下载 handler 层统一实现（LocalFS 与 S3 同构，不依赖
// S3 Presign）。URL 的路径与文件名保持稳定，签名只出现在 query：
//
//	{path}?exp={unix 秒}&sig={hex}
//	sig = hex(HMAC-SHA256(secret, "GET\n{path}\n{exp}"))
//
// 校验使用 hmac.Equal 恒定时间比较，过期（exp <= now）即拒绝。
// 未配置实例级密钥时进程启动生成临时随机密钥（重启即全部失效），
// 必须显式配置 KIRIVERS_URL_SIGNING_SECRET 才能获得跨重启稳定的 URL。
package urlsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 签名 query 参数名（§13.7：签名只在 query）。
const (
	// QueryExp 签名过期时间的 Unix 秒参数名。
	QueryExp = "exp"
	// QuerySig 签名 hex 参数名。
	QuerySig = "sig"
	// SignMethod 参与签名消息的 HTTP 方法（下载恒为 GET；HEAD 请求按 GET 校验）。
	SignMethod = "GET"
)

// 签名校验错误（HTTP 层统一映射为 403 FORBIDDEN）。
var (
	// ErrMissingSignature query 缺失 exp 或 sig 参数。
	ErrMissingSignature = errors.New("urlsign: missing exp or sig query parameter")
	// ErrBadExpFormat exp 不是合法整数。
	ErrBadExpFormat = errors.New("urlsign: exp is not a valid integer")
	// ErrExpired 签名已过期（exp <= now）。
	ErrExpired = errors.New("urlsign: signature expired")
	// ErrBadSignature 签名不匹配（恒定时间比较后失败）。
	ErrBadSignature = errors.New("urlsign: signature mismatch")
)

// Sign 计算下载 URL 签名：hex(HMAC-SHA256(secret, "GET\n{path}\n{exp}")）。
// secret 为空时签名退化为对空密钥的 HMAC——仍然确定、可校验，但调用方
// 应保证进程启动时已注入随机或配置的密钥。
func Sign(secret, path string, exp int64) string {
	return hex.EncodeToString(macParts(secret, path, exp))
}

// macParts 计算签名消息的 HMAC-SHA256 摘要。
// 消息格式固定为 "GET\n{path}\n{exp}"，分隔符换行避免字段拼接歧义。
func macParts(secret, path string, exp int64) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(SignMethod))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(path))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return mac.Sum(nil)
}

// Verify 校验签名：exp 必须是未来时间（严格大于 now），sig 必须与重算值
// 恒定时间相等。path 必须与签发时的路径逐字节一致（含既有 query 的顺序）。
func Verify(secret, path string, exp int64, sig string, now time.Time) error {
	if exp <= now.Unix() {
		return ErrExpired
	}
	want := macParts(secret, path, exp)
	got, err := hex.DecodeString(sig)
	if err != nil {
		return ErrBadSignature
	}
	if !hmac.Equal(want, got) {
		return ErrBadSignature
	}
	return nil
}

// Signer 持有实例级密钥与默认 TTL 的签名器。
//
// SignDownload 满足 internal/service/update.URLSigner 接口（结构化类型，
// 无导入依赖）：update 包只为私有项目（StorageVisibility=private）调用它；
// 公开项目路径原样返回，签名器不会被触达。
type Signer struct {
	secret []byte
	// defaultTTL 未指定项目级 TTL 时的默认有效期（§13.7 默认 1h）。
	defaultTTL time.Duration
	// now 可注入时钟（TTL 边界测试用）；nil 时用 time.Now。
	now func() time.Time
}

// NewSigner 构造签名器。defaultTTL <= 0 时回退 1 小时（§13.7 默认）。
func NewSigner(secret string, defaultTTL time.Duration) *Signer {
	if defaultTTL <= 0 {
		defaultTTL = time.Hour
	}
	return &Signer{secret: []byte(secret), defaultTTL: defaultTTL}
}

// SetClock 注入时钟（测试用）。
func (s *Signer) SetClock(now func() time.Time) { s.now = now }

func (s *Signer) currentTime() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

// DefaultTTL 返回实例默认 TTL。
func (s *Signer) DefaultTTL() time.Duration { return s.defaultTTL }

// SignDownload 对稳定路径 path 追加 ?exp=&sig= 并返回完整 URL。
// ttlSeconds <= 0 时使用实例默认 TTL。path 已含 query（如 integrity URL）
// 时以 & 追加；本包对 query 值做 URL 转义以防特殊字符破坏结构。
func (s *Signer) SignDownload(path string, ttlSeconds int) string {
	ttl := s.defaultTTL
	if ttlSeconds > 0 {
		ttl = time.Duration(ttlSeconds) * time.Second
	}
	exp := s.currentTime().Add(ttl).Unix()
	sig := Sign(string(s.secret), path, exp)
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + QueryExp + "=" + strconv.FormatInt(exp, 10) +
		"&" + QuerySig + "=" + url.QueryEscape(sig)
}

// VerifyRequest 校验下载请求：从 exp/sig 原始 query 值解析并验证签名与
// 有效期。校验的 path 必须是请求的 URL 路径（与签发路径一致）。
// 任何失败（缺失 / 格式 / 过期 / 不匹配）都返回错误，由 HTTP 层统一 403。
func (s *Signer) VerifyRequest(path, expRaw, sigRaw string) error {
	if strings.TrimSpace(expRaw) == "" || strings.TrimSpace(sigRaw) == "" {
		return ErrMissingSignature
	}
	exp, err := strconv.ParseInt(strings.TrimSpace(expRaw), 10, 64)
	if err != nil {
		return ErrBadExpFormat
	}
	sig, err := url.QueryUnescape(sigRaw)
	if err != nil {
		return ErrBadSignature
	}
	return Verify(string(s.secret), path, exp, sig, s.currentTime())
}
