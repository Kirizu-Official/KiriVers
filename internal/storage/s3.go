package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const defaultPresignTTL = time.Hour

// s3ObjectAPI 是 *s3.Client 的测试缝，便于假后端证明 Range 走 GetObject Range 头。
type s3ObjectAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// s3PresignAPI 是 *s3.PresignClient 的测试缝。
type s3PresignAPI interface {
	PresignPutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4Presigned, error)
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4Presigned, error)
}

// v4Presigned 只取预签名 URL，避免测试依赖 signer 内部字段布局。
type v4Presigned struct {
	URL string
}

type s3PresignAdapter struct {
	inner *s3.PresignClient
}

func (a s3PresignAdapter) PresignPutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4Presigned, error) {
	out, err := a.inner.PresignPutObject(ctx, params, optFns...)
	if err != nil {
		return nil, err
	}
	return &v4Presigned{URL: out.URL}, nil
}

func (a s3PresignAdapter) PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4Presigned, error) {
	out, err := a.inner.PresignGetObject(ctx, params, optFns...)
	if err != nil {
		return nil, err
	}
	return &v4Presigned{URL: out.URL}, nil
}

// S3 是 AWS SDK v2 的兼容实现，同一配置可对接 MinIO / R2 / AWS。
type S3 struct {
	client    s3ObjectAPI
	presigner s3PresignAPI
	bucket    string
}

var _ Backend = (*S3)(nil)

// NewS3 按 endpoint/region/bucket/密钥构造客户端。region 为空时用 us-east-1（SDK 与 MinIO 均需要 region）。
func NewS3(ctx context.Context, cfg S3Settings) (*S3, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("s3 bucket is empty")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}
	if cfg.AccessKey != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	return &S3{
		client:    client,
		presigner: s3PresignAdapter{inner: s3.NewPresignClient(client)},
		bucket:    cfg.Bucket,
	}, nil
}

// Put 上传对象。size >= 0 时写入 Content-Length，便于 S3 兼容端校验。
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	key, err := sanitizeKey(key)
	if err != nil {
		return err
	}
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	}
	if size >= 0 {
		in.ContentLength = aws.Int64(size)
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	_, err = s.client.PutObject(ctx, in)
	if err != nil {
		return fmt.Errorf("s3 put: %w", err)
	}
	return nil
}

// Get 下载整个对象。调用方必须 Close Body。
func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	key, err := sanitizeKey(key)
	if err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, mapS3Err(err)
	}
	return out.Body, nil
}

// Range 通过 GetObject 的 Range 头读取闭区间字节，与 HTTP/S3 bytes=start-end 一致。
func (s *S3) Range(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	key, err := sanitizeKey(key)
	if err != nil {
		return nil, err
	}
	if start < 0 {
		return nil, ErrInvalidRange
	}
	header := fmt.Sprintf("bytes=%d-", start)
	if end >= 0 {
		if end < start {
			return nil, ErrInvalidRange
		}
		header = fmt.Sprintf("bytes=%d-%d", start, end)
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Range:  aws.String(header),
	})
	if err != nil {
		return nil, mapS3Err(err)
	}
	return out.Body, nil
}

// Head 返回 Content-Length 与去掉引号的 ETag。
func (s *S3) Head(ctx context.Context, key string) (int64, string, error) {
	key, err := sanitizeKey(key)
	if err != nil {
		return 0, "", err
	}
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, "", mapS3Err(err)
	}
	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	etag := strings.Trim(aws.ToString(out.ETag), `"`)
	return size, etag, nil
}

// Delete 删除对象。S3 对缺失键视为成功。
func (s *S3) Delete(ctx context.Context, key string) error {
	key, err := sanitizeKey(key)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return mapS3Err(err)
	}
	return nil
}

// PresignPut 用 SDK 签发 PUT URL；ttl <= 0 时默认 1 小时（产品过期策略由后续任务收紧）。
func (s *S3) PresignPut(ctx context.Context, key string, ttl time.Duration, contentType string) (string, error) {
	key, err := sanitizeKey(key)
	if err != nil {
		return "", err
	}
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := s.presigner.PresignPutObject(ctx, in, s3.WithPresignExpires(presignTTL(ttl)))
	if err != nil {
		return "", fmt.Errorf("s3 presign put: %w", err)
	}
	return out.URL, nil
}

// PresignGet 用 SDK 签发 GET URL。
func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	key, err := sanitizeKey(key)
	if err != nil {
		return "", err
	}
	out, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(presignTTL(ttl)))
	if err != nil {
		return "", fmt.Errorf("s3 presign get: %w", err)
	}
	return out.URL, nil
}

func presignTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultPresignTTL
	}
	return ttl
}

func mapS3Err(err error) error {
	if err == nil {
		return nil
	}
	var nsk *types.NoSuchKey
	var nf *types.NotFound
	if errors.As(err, &nsk) || errors.As(err, &nf) {
		return ErrNotFound
	}
	// MinIO / 部分兼容端 Head 缺失对象时只给 GenericAPIError，而不是 *types.NotFound。
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey":
			return ErrNotFound
		}
	}
	return err
}
