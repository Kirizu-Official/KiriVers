package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rs/zerolog"
)

func TestLogAdminStatic(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf)

	logAdminStatic(log, "", nil)
	if !strings.Contains(buf.String(), `"level":"warn"`) || !strings.Contains(buf.String(), "serving API only") {
		t.Fatalf("empty static_dir log=%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"static_dir":""`) {
		t.Fatalf("empty static_dir field missing: %s", buf.String())
	}

	dir := t.TempDir()
	buf.Reset()
	logAdminStatic(log, dir, nil)
	if !strings.Contains(buf.String(), `"level":"warn"`) || !strings.Contains(buf.String(), "index.html missing") {
		t.Fatalf("missing index log=%s", buf.String())
	}

	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	logAdminStatic(log, dir, nil)
	if !strings.Contains(buf.String(), `"level":"info"`) || !strings.Contains(buf.String(), "static files enabled") {
		t.Fatalf("present index log=%s", buf.String())
	}
	if strings.Contains(buf.String(), "<html") {
		t.Fatal("must not log file contents")
	}

	if err := os.Remove(filepath.Join(dir, "index.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "index.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	logAdminStatic(log, dir, nil)
	if !strings.Contains(buf.String(), `"level":"warn"`) || !strings.Contains(buf.String(), "index.html missing") {
		t.Fatalf("index.html directory log=%s", buf.String())
	}

	embedded := fstest.MapFS{"index.html": {Data: []byte("<html>embed</html>\n")}}
	buf.Reset()
	logAdminStatic(log, dir, embedded)
	if !strings.Contains(buf.String(), `"level":"info"`) || !strings.Contains(buf.String(), "embedded files enabled") {
		t.Fatalf("embed fallback log=%s", buf.String())
	}
	if strings.Contains(buf.String(), "embed</html>") {
		t.Fatal("must not log embedded file contents")
	}

	buf.Reset()
	logAdminStatic(log, "", embedded)
	if !strings.Contains(buf.String(), `"level":"warn"`) || !strings.Contains(buf.String(), "empty static_dir") {
		t.Fatalf("empty static_dir must disable embed: %s", buf.String())
	}
}
