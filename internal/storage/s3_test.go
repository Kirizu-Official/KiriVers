package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// fakeBackend 证明业务只依赖 Backend 接口即可完成 Range / Presign，无需绑 S3 SDK。
type fakeBackend struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{objects: map[string][]byte{}}
}

func (f *fakeBackend) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = b
	return nil
}

func (f *fakeBackend) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeBackend) Range(_ context.Context, key string, start, end int64) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	n, err := rangeLength(int64(len(b)), start, end)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b[start : start+n])), nil
}

func (f *fakeBackend) Head(_ context.Context, key string) (int64, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return 0, "", ErrNotFound
	}
	return int64(len(b)), "fake", nil
}

func (f *fakeBackend) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}

func (f *fakeBackend) PresignPut(_ context.Context, key string, _ time.Duration, _ string) (string, error) {
	return "https://example.invalid/put/" + key, nil
}

func (f *fakeBackend) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://example.invalid/get/" + key, nil
}

var _ Backend = (*fakeBackend)(nil)

func TestFakeBackendRangeAndPresign(t *testing.T) {
	ctx := t.Context()
	var b Backend = newFakeBackend()
	payload := []byte("abcdef")
	if err := b.Put(ctx, "k", bytes.NewReader(payload), int64(len(payload)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	rc, err := b.Range(ctx, "k", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bcd" {
		t.Fatalf("range=%q", got)
	}
	u, err := b.PresignGet(ctx, "k", time.Minute)
	if err != nil || u == "" {
		t.Fatalf("presign get %q %v", u, err)
	}
	u, err = b.PresignPut(ctx, "k", time.Minute, "text/plain")
	if err != nil || u == "" {
		t.Fatalf("presign put %q %v", u, err)
	}
}

type mockS3Client struct {
	objects   map[string][]byte
	lastRange string
}

func (m *mockS3Client) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	b, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	if m.objects == nil {
		m.objects = map[string][]byte{}
	}
	m.objects[aws.ToString(params.Key)] = b
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	m.lastRange = aws.ToString(params.Range)
	b, ok := m.objects[aws.ToString(params.Key)]
	if !ok {
		return nil, ErrNotFound
	}
	body := b
	if params.Range != nil {
		start, end, err := parseBytesRange(aws.ToString(params.Range), int64(len(b)))
		if err != nil {
			return nil, err
		}
		body = b[start : end+1]
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (m *mockS3Client) HeadObject(_ context.Context, params *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	b, ok := m.objects[aws.ToString(params.Key)]
	if !ok {
		return nil, ErrNotFound
	}
	n := int64(len(b))
	etag := `"mock"`
	return &s3.HeadObjectOutput{ContentLength: &n, ETag: &etag}, nil
}

func (m *mockS3Client) DeleteObject(_ context.Context, _ *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return &s3.DeleteObjectOutput{}, nil
}

type mockPresign struct {
	putURL, getURL string
	putCalls       int
	getCalls       int
}

func (m *mockPresign) PresignPutObject(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.PresignOptions)) (*v4Presigned, error) {
	m.putCalls++
	return &v4Presigned{URL: m.putURL}, nil
}

func (m *mockPresign) PresignGetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.PresignOptions)) (*v4Presigned, error) {
	m.getCalls++
	return &v4Presigned{URL: m.getURL}, nil
}

func TestS3MockRangeAndPresign(t *testing.T) {
	ctx := t.Context()
	cli := &mockS3Client{objects: map[string][]byte{}}
	pre := &mockPresign{putURL: "https://s3.example/put", getURL: "https://s3.example/get"}
	b := &S3{client: cli, presigner: pre, bucket: "bucket"}

	payload := []byte("hello world")
	if err := b.Put(ctx, "obj", bytes.NewReader(payload), int64(len(payload)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	rc, err := b.Range(ctx, "obj", 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("range body=%q", got)
	}
	if cli.lastRange != "bytes=0-4" {
		t.Fatalf("range header=%q", cli.lastRange)
	}

	u, err := b.PresignPut(ctx, "obj", time.Minute, "text/plain")
	if err != nil || u != pre.putURL || pre.putCalls != 1 {
		t.Fatalf("presign put url=%q calls=%d err=%v", u, pre.putCalls, err)
	}
	u, err = b.PresignGet(ctx, "obj", time.Minute)
	if err != nil || u != pre.getURL || pre.getCalls != 1 {
		t.Fatalf("presign get url=%q calls=%d err=%v", u, pre.getCalls, err)
	}

	size, etag, err := b.Head(ctx, "obj")
	if err != nil || size != int64(len(payload)) || etag != "mock" {
		t.Fatalf("head size=%d etag=%q err=%v", size, etag, err)
	}
}

func TestMapS3ErrNotFound(t *testing.T) {
	if !errors.Is(mapS3Err(&types.NotFound{}), ErrNotFound) {
		t.Fatal("types.NotFound")
	}
	if !errors.Is(mapS3Err(&types.NoSuchKey{}), ErrNotFound) {
		t.Fatal("types.NoSuchKey")
	}
	if !errors.Is(mapS3Err(&smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}), ErrNotFound) {
		t.Fatal("generic NoSuchKey")
	}
	if !errors.Is(mapS3Err(&smithy.GenericAPIError{Code: "NotFound", Message: "missing"}), ErrNotFound) {
		t.Fatal("generic NotFound")
	}
	orig := errors.New("boom")
	if !errors.Is(mapS3Err(orig), orig) {
		t.Fatal("unknown errors must pass through")
	}
}

func parseBytesRange(header string, size int64) (int64, int64, error) {
	var start, end int64
	if _, err := fmt.Sscanf(header, "bytes=%d-%d", &start, &end); err == nil {
		return start, end, nil
	}
	if _, err := fmt.Sscanf(header, "bytes=%d-", &start); err != nil {
		return 0, 0, err
	}
	return start, size - 1, nil
}
