package storage

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLocalFSRoundTrip(t *testing.T) {
	ctx := t.Context()
	fs, err := NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello world")
	if err := fs.Put(ctx, "dir/obj.bin", bytes.NewReader(payload), int64(len(payload)), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}

	got, err := fs.Get(ctx, "dir/obj.bin")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(got)
	_ = got.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("get=%q", body)
	}

	rc, err := fs.Range(ctx, "dir/obj.bin", 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(chunk) != "hello" {
		t.Fatalf("range=%q", chunk)
	}

	mid, err := fs.Range(ctx, "dir/obj.bin", 6, -1)
	if err != nil {
		t.Fatal(err)
	}
	rest, err := io.ReadAll(mid)
	_ = mid.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "world" {
		t.Fatalf("range-eof=%q", rest)
	}

	size, etag, err := fs.Head(ctx, "dir/obj.bin")
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(payload)) || etag == "" {
		t.Fatalf("head size=%d etag=%q", size, etag)
	}

	putURL, err := fs.PresignPut(ctx, "dir/obj.bin", time.Minute, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	getURL, err := fs.PresignGet(ctx, "dir/obj.bin", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(putURL, "file:") || !strings.HasPrefix(getURL, "file:") {
		t.Fatalf("presign put=%q get=%q", putURL, getURL)
	}
	fromFile, err := os.ReadFile(fileURLPath(t, getURL))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromFile, payload) {
		t.Fatalf("presign get bytes=%q", fromFile)
	}

	if err := fs.Delete(ctx, "dir/obj.bin"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fs.Head(ctx, "dir/obj.bin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("head after delete: %v", err)
	}
}

func TestLocalFSRejectsTraversal(t *testing.T) {
	fs, err := NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../escape", "foo/../bar", "/abs", `C:\windows`, "foo/./bar"} {
		if _, err := fs.PresignGet(t.Context(), key, 0); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("key %q: %v", key, err)
		}
	}
}

func TestEnsureProbe(t *testing.T) {
	ctx := t.Context()
	fs, err := NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureProbe(ctx, fs); err != nil {
		t.Fatal(err)
	}
	size, _, err := fs.Head(ctx, ReadyProbeKey)
	if err != nil || size != 2 {
		t.Fatalf("probe head size=%d err=%v", size, err)
	}
	if err := EnsureProbe(ctx, fs); err != nil {
		t.Fatal(err)
	}
}

func TestOpenLocalFactory(t *testing.T) {
	b, err := Open(Options{Driver: "local", LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.(*LocalFS); !ok {
		t.Fatalf("got %T", b)
	}
}

func TestOpenUnknownDriver(t *testing.T) {
	if _, err := Open(Options{Driver: "ftp"}); err == nil {
		t.Fatal("expected error")
	}
}

func fileURLPath(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	p := u.Path
	if runtime.GOOS == "windows" && strings.HasPrefix(p, "/") && len(p) >= 3 && p[2] == ':' {
		p = strings.TrimPrefix(p, "/")
	}
	return filepath.FromSlash(p)
}
