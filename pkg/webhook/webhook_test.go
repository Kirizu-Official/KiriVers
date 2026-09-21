package webhook

import (
	"testing"
)

// TestSignVerifyRoundtrip 正常路径：Sign 生成的头可被 Verify 校验通过。
func TestSignVerifyRoundtrip(t *testing.T) {
	secret := "kv_webhook_secret_0123456789abcdef"
	body := []byte(`{"event":"version.published","data":{"version_ref":"1.2.3"}}`)

	got := Sign(secret, body)
	if len(got) <= len(Prefix) {
		t.Fatalf("signature too short: %q", got)
	}
	if got[:len(Prefix)] != Prefix {
		t.Fatalf("signature missing %q prefix: %q", Prefix, got)
	}
	if !Verify(secret, body, got) {
		t.Fatalf("valid signature failed verification")
	}
	if err := VerifyErr(secret, body, got); err != nil {
		t.Fatalf("VerifyErr on valid signature: %v", err)
	}
}

// TestVerifyTamperedBody 篡改 body 后验签必须失败（验收 5）。
func TestVerifyTamperedBody(t *testing.T) {
	secret := "kv_webhook_secret_0123456789abcdef"
	body := []byte(`{"event":"version.published","data":{"version_ref":"1.2.3"}}`)

	sig := Sign(secret, body)
	if Verify(secret, []byte(`{"event":"version.published","data":{"version_ref":"9.9.9"}}`), sig) {
		t.Fatalf("tampered body must fail verification")
	}
	if err := VerifyErr(secret, []byte(`{"event":"version.revoked"}`), sig); err == nil {
		t.Fatalf("VerifyErr must return error on tampered body")
	}
}

// TestVerifyRejectsBadHeader 缺前缀、错误 secret、空头均拒绝。
func TestVerifyRejectsBadHeader(t *testing.T) {
	secret := "secret-a"
	body := []byte(`payload`)
	sig := Sign(secret, body)

	cases := []struct {
		name   string
		secret string
		header string
	}{
		{"wrong secret", "secret-b", sig},
		{"missing prefix", secret, "md5=deadbeef"},
		{"empty header", secret, ""},
		{"truncated hex", secret, Prefix + sig[len(Prefix):len(Prefix)+8]},
	}
	for _, tc := range cases {
		if Verify(tc.secret, body, tc.header) {
			t.Fatalf("%s: verification must fail", tc.name)
		}
	}
}
