package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/config"
)

func TestParseArgs(t *testing.T) {
	args, err := parseArgs([]string{"admin", "add", "root", "-password", "secret12"})
	if err != nil {
		t.Fatal(err)
	}
	if args.configPath != "" || args.password != "secret12" {
		t.Fatalf("configPath=%q password=%q", args.configPath, args.password)
	}
	if want := []string{"admin", "add", "root"}; !reflect.DeepEqual(args.positional, want) {
		t.Fatalf("positional=%v", args.positional)
	}
}

func TestParseArgsClear2FA(t *testing.T) {
	args, err := parseArgs([]string{"admin", "clear-2fa", "root"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"admin", "clear-2fa", "root"}; !reflect.DeepEqual(args.positional, want) {
		t.Fatalf("positional=%v", args.positional)
	}
}

func TestParseArgsConfigFlag(t *testing.T) {
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"-config", "a.yaml", "server"}, "a.yaml"},
		{[]string{"server", "-config=a.yaml"}, "a.yaml"},
		{[]string{"-config", "configs", "server"}, "configs"},
	}
	for _, tc := range cases {
		args, err := parseArgs(tc.argv)
		if err != nil {
			t.Fatal(err)
		}
		if args.configPath != tc.want {
			t.Fatalf("argv=%v configPath=%q want %q", tc.argv, args.configPath, tc.want)
		}
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, argv := range [][]string{
		{"-config"},       // 缺值
		{"-password"},     // 缺值
		{"-unknown"},      // 未知标志
		{"server", "-pw"}, // 未知标志
	} {
		if _, err := parseArgs(argv); err == nil {
			t.Fatalf("argv=%v: want error", argv)
		}
	}
}

func TestCommandOf(t *testing.T) {
	for argv, want := range map[string]string{
		"server": "server",
		"admin":  "admin",
		"help":   "help",
	} {
		if got := commandOf([]string{argv}); got != want {
			t.Fatalf("commandOf(%q)=%q want %q", argv, got, want)
		}
	}
	// 未跟任何参数：默认启动 server。
	if got := commandOf(nil); got != "server" {
		t.Fatalf("commandOf(nil)=%q want server", got)
	}
}

func TestResolveConfigPath(t *testing.T) {
	t.Setenv("KIRIVERS_CONFIG", "env.yaml")
	if got := resolveConfigPath(""); got != "env.yaml" {
		t.Fatalf("env fallback=%q", got)
	}
	if got := resolveConfigPath("flag.yaml"); got != "flag.yaml" {
		t.Fatalf("flag wins=%q", got)
	}
	if got := resolveConfigPath("configs"); got != "configs" {
		t.Fatalf("directory path=%q", got)
	}
}

func TestUsageMentionsConfigDirectory(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	usage()
	_ = w.Close()
	os.Stderr = old
	raw, _ := io.ReadAll(r)
	_ = r.Close()
	s := string(raw)
	if !strings.Contains(s, "directory") || !strings.Contains(s, "admin.yaml") {
		t.Fatalf("usage must mention directory and plane files:\n%s", s)
	}
}

func TestLoadThreeFilesFromDirectory(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("config.yaml", "postgres:\n  dsn: cmd-dir\n")
	write("admin.yaml", "addr: \"127.0.0.1:18081\"\n")
	write("client.yaml", "addr: \":18080\"\n")

	cfg, err := config.Load(resolveConfigPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Postgres.DSN != "cmd-dir" {
		t.Fatalf("dsn=%q", cfg.System.Postgres.DSN)
	}
	if cfg.Admin.Addr != "127.0.0.1:18081" || cfg.Client.Addr != ":18080" {
		t.Fatalf("admin=%q client=%q", cfg.Admin.Addr, cfg.Client.Addr)
	}
}

func TestProcessGinMode(t *testing.T) {
	if got := processGinMode("release", "debug"); got != gin.DebugMode {
		t.Fatalf("either debug → debug, got %q", got)
	}
	if got := processGinMode("DEBUG", "release"); got != gin.DebugMode {
		t.Fatalf("admin debug → debug, got %q", got)
	}
	if got := processGinMode("release", "RELEASE"); got != gin.ReleaseMode {
		t.Fatalf("both release → release, got %q", got)
	}
}

func TestUsageMentionsAdminOnlySystemFile(t *testing.T) {
	var buf bytes.Buffer
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	usage()
	_ = w.Close()
	os.Stderr = old
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	if !strings.Contains(buf.String(), "only the system file") {
		t.Fatalf("admin LoadSystem hint missing:\n%s", buf.String())
	}
}
