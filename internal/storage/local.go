package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// LocalFS 把对象键映射到本地目录树。键必须路径安全（禁止 ..、绝对路径、盘符）。
type LocalFS struct {
	root string
}

var _ Backend = (*LocalFS)(nil)

// NewLocalFS 以 root 为对象存储根目录；不存在则创建。
func NewLocalFS(root string) (*LocalFS, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("local storage root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir local storage root: %w", err)
	}
	return &LocalFS{root: abs}, nil
}

// Root 返回绝对根目录，供本机 temp / 副本路径使用。
func (l *LocalFS) Root() string {
	if l == nil {
		return ""
	}
	return l.root
}

// Put 以流式拷贝写入对象，避免把整个文件读入内存。
func (l *LocalFS) Put(_ context.Context, key string, r io.Reader, size int64, _ string) error {
	full, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("mkdir object dir: %w", err)
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}
	defer f.Close()
	src := r
	if size >= 0 {
		src = io.LimitReader(r, size)
	}
	if _, err := io.Copy(f, src); err != nil {
		return fmt.Errorf("write object: %w", err)
	}
	return nil
}

// Get 打开对象并返回 ReadCloser；调用方必须 Close。
func (l *LocalFS) Get(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, mapOpenErr(err)
	}
	return f, nil
}

// Range 用 ReadAt/SectionReader 读取闭区间 [start, end]，end < 0 表示直到 EOF。
func (l *LocalFS) Range(_ context.Context, key string, start, end int64) (io.ReadCloser, error) {
	full, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, mapOpenErr(err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	length, err := rangeLength(info.Size(), start, end)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &sectionCloser{SectionReader: io.NewSectionReader(f, start, length), closer: f}, nil
}

// Head 返回大小与弱 ETag（大小+修改时间），不读文件内容。
func (l *LocalFS) Head(_ context.Context, key string) (int64, string, error) {
	full, err := l.resolve(key)
	if err != nil {
		return 0, "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return 0, "", mapOpenErr(err)
	}
	etag := fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
	return info.Size(), etag, nil
}

// Delete 删除对象。不存在视为成功（与 S3 幂等语义对齐）。
func (l *LocalFS) Delete(_ context.Context, key string) error {
	full, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PresignPut 返回本进程可访问的 file:// URL；TTL 产品语义不在本任务实现。
func (l *LocalFS) PresignPut(_ context.Context, key string, _ time.Duration, _ string) (string, error) {
	return l.fileURL(key)
}

// PresignGet 返回本进程可 GET 的 file:// URL（直接读本地文件）。
func (l *LocalFS) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return l.fileURL(key)
}

func (l *LocalFS) fileURL(key string) (string, error) {
	full, err := l.resolve(key)
	if err != nil {
		return "", err
	}
	slash := filepath.ToSlash(full)
	if runtime.GOOS == "windows" && !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	u := url.URL{Scheme: "file", Path: slash}
	return u.String(), nil
}

// resolve 把对象键限制在 root 之下，拒绝路径穿越。
func (l *LocalFS) resolve(key string) (string, error) {
	rel, err := sanitizeKey(key)
	if err != nil {
		return "", err
	}
	full := filepath.Join(l.root, filepath.FromSlash(rel))
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", ErrInvalidKey
	}
	relToRoot, err := filepath.Rel(l.root, abs)
	if err != nil {
		return "", ErrInvalidKey
	}
	relToRoot = filepath.Clean(relToRoot)
	sep := string(os.PathSeparator)
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+sep) {
		return "", ErrInvalidKey
	}
	return abs, nil
}

// sanitizeKey 规范化对象键：反斜杠改 /，禁止 .. / 绝对路径 / 盘符 / NUL。
func sanitizeKey(key string) (string, error) {
	if key == "" || strings.ContainsRune(key, 0) {
		return "", ErrInvalidKey
	}
	norm := strings.ReplaceAll(key, "\\", "/")
	if strings.HasPrefix(norm, "/") || filepath.IsAbs(key) {
		return "", ErrInvalidKey
	}
	if len(norm) >= 2 && unicode.IsLetter(rune(norm[0])) && norm[1] == ':' {
		return "", ErrInvalidKey
	}
	parts := strings.Split(norm, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if p == "." || p == ".." {
			return "", ErrInvalidKey
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return "", ErrInvalidKey
	}
	return path.Join(out...), nil
}

func mapOpenErr(err error) error {
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	return err
}

// rangeLength 把闭区间 [start, end] 转成 SectionReader 需要的长度。
func rangeLength(size, start, end int64) (int64, error) {
	if start < 0 {
		return 0, ErrInvalidRange
	}
	if size == 0 {
		if start == 0 && (end < 0 || end == 0) {
			return 0, nil
		}
		return 0, ErrInvalidRange
	}
	if start >= size {
		return 0, ErrInvalidRange
	}
	last := size - 1
	if end < 0 {
		end = last
	}
	if end < start {
		return 0, ErrInvalidRange
	}
	if end > last {
		end = last
	}
	return end - start + 1, nil
}

type sectionCloser struct {
	*io.SectionReader
	closer io.Closer
}

func (s *sectionCloser) Close() error {
	if s.closer == nil {
		return nil
	}
	return s.closer.Close()
}
