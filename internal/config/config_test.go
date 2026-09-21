package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeThree(t *testing.T, dir, system, admin, client string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(system), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "admin.yaml"), []byte(admin), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "client.yaml"), []byte(client), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "config.yaml")
}

func TestDefaultAddrs(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "log:\n  level: info\n", "", "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Client.Addr != ":8080" || cfg.Admin.Addr != ":8081" {
		t.Fatalf("client=%q admin=%q", cfg.Client.Addr, cfg.Admin.Addr)
	}
}

func TestLoadFromDirectory(t *testing.T) {
	dir := t.TempDir()
	writeThree(t, dir,
		"postgres:\n  dsn: dir-dsn\n",
		"addr: \"127.0.0.1:8444\"\n",
		"addr: \":8443\"\n",
	)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Postgres.DSN != "dir-dsn" {
		t.Fatalf("dsn=%q", cfg.System.Postgres.DSN)
	}
	if cfg.Admin.Addr != "127.0.0.1:8444" || cfg.Client.Addr != ":8443" {
		t.Fatalf("admin=%q client=%q", cfg.Admin.Addr, cfg.Client.Addr)
	}
}

func TestPlaneTLSBothOrNeither(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "",
		"tls_cert: /etc/certs/admin.pem\ntls_key: /etc/certs/admin.key\n",
		"tls_cert: /etc/certs/client.pem\n",
	)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Admin.TLSEnabled() {
		t.Fatal("admin both cert+key should enable TLS")
	}
	if cfg.Client.TLSEnabled() {
		t.Fatal("client cert without key must stay plaintext")
	}
	if (PlaneConfig{TLSCert: "a"}).TLSEnabled() {
		t.Fatal("cert only")
	}
	if (PlaneConfig{TLSKey: "b"}).TLSEnabled() {
		t.Fatal("key only")
	}
	if (PlaneConfig{}).TLSEnabled() {
		t.Fatal("empty")
	}
}

func TestPrefixIsolation(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir,
		"log:\n  level: info\n",
		"addr: \":8081\"\n",
		"addr: \":8080\"\n",
	)
	t.Setenv("KIRIVERS_ADMIN_ADDR", "127.0.0.1:9443")
	t.Setenv("KIRIVERS_CLIENT_ADDR", ":9090")
	t.Setenv("KIRIVERS_LOG_LEVEL", "warn")
	t.Setenv("KIRIVERS_ADMIN_LOG_SYSTEM_LEVEL", "error")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Addr != "127.0.0.1:9443" {
		t.Fatalf("admin addr=%q", cfg.Admin.Addr)
	}
	if cfg.Client.Addr != ":9090" {
		t.Fatalf("client addr=%q", cfg.Client.Addr)
	}
	if cfg.System.Log.Level != "warn" {
		t.Fatalf("system log level=%q", cfg.System.Log.Level)
	}
	if cfg.Admin.Log.System.Level != "error" {
		t.Fatalf("admin system level=%q", cfg.Admin.Log.System.Level)
	}
	if cfg.Client.Log.System.Level != "info" {
		t.Fatalf("client system level leaked=%q", cfg.Client.Log.System.Level)
	}
}

func TestPostgresDSNEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "postgres:\n  dsn: from-file\n", "", "")
	t.Setenv("KIRIVERS_POSTGRES_DSN", "from-env")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Postgres.DSN != "from-env" {
		t.Fatalf("dsn=%q", cfg.System.Postgres.DSN)
	}
}

// TestUnknownSecurityKeysIgnored 钉住兼容结论：老部署 YAML 里残留、如今已从
// SecurityConfig 移除的配置键不再映射到任何字段，Viper 宽松解码会直接忽略它们——
// config.Load 不报错，其余 security 键照常按 YAML 值解析。
func TestUnknownSecurityKeysIgnored(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir,
		"security:\n  retired_auth_secret: \"legacy\"\n  retired_ticket_ttl_hours: 24\n  session_idle_hours: 8\n",
		"", "")
	t.Setenv("KIRIVERS_JOBS_WORKERS", "5")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("legacy YAML keys must not fail Load: %v", err)
	}
	if cfg.System.Security.SessionIdleHours != 8 {
		t.Fatalf("session_idle_hours=%d, want 8 from YAML", cfg.System.Security.SessionIdleHours)
	}
	if cfg.System.Jobs.Workers != 5 {
		t.Fatalf("workers=%d", cfg.System.Jobs.Workers)
	}
}

func TestSecuritySessionDefaultsAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "", "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Security.SessionIdleHours != 72 {
		t.Fatalf("session_idle_hours=%d", cfg.System.Security.SessionIdleHours)
	}
	if cfg.System.Security.LoginPendingTTLSeconds != 600 {
		t.Fatalf("login_pending_ttl_seconds=%d", cfg.System.Security.LoginPendingTTLSeconds)
	}
	if cfg.System.Security.TOTPMaxAttemptsPerPeriod != 10 {
		t.Fatalf("totp_max_attempts_per_period=%d", cfg.System.Security.TOTPMaxAttemptsPerPeriod)
	}
	if cfg.System.Security.WebAuthnRPID != "" {
		t.Fatalf("webauthn_rp_id=%q", cfg.System.Security.WebAuthnRPID)
	}
	t.Setenv("KIRIVERS_SECURITY_SESSION_IDLE_HOURS", "2")
	t.Setenv("KIRIVERS_SECURITY_LOGIN_PENDING_TTL_SECONDS", "30")
	t.Setenv("KIRIVERS_SECURITY_TOTP_MAX_ATTEMPTS_PER_PERIOD", "3")
	t.Setenv("KIRIVERS_SECURITY_WEBAUTHN_RP_ID", "localhost")
	t.Setenv("KIRIVERS_SECURITY_WEBAUTHN_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Security.SessionIdleHours != 2 {
		t.Fatalf("idle env=%d", cfg.System.Security.SessionIdleHours)
	}
	if cfg.System.Security.LoginPendingTTLSeconds != 30 {
		t.Fatalf("pending env=%d", cfg.System.Security.LoginPendingTTLSeconds)
	}
	if cfg.System.Security.TOTPMaxAttemptsPerPeriod != 3 {
		t.Fatalf("totp env=%d", cfg.System.Security.TOTPMaxAttemptsPerPeriod)
	}
	if cfg.System.Security.WebAuthnRPID != "localhost" {
		t.Fatalf("rp_id env=%q", cfg.System.Security.WebAuthnRPID)
	}
}

func TestLoadSystemIgnoresMissingPlaneFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("postgres:\n  dsn: cli-only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sys, err := LoadSystem(path)
	if err != nil {
		t.Fatal(err)
	}
	if sys.Postgres.DSN != "cli-only" {
		t.Fatalf("dsn=%q", sys.Postgres.DSN)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("server Load must require sibling plane files")
	}
}

func TestExampleFallbackIndependent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("postgres:\n  dsn: live\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "admin-example.yaml"), []byte("addr: \":9999\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "client-example.yaml"), []byte("addr: \":9998\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Postgres.DSN != "live" {
		t.Fatalf("system should use config.yaml, dsn=%q", cfg.System.Postgres.DSN)
	}
	if cfg.Admin.Addr != ":9999" || cfg.Client.Addr != ":9998" {
		t.Fatalf("example fallback admin=%q client=%q", cfg.Admin.Addr, cfg.Client.Addr)
	}
}

func TestExplicitDirMissingPlaneIsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("log:\n  level: info\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("explicit dir without admin.yaml/client.yaml must fail")
	}
}

func TestTLSEnvOverrideOnPlane(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "", "")
	t.Setenv("KIRIVERS_ADMIN_TLS_CERT", "admin.pem")
	t.Setenv("KIRIVERS_ADMIN_TLS_KEY", "admin.key")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.TLSCert != "admin.pem" || cfg.Admin.TLSKey != "admin.key" {
		t.Fatalf("admin tls cert=%q key=%q", cfg.Admin.TLSCert, cfg.Admin.TLSKey)
	}
	if !cfg.Admin.TLSEnabled() {
		t.Fatal("expected admin TLS from env")
	}
	if cfg.Client.TLSEnabled() {
		t.Fatal("client TLS must stay off")
	}
}

func TestExampleHasNoBootstrapAdminToken(t *testing.T) {
	files := []string{
		filepath.Join("..", "..", "configs", "config-example.yaml"),
		filepath.Join("..", "..", "configs", "admin-example.yaml"),
		filepath.Join("..", "..", "configs", "client-example.yaml"),
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "bootstrap_admin_token") {
			t.Fatalf("%s must not contain bootstrap_admin_token", path)
		}
	}
	system, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(system), "tls_cert") || strings.Contains(string(system), "\naddr:") {
		t.Fatal("system example must not contain plane addr/TLS")
	}
	for _, path := range files[1:] {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(raw)
		if !strings.Contains(s, "tls_cert") || !strings.Contains(s, "tls_key") {
			t.Fatalf("%s must document tls_cert and tls_key", path)
		}
	}
}

func TestEnvDisablesOneSinkLeavesTheOther(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "", "")
	t.Setenv("KIRIVERS_LOG_CONSOLE_ENABLED", "false")
	t.Setenv("KIRIVERS_ADMIN_LOG_ACCESS_FILE_ENABLED", "false")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Log.Console.Enabled {
		t.Fatal("system console should be disabled by env")
	}
	if !cfg.System.Log.File.Enabled {
		t.Fatal("system file must stay enabled")
	}
	if cfg.Admin.Log.Access.File.Enabled {
		t.Fatal("admin access file should be disabled by env")
	}
	if !cfg.Admin.Log.Access.Console.Enabled {
		t.Fatal("admin access console must stay enabled")
	}
	if !cfg.Client.Log.Access.File.Enabled || !cfg.Client.Log.Access.Console.Enabled {
		t.Fatal("client access sinks must be unchanged")
	}
}

func TestTrustedProxiesYAMLList(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "",
		"trusted_proxies:\n  - 192.0.2.1\n  - 10.0.0.0/8\n",
		"",
	)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Admin.TrustedProxies) != 2 || cfg.Admin.TrustedProxies[0] != "192.0.2.1" || cfg.Admin.TrustedProxies[1] != "10.0.0.0/8" {
		t.Fatalf("admin trusted_proxies=%v", cfg.Admin.TrustedProxies)
	}
	if len(cfg.Client.TrustedProxies) != 0 {
		t.Fatalf("client default must be empty, got %v", cfg.Client.TrustedProxies)
	}
}

func TestTrustedProxiesAdminEnvDoesNotLeakToClient(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "",
		"trusted_proxies:\n  - 192.0.2.1\n",
		"trusted_proxies:\n  - 198.51.100.1\n",
	)
	t.Setenv("KIRIVERS_ADMIN_TRUSTED_PROXIES", "203.0.113.1,203.0.113.2")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Admin.TrustedProxies) != 2 || cfg.Admin.TrustedProxies[0] != "203.0.113.1" || cfg.Admin.TrustedProxies[1] != "203.0.113.2" {
		t.Fatalf("admin env override=%v", cfg.Admin.TrustedProxies)
	}
	if len(cfg.Client.TrustedProxies) != 1 || cfg.Client.TrustedProxies[0] != "198.51.100.1" {
		t.Fatalf("client leaked admin env: %v", cfg.Client.TrustedProxies)
	}
}

func TestTrustedProxiesCommaSeparatedEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "", "")
	t.Setenv("KIRIVERS_CLIENT_TRUSTED_PROXIES", " 10.0.0.1 , 10.0.0.2 , ")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Client.TrustedProxies) != 2 || cfg.Client.TrustedProxies[0] != "10.0.0.1" || cfg.Client.TrustedProxies[1] != "10.0.0.2" {
		t.Fatalf("client comma split=%v", cfg.Client.TrustedProxies)
	}
	if len(cfg.Admin.TrustedProxies) != 0 {
		t.Fatalf("admin must stay empty, got %v", cfg.Admin.TrustedProxies)
	}
}

func TestExampleDocumentsTrustedProxies(t *testing.T) {
	adminRaw, err := os.ReadFile(filepath.Join("..", "..", "configs", "admin-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	clientRaw, err := os.ReadFile(filepath.Join("..", "..", "configs", "client-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	systemRaw, err := os.ReadFile(filepath.Join("..", "..", "configs", "config-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(adminRaw), "trusted_proxies:") || !strings.Contains(string(adminRaw), "KIRIVERS_ADMIN_TRUSTED_PROXIES") {
		t.Fatal("admin-example.yaml must document trusted_proxies and KIRIVERS_ADMIN_TRUSTED_PROXIES")
	}
	if !strings.Contains(string(clientRaw), "trusted_proxies:") || !strings.Contains(string(clientRaw), "KIRIVERS_CLIENT_TRUSTED_PROXIES") {
		t.Fatal("client-example.yaml must document trusted_proxies and KIRIVERS_CLIENT_TRUSTED_PROXIES")
	}
	if strings.Contains(string(systemRaw), "trusted_proxies") {
		t.Fatal("config-example.yaml must not contain trusted_proxies")
	}
}

func TestAdminStaticDirDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(writeThree(t, dir, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.StaticDir != "frontend/dist" {
		t.Fatalf("admin static_dir default=%q", cfg.Admin.StaticDir)
	}
	if cfg.Client.StaticDir != "" {
		t.Fatalf("client static_dir must stay empty, got %q", cfg.Client.StaticDir)
	}
}

func TestAdminStaticDirEmptyYAMLDisablesUI(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "static_dir: \"\"\n", "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.StaticDir != "" {
		t.Fatalf("empty YAML static_dir should disable UI, got %q", cfg.Admin.StaticDir)
	}
	if cfg.Client.StaticDir != "" {
		t.Fatalf("client static_dir leaked=%q", cfg.Client.StaticDir)
	}
}

func TestAdminStaticDirEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "static_dir: frontend/dist\n", "")
	t.Setenv("KIRIVERS_ADMIN_STATIC_DIR", "/opt/kirivers/admin-ui")
	t.Setenv("KIRIVERS_CLIENT_STATIC_DIR", "/should-not-bind")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.StaticDir != "/opt/kirivers/admin-ui" {
		t.Fatalf("admin static_dir=%q", cfg.Admin.StaticDir)
	}
	if cfg.Client.StaticDir != "" {
		t.Fatalf("KIRIVERS_CLIENT_STATIC_DIR must not bind, got %q", cfg.Client.StaticDir)
	}
}

func TestAdminStaticDirEnvOverrideOmitsYAMLKey(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "addr: \":8081\"\n", "")
	t.Setenv("KIRIVERS_ADMIN_STATIC_DIR", "/from-env")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.StaticDir != "/from-env" {
		t.Fatalf("omitted YAML key must still BindEnv, got %q", cfg.Admin.StaticDir)
	}
}

func TestExampleDocumentsAdminStaticDir(t *testing.T) {
	adminRaw, err := os.ReadFile(filepath.Join("..", "..", "configs", "admin-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	clientRaw, err := os.ReadFile(filepath.Join("..", "..", "configs", "client-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	systemRaw, err := os.ReadFile(filepath.Join("..", "..", "configs", "config-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	admin := string(adminRaw)
	if !strings.Contains(admin, "static_dir:") || !strings.Contains(admin, "KIRIVERS_ADMIN_STATIC_DIR") {
		t.Fatal("admin-example.yaml must document static_dir and KIRIVERS_ADMIN_STATIC_DIR")
	}
	if strings.Contains(string(clientRaw), "static_dir") || strings.Contains(string(clientRaw), "KIRIVERS_CLIENT_STATIC_DIR") {
		t.Fatal("client-example.yaml must not document static_dir")
	}
	if strings.Contains(string(systemRaw), "static_dir") {
		t.Fatal("config-example.yaml must not contain static_dir")
	}
}

func TestCacheDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(writeThree(t, dir, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Cache.Driver != "memory" {
		t.Fatalf("driver=%q", cfg.System.Cache.Driver)
	}
	if cfg.System.Cache.Redis.Addr != "127.0.0.1:6379" {
		t.Fatalf("addr=%q", cfg.System.Cache.Redis.Addr)
	}
	if cfg.System.Cache.Redis.Password != "" {
		t.Fatalf("password=%q", cfg.System.Cache.Redis.Password)
	}
	if cfg.System.Cache.Redis.DB != 0 {
		t.Fatalf("db=%d", cfg.System.Cache.Redis.DB)
	}
	if cfg.System.Cache.Redis.ReconnectIntervalMinutes != 10 {
		t.Fatalf("reconnect=%d", cfg.System.Cache.Redis.ReconnectIntervalMinutes)
	}
}

func TestCacheEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "cache:\n  driver: memory\n  redis:\n    addr: \"127.0.0.1:6379\"\n    password: file-secret\n    db: 1\n    reconnect_interval_minutes: 10\n", "", "")
	t.Setenv("KIRIVERS_CACHE_DRIVER", "redis")
	t.Setenv("KIRIVERS_CACHE_REDIS_ADDR", "10.0.0.5:6380")
	t.Setenv("KIRIVERS_CACHE_REDIS_PASSWORD", "from-env")
	t.Setenv("KIRIVERS_CACHE_REDIS_DB", "2")
	t.Setenv("KIRIVERS_CACHE_REDIS_RECONNECT_INTERVAL_MINUTES", "15")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Cache.Driver != "redis" {
		t.Fatalf("driver=%q", cfg.System.Cache.Driver)
	}
	if cfg.System.Cache.Redis.Addr != "10.0.0.5:6380" {
		t.Fatalf("addr=%q", cfg.System.Cache.Redis.Addr)
	}
	if cfg.System.Cache.Redis.Password != "from-env" {
		t.Fatalf("password=%q", cfg.System.Cache.Redis.Password)
	}
	if cfg.System.Cache.Redis.DB != 2 {
		t.Fatalf("db=%d", cfg.System.Cache.Redis.DB)
	}
	if cfg.System.Cache.Redis.ReconnectIntervalMinutes != 15 {
		t.Fatalf("reconnect=%d", cfg.System.Cache.Redis.ReconnectIntervalMinutes)
	}
}

func TestExampleDocumentsCache(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "configs", "config-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"cache:",
		"driver: memory",
		"KIRIVERS_CACHE_REDIS_PASSWORD",
		"reconnect_interval_minutes:",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("config-example.yaml must document %q", want)
		}
	}
}

func TestStreamDefaultsBothSinksEnabled(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(writeThree(t, dir, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	streams := []StreamConfig{
		cfg.System.Log,
		cfg.Admin.Log.System,
		cfg.Admin.Log.Access,
		cfg.Client.Log.System,
		cfg.Client.Log.Access,
	}
	for i, s := range streams {
		if !s.Console.Enabled || !s.File.Enabled {
			t.Fatalf("stream %d: console=%v file=%v", i, s.Console.Enabled, s.File.Enabled)
		}
	}
	if cfg.System.Log.File.Filename != "kirivers.log" {
		t.Fatalf("process filename=%q", cfg.System.Log.File.Filename)
	}
	if cfg.Admin.Log.System.File.Filename != "admin-system.log" || cfg.Admin.Log.Access.File.Filename != "admin-access.log" {
		t.Fatalf("admin files=%q/%q", cfg.Admin.Log.System.File.Filename, cfg.Admin.Log.Access.File.Filename)
	}
	if cfg.Client.Log.System.File.Filename != "client-system.log" || cfg.Client.Log.Access.File.Filename != "client-access.log" {
		t.Fatalf("client files=%q/%q", cfg.Client.Log.System.File.Filename, cfg.Client.Log.Access.File.Filename)
	}
}

func TestDynamicPackMaxBytesDefaultAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "log:\n  level: info\n", "", "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.DynamicPack.MaxBytes != DefaultDynamicPackMaxBytes {
		t.Fatalf("default max_bytes=%d want %d", cfg.System.DynamicPack.MaxBytes, DefaultDynamicPackMaxBytes)
	}

	t.Setenv("KIRIVERS_DYNAMIC_PACK_MAX_BYTES", "1024")
	cfg2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.System.DynamicPack.MaxBytes != 1024 {
		t.Fatalf("env max_bytes=%d", cfg2.System.DynamicPack.MaxBytes)
	}
}

func TestChangelogLimitsDefaultAndRejectInverted(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		path := writeThree(t, t.TempDir(), "log:\n  level: info\n", "", "")
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.System.Changelog.DefaultEntries != DefaultChangelogDefaultEntries {
			t.Fatalf("default default_entries=%d", cfg.System.Changelog.DefaultEntries)
		}
		if cfg.System.Changelog.MaxEntries != DefaultChangelogMaxEntries {
			t.Fatalf("default max_entries=%d", cfg.System.Changelog.MaxEntries)
		}
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("KIRIVERS_CHANGELOG_DEFAULT_ENTRIES", "3")
		t.Setenv("KIRIVERS_CHANGELOG_MAX_ENTRIES", "8")
		path := writeThree(t, t.TempDir(), "log:\n  level: info\n", "", "")
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.System.Changelog.DefaultEntries != 3 || cfg.System.Changelog.MaxEntries != 8 {
			t.Fatalf("env changelog=%+v", cfg.System.Changelog)
		}
	})

	t.Run("lt1-fallback", func(t *testing.T) {
		path := writeThree(t, t.TempDir(), "changelog:\n  default_entries: 0\n  max_entries: 0\n", "", "")
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.System.Changelog.DefaultEntries != DefaultChangelogDefaultEntries || cfg.System.Changelog.MaxEntries != DefaultChangelogMaxEntries {
			t.Fatalf("<1 fallback=%+v", cfg.System.Changelog)
		}
	})

	t.Run("default-gt-max", func(t *testing.T) {
		path := writeThree(t, t.TempDir(), "changelog:\n  default_entries: 10\n  max_entries: 5\n", "", "")
		if _, err := Load(path); err == nil {
			t.Fatal("default > max must fail Load")
		}
	})
}

func TestFileListMaxFilesDefaultEnvAndClamp(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "log:\n  level: info\n", "", "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.FileList.MaxFiles != DefaultFileListMaxFiles {
		t.Fatalf("default max_files=%d want %d", cfg.System.FileList.MaxFiles, DefaultFileListMaxFiles)
	}

	t.Setenv("KIRIVERS_FILE_LIST_MAX_FILES", "8")
	cfg2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.System.FileList.MaxFiles != 8 {
		t.Fatalf("env max_files=%d", cfg2.System.FileList.MaxFiles)
	}

	t.Setenv("KIRIVERS_FILE_LIST_MAX_FILES", "0")
	cfg3, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg3.System.FileList.MaxFiles != DefaultFileListMaxFiles {
		t.Fatalf("<1 must fall back to %d, got %d", DefaultFileListMaxFiles, cfg3.System.FileList.MaxFiles)
	}
}

func TestAdminEnabledDefaultAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "", "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Admin.Enabled {
		t.Fatal("admin.enabled default must be true")
	}
	if cfg.Client.Enabled {
		t.Fatal("client enabled must stay unset (false)")
	}

	t.Setenv("KIRIVERS_ADMIN_ENABLED", "false")
	cfg2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Admin.Enabled {
		t.Fatal("KIRIVERS_ADMIN_ENABLED=false must disable admin plane")
	}
}

func TestAdminEnabledYAMLFalse(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "enabled: false\n", "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Enabled {
		t.Fatal("admin.yaml enabled: false must disable")
	}
}

func TestNodeDisplayNameAndClusterDownloadDefaultsEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeThree(t, dir, "", "", "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.System.Node.DisplayName != "" {
		t.Fatalf("display_name=%q", cfg.System.Node.DisplayName)
	}
	if cfg.System.Cluster.Download != ClusterDownloadS3 {
		t.Fatalf("download default=%q", cfg.System.Cluster.Download)
	}

	t.Setenv("KIRIVERS_NODE_DISPLAY_NAME", "edge-hk")
	t.Setenv("KIRIVERS_CLUSTER_DOWNLOAD", "local")
	cfg2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.System.Node.DisplayName != "edge-hk" {
		t.Fatalf("display_name env=%q", cfg2.System.Node.DisplayName)
	}
	if cfg2.System.Cluster.Download != ClusterDownloadLocal {
		t.Fatalf("download env=%q", cfg2.System.Cluster.Download)
	}
}

func TestClusterActiveGate(t *testing.T) {
	localMem := &Config{System: SystemConfig{
		Storage: StorageConfig{Driver: "local"},
		Cache:   CacheConfig{Driver: "memory"},
	}}
	if ClusterActive(localMem) {
		t.Fatal("local+memory must not enable cluster")
	}
	s3Mem := &Config{System: SystemConfig{
		Storage: StorageConfig{Driver: "s3"},
		Cache:   CacheConfig{Driver: "memory"},
	}}
	if ClusterActive(s3Mem) {
		t.Fatal("s3+memory must not enable cluster")
	}
	localRedis := &Config{System: SystemConfig{
		Storage: StorageConfig{Driver: "local"},
		Cache:   CacheConfig{Driver: "redis"},
	}}
	if ClusterActive(localRedis) {
		t.Fatal("local+redis must not enable cluster")
	}
	s3Redis := &Config{System: SystemConfig{
		Storage: StorageConfig{Driver: "s3"},
		Cache:   CacheConfig{Driver: "redis"},
	}}
	if !ClusterActive(s3Redis) {
		t.Fatal("s3+redis must enable cluster")
	}
	if ClusterActive(nil) {
		t.Fatal("nil config")
	}
}

func TestExampleDocumentsClusterAndPrivateStorage(t *testing.T) {
	systemRaw, err := os.ReadFile(filepath.Join("..", "..", "configs", "config-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	adminRaw, err := os.ReadFile(filepath.Join("..", "..", "configs", "admin-example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	sys := string(systemRaw)
	for _, want := range []string{
		"node:",
		"display_name:",
		"cluster:",
		"download: s3",
		"KIRIVERS_CLUSTER_DOWNLOAD",
		"KIRIVERS_NODE_DISPLAY_NAME",
		"public_base_url:",
		"kirivers-private",
		"KIRIVERS_STORAGE_PRIVATE_",
	} {
		if !strings.Contains(sys, want) {
			t.Fatalf("config-example.yaml must document %q", want)
		}
	}
	admin := string(adminRaw)
	if !strings.Contains(admin, "enabled:") || !strings.Contains(admin, "KIRIVERS_ADMIN_ENABLED") {
		t.Fatal("admin-example.yaml must document enabled and KIRIVERS_ADMIN_ENABLED")
	}
}
