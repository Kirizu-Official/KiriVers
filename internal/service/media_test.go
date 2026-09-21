package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

type memBackend struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemBackend() *memBackend {
	return &memBackend{objects: map[string][]byte{}}
}

func (f *memBackend) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = b
	return nil
}

func (f *memBackend) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *memBackend) Range(_ context.Context, key string, start, end int64) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	if start < 0 || start >= int64(len(b)) {
		return nil, storage.ErrInvalidRange
	}
	if end < 0 || end >= int64(len(b)) {
		end = int64(len(b)) - 1
	}
	return io.NopCloser(bytes.NewReader(b[start : end+1])), nil
}

func (f *memBackend) Head(_ context.Context, key string) (int64, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return 0, "", storage.ErrNotFound
	}
	return int64(len(b)), "mem", nil
}

func (f *memBackend) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}

func (f *memBackend) PresignPut(context.Context, string, time.Duration, string) (string, error) {
	return "", errors.New("unused")
}

func (f *memBackend) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("unused")
}

func testProject() *model.Project {
	return &model.Project{ID: uuid.Must(uuid.NewRandom()), Slug: "demo-app"}
}

func TestExpandSiteURLAndOriginFromReferer(t *testing.T) {
	t.Parallel()
	md := "see ${site_url}/api/v1/projects/demo-app/media/" + uuid.Nil.String()
	got := ExpandSiteURL(md, "https://app.example")
	if !strings.Contains(got, "https://app.example/api/v1/projects/") || strings.Contains(got, SiteURLPlaceholder) {
		t.Fatalf("expand=%q", got)
	}

	cases := []struct {
		referer, fallback, want string
	}{
		{"https://app.example/path?q=1", "http://api.local", "https://app.example"},
		{"https://app.example:8443/x", "http://api.local", "https://app.example:8443"},
		{"", "https://api.example:8080", "https://api.example:8080"},
		{"not a url", "http://fallback", "http://fallback"},
		{"ftp://files.example/a", "http://fallback", "http://fallback"},
		{"https://app.example/", "http://fallback", "https://app.example"},
	}
	for _, tc := range cases {
		if got := OriginFromReferer(tc.referer, tc.fallback); got != tc.want {
			t.Fatalf("referer=%q fallback=%q got=%q want=%q", tc.referer, tc.fallback, got, tc.want)
		}
	}

	vary := AppendVaryReferer([]string{"Accept-Encoding"})
	if strings.Join(vary, ",") != "Accept-Encoding,Referer" {
		t.Fatalf("vary=%v", vary)
	}
	if len(AppendVaryReferer(vary)) != 2 {
		t.Fatalf("duplicate Referer: %v", AppendVaryReferer(vary))
	}
}

func TestMediaPutRejectsMIMEAndSize(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewMediaService(repository.NewMemoryProjectMediaStore(), newMemBackend(), nil)
	p := testProject()

	_, err := svc.Put(ctx, p, MediaPutInput{FileName: "x.html", ContentType: "text/html", Body: strings.NewReader("<html></html>")})
	if !IsInvalidRequest(err) {
		t.Fatalf("html: %v", err)
	}
	_, err = svc.Put(ctx, p, MediaPutInput{FileName: "x.svg", ContentType: "image/svg+xml", Body: strings.NewReader("<svg/>")})
	if !IsInvalidRequest(err) {
		t.Fatalf("svg: %v", err)
	}
	_, err = svc.Put(ctx, p, MediaPutInput{FileName: "x.js", ContentType: "application/javascript", Body: strings.NewReader("alert(1)")})
	if !IsInvalidRequest(err) {
		t.Fatalf("js: %v", err)
	}
	_, err = svc.Put(ctx, p, MediaPutInput{FileName: "empty.bin", ContentType: "application/octet-stream", Body: strings.NewReader("")})
	if !IsInvalidRequest(err) {
		t.Fatalf("empty: %v", err)
	}
	_, err = svc.Put(ctx, p, MediaPutInput{
		FileName:    "big.bin",
		ContentType: "application/octet-stream",
		Body:        bytes.NewReader(bytes.Repeat([]byte("a"), int(model.MediaMaxBytes)+1)),
	})
	if !IsInvalidRequest(err) {
		t.Fatalf("size: %v", err)
	}
}

func TestMediaPutAndGetWithS3Replica(t *testing.T) {
	ctx := context.Background()
	primary := newMemBackend()
	replica, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(repository.NewMemoryProjectMediaStore(), primary, replica)
	p := testProject()
	payload := []byte("\x89PNG\r\n\x1a\n" + "not-really-png-but-fine")
	res, err := svc.Put(ctx, p, MediaPutInput{FileName: "shot.png", ContentType: "image/png", Body: bytes.NewReader(payload)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.URL, SiteURLPlaceholder+"/api/v1/projects/demo-app/media/") {
		t.Fatalf("url=%q", res.URL)
	}
	if _, _, err := replica.Head(ctx, replicaKey(res.Media)); err != nil {
		t.Fatalf("replica after put: %v", err)
	}

	meta, err := svc.GetMeta(ctx, p.ID, res.Media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Delete(ctx, replicaKey(meta)); err != nil {
		t.Fatal(err)
	}
	rc, err := svc.Open(ctx, meta, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, payload) {
		t.Fatalf("bytes mismatch")
	}
	if _, _, err := replica.Head(ctx, replicaKey(meta)); err != nil {
		t.Fatalf("replica after get miss: %v", err)
	}

	rc, err = svc.Open(ctx, meta, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	part, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(part, payload[:4]) {
		t.Fatalf("range=%q", part)
	}

	other := uuid.Must(uuid.NewRandom())
	if _, err := svc.GetMeta(ctx, other, res.Media.ID); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("wrong project: %v", err)
	}
}

func TestMediaPublicURLKeepsPlaceholder(t *testing.T) {
	id := uuid.Must(uuid.NewRandom())
	u := MediaPublicURL("my-slug", id)
	if !strings.HasPrefix(u, SiteURLPlaceholder) || strings.Contains(u, "http") {
		t.Fatalf("url=%q", u)
	}
}
