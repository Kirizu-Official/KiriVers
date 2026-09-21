package service

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

// ArchiveMember 是写入 zip 的一个文件体。SHA256 必须是 64 位小写 hex（哈希根目录成员名）。
type ArchiveMember struct {
	Path   string
	SHA256 string
	Body   []byte
}

// BuildHashRootZip 构造原生 full/patch：成员名为文件内容 SHA-256 hex（小写）。
// 相同哈希只写一次。KEEP 元数据文件不进入本归档。
func BuildHashRootZip(files []ArchiveMember) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	seen := make(map[string]struct{}, len(files))
	for _, f := range files {
		name := strings.ToLower(strings.TrimSpace(f.SHA256))
		if len(name) != 64 {
			_ = zw.Close()
			return nil, fmt.Errorf("hash-root member sha256 must be 64 hex, got %q for path %q", f.SHA256, f.Path)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("create hash-root member %s: %w", name, err)
		}
		if _, err := w.Write(f.Body); err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("write hash-root member %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalize hash-root zip: %w", err)
	}
	return buf.Bytes(), nil
}

// BuildPathZip 构造 store_full：成员名为 Manifest 路径（NFC + `/`）。
func BuildPathZip(files []ArchiveMember) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	seen := make(map[string]struct{}, len(files))
	for _, f := range files {
		name, err := pathutil.NormalizeAndValidatePath(f.Path)
		if err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("store_full path %q: %w", f.Path, err)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("create path zip member %s: %w", name, err)
		}
		if _, err := w.Write(f.Body); err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("write path zip member %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalize path zip: %w", err)
	}
	return buf.Bytes(), nil
}

// ExtractZipMember 按成员名取出字节；hash-root 成员名即 sha256 hex。
func ExtractZipMember(zipBytes []byte, memberName string) ([]byte, error) {
	want := strings.ReplaceAll(memberName, "\\", "/")
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name
		if norm, nerr := pathutil.NormalizeAndValidatePath(f.Name); nerr == nil {
			name = norm
		}
		if name == want || strings.EqualFold(strings.Trim(f.Name, "/"), want) {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open zip member %q: %w", f.Name, err)
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("read zip member %q: %w", f.Name, err)
			}
			return body, nil
		}
	}
	return nil, fmt.Errorf("zip missing member %q", memberName)
}

// ExtractHashRootMember 按文件 SHA-256 hex 取出哈希根目录归档中的成员。
func ExtractHashRootMember(zipBytes []byte, fileSHA256 string) ([]byte, error) {
	return ExtractZipMember(zipBytes, strings.ToLower(strings.TrimSpace(fileSHA256)))
}
