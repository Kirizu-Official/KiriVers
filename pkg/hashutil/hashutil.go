// Package hashutil 提供 SHA-256 / MD5 / SHA-512 流式计算，避免把整个文件读入内存。
package hashutil

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
)

// SHA256Hex 从 r 流式计算 SHA-256，返回小写 hex。
func SHA256Hex(r io.Reader) (string, error) {
	return sumHex(sha256.New(), r)
}

// MD5Hex 从 r 流式计算 MD5。不得作为唯一安全依据，仅兼容旧客户端。
func MD5Hex(r io.Reader) (string, error) {
	return sumHex(md5.New(), r)
}

// SHA512Hex 从 r 流式计算 SHA-512，返回小写 hex。
func SHA512Hex(r io.Reader) (string, error) {
	return sumHex(sha512.New(), r)
}

func sumHex(h hash.Hash, r io.Reader) (string, error) {
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// MultiHasher 封装了同时计算多种哈希及大小的流式 Writer。
type MultiHasher struct {
	sha256     hash.Hash
	md5        hash.Hash
	sha512     hash.Hash
	withSHA512 bool
	size       int64
	w          io.Writer
}

// NewMultiHasher 创建一个多重哈希流式计算器。如果 withSHA512 为 true，同时计算 SHA-512。
func NewMultiHasher(withSHA512 bool) *MultiHasher {
	s256 := sha256.New()
	m := md5.New()
	writers := []io.Writer{s256, m}
	var s512 hash.Hash
	if withSHA512 {
		s512 = sha512.New()
		writers = append(writers, s512)
	}
	return &MultiHasher{
		sha256:     s256,
		md5:        m,
		sha512:     s512,
		withSHA512: withSHA512,
		w:          io.MultiWriter(writers...),
	}
}

// Write 实现 io.Writer，累计字节数并更新各哈希状态。
func (m *MultiHasher) Write(p []byte) (n int, err error) {
	n, err = m.w.Write(p)
	m.size += int64(n)
	return n, err
}

// Size 返回累计处理的字节数。
func (m *MultiHasher) Size() int64 {
	return m.size
}

// SHA256 返回当前累积内容的 SHA-256 小写 hex 字符串。
func (m *MultiHasher) SHA256() string {
	return hex.EncodeToString(m.sha256.Sum(nil))
}

// MD5 返回当前累积内容的 MD5 小写 hex 字符串。
func (m *MultiHasher) MD5() string {
	return hex.EncodeToString(m.md5.Sum(nil))
}

// SHA512 返回当前累积内容的 SHA-512 小写 hex 字符串。如果未启用 SHA-512 则返回空字符串。
func (m *MultiHasher) SHA512() string {
	if !m.withSHA512 || m.sha512 == nil {
		return ""
	}
	return hex.EncodeToString(m.sha512.Sum(nil))
}
