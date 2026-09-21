// Package signature 提供原生 check / diff / integrity 响应关键字段的签名原语
// （docs/app-init.md §12.1）。载荷规范化与算法实现集中在此，供 check、后续
// integrity/diff 子任务以及 feed 适配器（仅复用密钥装载，不复用字节格式）调用。
//
// 载荷格式（§12.1）：字段以 "\n" 连接，空值用空串占位（缺字段与空字段靠位置区分）：
//
//	version_integer(十进制或空) \n version_semver \n root_hash \n package_url \n size(十进制) \n sha256
//
// 算法：Ed25519（首选，输出 base64 std）与 RSA-SHA256（PKCS1v15，输出 base64 std）。
package signature

import (
	"crypto"
	"crypto/ed25519"
	corand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// 签名算法常量（取值与 internal/model 的 SigningAlgo* 一致；pkg 不反向依赖 internal）。
const (
	AlgoEd25519   = "ed25519"
	AlgoRSASHA256 = "rsa-sha256"
)

var (
	// ErrUnsupportedAlgo 未知的签名算法。
	ErrUnsupportedAlgo = errors.New("unsupported signature algorithm")
	// ErrBadPrivateKey 私钥 PEM 无法解析或类型不符。
	ErrBadPrivateKey = errors.New("invalid private key pem")
	// ErrBadPublicKey 公钥 PEM 无法解析或类型不符。
	ErrBadPublicKey = errors.New("invalid public key pem")
	// ErrBadSignature 签名与载荷不匹配。
	ErrBadSignature = errors.New("signature mismatch")
)

// BuildCheckPayload 构造 check/integrity 签名载荷：双号、root_hash、package_url、size、sha256。
// versionInteger / size 传十进制字符串；缺省传空串占位。
func BuildCheckPayload(versionInteger, versionSemver, rootHash, packageURL, size, sha256Hex string) string {
	return strings.Join([]string{
		versionInteger,
		versionSemver,
		rootHash,
		packageURL,
		size,
		sha256Hex,
	}, "\n")
}

// SignPayload 用 PEM 私钥对 payload 签名，返回 base64（std 编码）签名。
//
//   - ed25519：PKCS#8 私钥 → ed25519.Sign；
//   - rsa-sha256：PKCS#8 或 PKCS#1 私钥 → RSA PKCS1v15 + SHA-256。
func SignPayload(algo, privateKeyPEM, payload string) (string, error) {
	switch algo {
	case AlgoEd25519:
		key, err := parseEd25519PrivateKey(privateKeyPEM)
		if err != nil {
			return "", err
		}
		sig := ed25519.Sign(key, []byte(payload))
		return base64.StdEncoding.EncodeToString(sig), nil
	case AlgoRSASHA256:
		key, err := parseRSAPrivateKey(privateKeyPEM)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256([]byte(payload))
		sig, err := rsa.SignPKCS1v15(corand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			return "", fmt.Errorf("rsa sign: %w", err)
		}
		return base64.StdEncoding.EncodeToString(sig), nil
	default:
		return "", ErrUnsupportedAlgo
	}
}

// VerifyPayload 用 PEM 公钥验证 base64 签名；验证失败返回 ErrBadSignature。
// 供测试与外部校验方使用（标准库 Verify 语义）。
func VerifyPayload(algo, publicKeyPEM, payload, sigBase64 string) error {
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBadSignature, err.Error())
	}
	switch algo {
	case AlgoEd25519:
		key, err := parseEd25519PublicKey(publicKeyPEM)
		if err != nil {
			return err
		}
		if !ed25519.Verify(key, []byte(payload), sig) {
			return ErrBadSignature
		}
		return nil
	case AlgoRSASHA256:
		key, err := parseRSAPublicKey(publicKeyPEM)
		if err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(payload))
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
			return fmt.Errorf("%w: %s", ErrBadSignature, err.Error())
		}
		return nil
	default:
		return ErrUnsupportedAlgo
	}
}

// parseEd25519PrivateKey 解析 PKCS#8 包装的 Ed25519 私钥。
func parseEd25519PrivateKey(pemStr string) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, ErrBadPrivateKey
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBadPrivateKey, err.Error())
	}
	ed, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: not an ed25519 key", ErrBadPrivateKey)
	}
	return ed, nil
}

// parseRSAPrivateKey 解析 PKCS#8 或 PKCS#1 包装的 RSA 私钥。
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, ErrBadPrivateKey
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("%w: not an rsa key", ErrBadPrivateKey)
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBadPrivateKey, err.Error())
	}
	return key, nil
}

// parseEd25519PublicKey 解析 PKIX 包装的 Ed25519 公钥。
func parseEd25519PublicKey(pemStr string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, ErrBadPublicKey
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBadPublicKey, err.Error())
	}
	ed, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: not an ed25519 key", ErrBadPublicKey)
	}
	return ed, nil
}

// parseRSAPublicKey 解析 PKIX 包装的 RSA 公钥。
func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, ErrBadPublicKey
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBadPublicKey, err.Error())
	}
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: not an rsa key", ErrBadPublicKey)
	}
	return rsaKey, nil
}
