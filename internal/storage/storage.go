// Package storage 抽象对象存储。业务层只依赖 Backend，不得直接使用文件系统或 S3 SDK。
package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

// ReadyProbeKey 是 /ready 用来 Head 的探针对象。启动时若不存在则 Put 写入。
const ReadyProbeKey = ".ready"

var (
	// ErrNotFound 表示对象不存在。
	ErrNotFound = errors.New("storage object not found")
	// ErrInvalidKey 表示对象键包含路径穿越或非法段。
	ErrInvalidKey = errors.New("invalid storage key")
	// ErrInvalidRange 表示字节范围不合法。
	ErrInvalidRange = errors.New("invalid byte range")
)

// Backend 是 LocalFS 与 S3 兼容实现的共同合同。
// Range 使用 HTTP 闭区间语义：[start, end]；end < 0 表示读到对象末尾。
// Presign 的 TTL/私有项目产品规则由后续任务补齐，本接口必须先存在以免再拆一套存储抽象。
type Backend interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Range(ctx context.Context, key string, start, end int64) (io.ReadCloser, error)
	Head(ctx context.Context, key string) (size int64, etag string, err error)
	Delete(ctx context.Context, key string) error
	PresignPut(ctx context.Context, key string, ttl time.Duration, contentType string) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// Options 是 Open 的输入，由 cmd 包 server 模式从进程配置映射而来（本包不读环境变量）。
type Options struct {
	Driver    string
	LocalRoot string
	S3        S3Settings
}

// S3Settings 兼容 MinIO / Cloudflare R2 / AWS S3。
type S3Settings struct {
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	UsePathStyle  bool
	PublicBaseURL string
}

// Open 按 driver 构造 Backend：local（默认）或 s3。
func Open(opts Options) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(opts.Driver)) {
	case "", "local":
		return NewLocalFS(opts.LocalRoot)
	case "s3":
		return NewS3(context.Background(), opts.S3)
	default:
		return nil, errors.New("unknown storage driver: " + opts.Driver)
	}
}

// EnsureProbe 若探针键不存在则写入一小段内容，供启动后 /ready 做 Head。
func EnsureProbe(ctx context.Context, b Backend) error {
	if b == nil {
		return errors.New("storage backend is nil")
	}
	_, _, err := b.Head(ctx, ReadyProbeKey)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	return b.Put(ctx, ReadyProbeKey, strings.NewReader("ok"), 2, "text/plain")
}
