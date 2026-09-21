package urlsign

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestSignVerifyRoundTrip 签发后立即可校验，路径或签名任一被篡改即拒绝。
func TestSignVerifyRoundTrip(t *testing.T) {
	const secret = "unit-test-secret"
	path := "/api/v1/projects/demo/packages/app-1.2.3.zip"
	exp := int64(2000000000)
	sig := Sign(secret, path, exp)
	if len(sig) != 64 { // SHA-256 hex
		t.Fatalf("sig length = %d, want 64", len(sig))
	}
	if err := Verify(secret, path, exp, sig, time.Unix(exp-1, 0)); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	// 篡改路径 → 不匹配。
	if err := Verify(secret, path+"/x", exp, sig, time.Unix(exp-1, 0)); err == nil {
		t.Fatal("tampered path must fail")
	}
	// 篡改签名 → 不匹配。
	if err := Verify(secret, path, exp, strings.Repeat("0", 64), time.Unix(exp-1, 0)); err == nil {
		t.Fatal("tampered sig must fail")
	}
	// 密钥不同 → 不匹配。
	if err := Verify("other", path, exp, sig, time.Unix(exp-1, 0)); err == nil {
		t.Fatal("wrong secret must fail")
	}
}

// TestVerifyExpiryBoundary TTL 边界：exp == now 视为已过期（严格大于才有效），
// exp == now+1 仍有效。
func TestVerifyExpiryBoundary(t *testing.T) {
	const secret = "boundary-secret"
	path := "/api/v1/projects/demo/packages/app.zip"
	now := time.Unix(1700000000, 0)
	exp := now.Unix()
	sig := Sign(secret, path, exp)
	if err := Verify(secret, path, exp, sig, now); err != ErrExpired {
		t.Fatalf("exp==now: got %v, want ErrExpired", err)
	}
	if err := Verify(secret, path, exp+1, Sign(secret, path, exp+1), now); err != nil {
		t.Fatalf("exp==now+1 must pass: %v", err)
	}
}

// TestVerifyMalformed 非法 hex 签名必须显式报错（HTTP 403）。
func TestVerifyMalformed(t *testing.T) {
	if err := Verify("s", "/p", 2000000000, "zz-not-hex", time.Now()); err == nil {
		t.Fatal("non-hex sig must fail")
	}
}

// queryValue 从签发 URL 的 query 部分解析指定参数（exp 为整数、sig 为 hex，
// 均为 URL 安全字符，无需反转义）。
func queryValue(signed, key string) string {
	for _, kv := range strings.Split(signed[strings.Index(signed, "?")+1:], "&") {
		if strings.HasPrefix(kv, key+"=") {
			return strings.TrimPrefix(kv, key+"=")
		}
	}
	return ""
}

// TestSignerDownloadRoundTrip Signer 端到端：SignDownload 产出带 exp/sig 的
// URL，VerifyRequest 立即通过；TTL 到期边界拒绝（验收 1 的 TTL 边界单测）。
func TestSignerDownloadRoundTrip(t *testing.T) {
	now := time.Unix(1700000000, 0)
	s := NewSigner("signer-secret", time.Hour)
	s.SetClock(func() time.Time { return now })

	path := "/api/v1/projects/demo/packages/app-1.2.3.zip"
	signed := s.SignDownload(path, 0)
	if !strings.HasPrefix(signed, path+"?exp=") || !strings.Contains(signed, "&sig=") {
		t.Fatalf("unexpected signed url: %s", signed)
	}
	expRaw := queryValue(signed, "exp")
	sigRaw := queryValue(signed, "sig")
	if err := s.VerifyRequest(path, expRaw, sigRaw); err != nil {
		t.Fatalf("fresh signature must verify: %v", err)
	}
	// 默认 TTL 1h：exp == now+3600。
	exp, _ := strconv.ParseInt(expRaw, 10, 64)
	if want := now.Unix() + 3600; exp != want {
		t.Fatalf("default TTL exp = %d, want %d", exp, want)
	}
	// TTL 边界：exp == now 再次校验 → 过期。
	s.SetClock(func() time.Time { return time.Unix(exp, 0) })
	if err := s.VerifyRequest(path, expRaw, sigRaw); err != ErrExpired {
		t.Fatalf("at exp: got %v, want ErrExpired", err)
	}
	// 过期前 1s 仍有效。
	s.SetClock(func() time.Time { return time.Unix(exp-1, 0) })
	if err := s.VerifyRequest(path, expRaw, sigRaw); err != nil {
		t.Fatalf("signature must verify 1s before expiry: %v", err)
	}
}

// TestSignerProjectTTLOverride 项目级 TTL 覆盖实例默认。
func TestSignerProjectTTLOverride(t *testing.T) {
	now := time.Unix(1700000000, 0)
	s := NewSigner("k", time.Hour)
	s.SetClock(func() time.Time { return now })
	signed := s.SignDownload("/p", 300)
	exp, _ := strconv.ParseInt(queryValue(signed, "exp"), 10, 64)
	if want := now.Unix() + 300; exp != want {
		t.Fatalf("project TTL exp = %d, want %d", exp, want)
	}
}

// TestSignerExistingQuery 已含 query 的路径以 & 追加签名参数。
func TestSignerExistingQuery(t *testing.T) {
	s := NewSigner("k", time.Hour)
	path := "/api/v1/projects/demo/versions/1/integrity?os=windows&arch=x86_64"
	signed := s.SignDownload(path, 0)
	if !strings.Contains(signed, "os=windows&arch=x86_64&exp=") {
		t.Fatalf("existing query must be preserved with & separator: %s", signed)
	}
}

// TestVerifyRequestMissingParams 缺失 exp/sig 与非法 exp → 明确错误（HTTP 403）。
func TestVerifyRequestMissingParams(t *testing.T) {
	s := NewSigner("k", time.Hour)
	if err := s.VerifyRequest("/p", "", "ab"); err != ErrMissingSignature {
		t.Fatalf("got %v, want ErrMissingSignature", err)
	}
	if err := s.VerifyRequest("/p", "123", ""); err != ErrMissingSignature {
		t.Fatalf("got %v, want ErrMissingSignature", err)
	}
	if err := s.VerifyRequest("/p", "not-a-number", "ab"); err != ErrBadExpFormat {
		t.Fatalf("got %v, want ErrBadExpFormat", err)
	}
}
