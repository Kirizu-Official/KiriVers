//go:build integration

package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"
)

// TestS3MinIORoundTrip 需要真实 MinIO。设置 KIRIVERS_S3_ENDPOINT 等环境变量后：
// go test -tags integration ./internal/storage
func TestS3MinIORoundTrip(t *testing.T) {
	endpoint := os.Getenv("KIRIVERS_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("KIRIVERS_S3_ENDPOINT not set")
	}
	ctx := context.Background()
	b, err := NewS3(ctx, S3Settings{
		Endpoint:     endpoint,
		Region:       envOr("KIRIVERS_S3_REGION", "us-east-1"),
		Bucket:       os.Getenv("KIRIVERS_S3_BUCKET"),
		AccessKey:    os.Getenv("KIRIVERS_S3_ACCESS_KEY"),
		SecretKey:    os.Getenv("KIRIVERS_S3_SECRET_KEY"),
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := "integration/range.bin"
	payload := []byte("hello world")
	if err := b.Put(ctx, key, bytes.NewReader(payload), int64(len(payload)), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Delete(ctx, key) })

	rc, err := b.Range(ctx, key, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("range=%q", got)
	}
	u, err := b.PresignGet(ctx, key, time.Minute)
	if err != nil || u == "" {
		t.Fatalf("presign get %q %v", u, err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
