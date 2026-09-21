package signature

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

// mustEd25519PEM 生成测试用 Ed25519 PKCS#8 私钥与 PKIX 公钥 PEM。
func mustEd25519PEM(t *testing.T) (priv, pub string) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		t.Fatal(err)
	}
	priv = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	pubDER, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		t.Fatal(err)
	}
	pub = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return priv, pub
}

// mustRSAPEM 生成测试用 RSA PKCS#1 私钥与 PKIX 公钥 PEM。
func mustRSAPEM(t *testing.T) (priv, pub string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	priv = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pub = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return priv, pub
}

// TestPayloadFormat 锁定载荷的换行拼接与空值占位语义。
func TestPayloadFormat(t *testing.T) {
	got := BuildCheckPayload("102", "1.2.3", "roothash", "/pkg", "123456", "abcd")
	want := "102\n1.2.3\nroothash\n/pkg\n123456\nabcd"
	if got != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
	// 缺失字段用空串占位，位置仍可区分
	got = BuildCheckPayload("", "1.2.3", "", "/pkg", "", "abcd")
	want = "\n1.2.3\n\n/pkg\n\nabcd"
	if got != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}

// TestSignVerifyEd25519 Ed25519 签名可被标准库公钥验证；篡改载荷后验证失败。
func TestSignVerifyEd25519(t *testing.T) {
	priv, pub := mustEd25519PEM(t)
	payload := BuildCheckPayload("102", "1.2.3", "root", "/u", "1", "aa")
	sig, err := SignPayload(AlgoEd25519, priv, payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(sig) == "" {
		t.Fatal("empty signature")
	}
	if err := VerifyPayload(AlgoEd25519, pub, payload, sig); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if err := VerifyPayload(AlgoEd25519, pub, payload+"x", sig); err == nil {
		t.Fatal("expected verify failure on tampered payload")
	}
}

// TestSignVerifyRSA RSA-SHA256 签名可被标准库公钥验证。
func TestSignVerifyRSA(t *testing.T) {
	priv, pub := mustRSAPEM(t)
	payload := BuildCheckPayload("7", "", "root", "/u", "9", "bb")
	sig, err := SignPayload(AlgoRSASHA256, priv, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPayload(AlgoRSASHA256, pub, payload, sig); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if err := VerifyPayload(AlgoRSASHA256, pub, payload+"\n1", sig); err == nil {
		t.Fatal("expected verify failure on tampered payload")
	}
}

// TestBadKeys 非法 PEM 与未知算法必须报错而不是 panic。
func TestBadKeys(t *testing.T) {
	if _, err := SignPayload(AlgoEd25519, "not a pem", "p"); err == nil {
		t.Fatal("expected error for bad pem")
	}
	if _, err := SignPayload("unknown-algo", "x", "p"); err == nil {
		t.Fatal("expected error for unknown algo")
	}
	_, rsaPub := mustRSAPEM(t)
	if err := VerifyPayload(AlgoRSASHA256, rsaPub, "p", "!!!not-base64!!!"); err == nil {
		t.Fatal("expected error for bad base64")
	}
}
