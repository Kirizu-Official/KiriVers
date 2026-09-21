package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/config"
)

func streamCtx(t *testing.T) zerolog.Context {
	t.Helper()
	return zerolog.New(io.Discard).With().
		Str(FieldCat, CatSystem).
		Str(FieldPlane, PlaneAdmin).
		Str(FieldMod, ModDB)
}

func TestOpenInvalidLevelFallsBackToInfo(t *testing.T) {
	dir := t.TempDir()
	cfg := config.StreamConfig{
		Level:   "not-a-level",
		Console: config.ConsoleConfig{Enabled: false},
		File: config.FileConfig{
			Enabled:  true,
			Dir:      dir,
			Filename: "app.log",
		},
	}
	log, closer, err := Open(cfg, streamCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	log.Debug().Msg("debug-hidden")
	log.Info().Msg("info-visible")
	raw, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "debug-hidden") {
		t.Fatalf("debug must be dropped after invalid level fallback: %s", s)
	}
	if !strings.Contains(s, "info-visible") {
		t.Fatalf("info must be kept: %s", s)
	}
}

func TestOpenInjectsContextFieldsOnJSONFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.StreamConfig{
		Level:   "info",
		Console: config.ConsoleConfig{Enabled: false},
		File: config.FileConfig{
			Enabled:  true,
			Dir:      dir,
			Filename: "mod.log",
		},
	}
	log, closer, err := Open(cfg, streamCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	log.Info().Msg("hello")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "mod.log"))
	if err != nil {
		t.Fatal(err)
	}
	line := bytes.TrimSpace(raw)
	var evt map[string]any
	if err := json.Unmarshal(line, &evt); err != nil {
		t.Fatalf("file must be JSON: %v raw=%s", err, line)
	}
	if evt[FieldCat] != CatSystem || evt[FieldMod] != ModDB || evt[FieldPlane] != PlaneAdmin {
		t.Fatalf("fields=%v", evt)
	}
	if evt["message"] != "hello" {
		t.Fatalf("message=%v", evt["message"])
	}
}

func TestOpenFileOnlyAndConsoleOnly(t *testing.T) {
	dir := t.TempDir()
	fileCfg := config.StreamConfig{
		Level:   "info",
		Console: config.ConsoleConfig{Enabled: false},
		File:    config.FileConfig{Enabled: true, Dir: dir, Filename: "only.log"},
	}
	log, closer, err := Open(fileCfg, streamCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	log.Info().Msg("file-only")
	_ = closer.Close()
	raw, err := os.ReadFile(filepath.Join(dir, "only.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("file-only")) {
		t.Fatalf("missing file-only line: %s", raw)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	consoleCfg := config.StreamConfig{
		Level:   "info",
		Console: config.ConsoleConfig{Enabled: true, NoColor: true},
		File:    config.FileConfig{Enabled: false},
	}
	clog, ccloser, err := Open(consoleCfg, streamCtx(t))
	if err != nil {
		os.Stderr = old
		t.Fatal(err)
	}
	clog.Info().Msg("console-only")
	_ = ccloser.Close()
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	_ = r.Close()
	if !bytes.Contains(out, []byte("console-only")) {
		t.Fatalf("stderr missing console-only: %s", out)
	}
	if bytes.HasPrefix(bytes.TrimSpace(out), []byte("{")) {
		t.Fatalf("console sink must be ConsoleWriter, not raw JSON: %s", out)
	}
}

func TestOpenBothSinks(t *testing.T) {
	dir := t.TempDir()
	cfg := config.StreamConfig{
		Level:   "info",
		Console: config.ConsoleConfig{Enabled: true, NoColor: true},
		File:    config.FileConfig{Enabled: true, Dir: dir, Filename: "both.log"},
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	log, closer, err := Open(cfg, streamCtx(t))
	if err != nil {
		os.Stderr = old
		t.Fatal(err)
	}
	log.Info().Msg("both-sinks")
	_ = closer.Close()
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	_ = r.Close()
	if !bytes.Contains(out, []byte("both-sinks")) {
		t.Fatalf("console missing: %s", out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "both.log"))
	if err != nil {
		t.Fatal(err)
	}
	var evt map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &evt); err != nil {
		t.Fatalf("file must stay JSON: %v raw=%s", err, raw)
	}
	if evt["message"] != "both-sinks" {
		t.Fatalf("file evt=%v", evt)
	}
}

func TestOpenEmptyLevelFallsBackToInfo(t *testing.T) {
	dir := t.TempDir()
	cfg := config.StreamConfig{
		Level:   "",
		Console: config.ConsoleConfig{Enabled: false},
		File:    config.FileConfig{Enabled: true, Dir: dir, Filename: "empty-level.log"},
	}
	log, closer, err := Open(cfg, streamCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	log.Debug().Msg("debug-hidden")
	log.Info().Msg("info-visible")
	raw, err := os.ReadFile(filepath.Join(dir, "empty-level.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "debug-hidden") {
		t.Fatalf("empty level must not become NoLevel: %s", s)
	}
	if !strings.Contains(s, "info-visible") {
		t.Fatalf("empty level must keep info: %s", s)
	}
}

func TestOpenMkdirFailure(t *testing.T) {
	notDir := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.StreamConfig{
		Console: config.ConsoleConfig{Enabled: false},
		File:    config.FileConfig{Enabled: true, Dir: notDir, Filename: "x.log"},
	}
	if _, _, err := Open(cfg, streamCtx(t)); err == nil {
		t.Fatal("mkdir on a file path must fail")
	}
}

func TestOpenBothSinksDisabled(t *testing.T) {
	_, _, err := Open(config.StreamConfig{}, streamCtx(t))
	if err == nil {
		t.Fatal("expected error when both sinks disabled")
	}
}

func TestOpenEmptyFilePathRejected(t *testing.T) {
	cfg := config.StreamConfig{
		Console: config.ConsoleConfig{Enabled: false},
		File:    config.FileConfig{Enabled: true, Dir: "", Filename: "x.log"},
	}
	if _, _, err := Open(cfg, streamCtx(t)); err == nil {
		t.Fatal("empty dir must fail")
	}
	cfg.File.Dir = t.TempDir()
	cfg.File.Filename = ""
	if _, _, err := Open(cfg, streamCtx(t)); err == nil {
		t.Fatal("empty filename must fail")
	}
	cfg.File.Dir = ""
	cfg.File.Filename = ""
	if _, closer, err := Open(cfg, streamCtx(t)); err == nil {
		if closer != nil {
			_ = closer.Close()
		}
		t.Fatal("empty path must fail and must not fall back to TempDir")
	}
}

func TestNewStdWriter(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, zerolog.InfoLevel).With().Str(FieldMod, ModGin).Logger()
	n, err := NewStdWriter(log).Write([]byte("  gin debug line  \n"))
	if err != nil || n == 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if !strings.Contains(buf.String(), "gin debug line") {
		t.Fatalf("got %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"mod":"gin"`) {
		t.Fatalf("mod missing: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"level":"info"`) {
		t.Fatalf("plain gin line must be info: %s", buf.String())
	}
}

func TestStdWriterMapsGinDebugToDebug(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, zerolog.DebugLevel).With().Str(FieldMod, ModGin).Logger()
	if _, err := NewStdWriter(log).Write([]byte("[GIN-debug] Listening and serving HTTP on :8080\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"level":"debug"`) {
		t.Fatalf("GIN-debug must be debug: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "[GIN-debug]") {
		t.Fatalf("message missing: %s", buf.String())
	}
}

func TestStdWriterDropsGinDebugAtInfo(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, zerolog.InfoLevel).With().Str(FieldMod, ModGin).Logger()
	if _, err := NewStdWriter(log).Write([]byte("[GIN-debug] GET /hidden\n")); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("info logger must drop [GIN-debug]: %s", buf.String())
	}
}

func TestErrorWriterIsError(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, zerolog.InfoLevel).With().Str(FieldMod, ModGin).Logger()
	if _, err := NewErrorWriter(log).Write([]byte("listen tcp :8080: bind: address already in use\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"level":"error"`) {
		t.Fatalf("error writer must be error: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "bind: address already in use") {
		t.Fatalf("message missing: %s", buf.String())
	}
}
