// Package config 使用 Viper 加载三份 YAML（系统 / 管理平面 / 客户端平面），
// 并允许 KIRIVERS_ / KIRIVERS_ADMIN_ / KIRIVERS_CLIENT_ 前缀环境变量覆盖。
// 其它包不得自行读取环境变量拼接 DSN 或存储密钥。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	systemFileName = "config.yaml"
	adminFileName  = "admin.yaml"
	clientFileName = "client.yaml"

	envPrefixSystem = "KIRIVERS"
	envPrefixAdmin  = "KIRIVERS_ADMIN"
	envPrefixClient = "KIRIVERS_CLIENT"
)

// Config 是 HTTP 服务端进程级配置：系统文件 + 两个平面文件。
type Config struct {
	System SystemConfig
	Admin  PlaneConfig
	Client PlaneConfig
}

// ClusterDownloadS3 是多节点下默认下载方式：check/feed 给出公共对象绝对 URL。
const ClusterDownloadS3 = "s3"

// ClusterDownloadLocal 是多节点本机代拉：check/feed 仍走 /packages/{sha256}。
const ClusterDownloadLocal = "local"

// SystemConfig 来自 config.yaml：共享基础设施与进程级系统日志。不含监听地址与 TLS。
type SystemConfig struct {
	Log      StreamConfig   `mapstructure:"log"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Security SecurityConfig `mapstructure:"security"`
	Jobs     JobsConfig     `mapstructure:"jobs"`
	Cache    CacheConfig    `mapstructure:"cache"`
	// Node 是本进程可选显示名（仅管理台；空则列表用节点 UUID）。
	Node NodeConfig `mapstructure:"node"`
	// Cluster 控制多节点下载提供方式；仅 ClusterActive 时生效。
	Cluster ClusterConfig `mapstructure:"cluster"`
	// DynamicPack 是多文件客户端 fileset 动态打包闸门（默认 512MiB）。
	DynamicPack DynamicPackConfig `mapstructure:"dynamic_pack"`
	// FileList 是原生 file_list 待下载条数的平台天花板（默认 16）。
	FileList FileListConfig `mapstructure:"file_list"`
	// Changelog 是原生 changelog 条数窗口（无 from 用 default，有 from 用 max）。
	Changelog ChangelogConfig `mapstructure:"changelog"`
	// URLSigningSecret 是私有存储短时签名 URL（§13.7）的实例级 HMAC 密钥。
	// 为空时进程启动生成临时随机值（重启即全部已签发 URL 失效）并告警。
	URLSigningSecret string `mapstructure:"url_signing_secret"`
}

// NodeConfig 是本进程节点身份的可选展示字段。
type NodeConfig struct {
	// DisplayName 仅 UI 使用，不参与身份分配。
	DisplayName string `mapstructure:"display_name"`
}

// ClusterConfig 是实例级多节点行为。Download 取值 s3 | local。
type ClusterConfig struct {
	Download string `mapstructure:"download"`
}

// DefaultDynamicPackMaxBytes 是 dynamic_pack.max_bytes 的默认硬顶（512MiB）。
const DefaultDynamicPackMaxBytes int64 = 512 * 1024 * 1024

// DefaultFileListMaxFiles 是 file_list.max_files 的默认平台天花板。
const DefaultFileListMaxFiles = 16

// DefaultChangelogDefaultEntries 是 changelog.default_entries 的缺省（无 from_version）。
const DefaultChangelogDefaultEntries = 5

// DefaultChangelogMaxEntries 是 changelog.max_entries 的缺省（有 from_version 截断）。
const DefaultChangelogMaxEntries = 50

// DynamicPackConfig 控制客户端 fileset 动态打包入队前的未压缩体积硬顶（D6）。
type DynamicPackConfig struct {
	// MaxBytes 差异路径在目标 Manifest 上的 size 去重合计达到该值则不入队、立即 full_package。
	MaxBytes int64 `mapstructure:"max_bytes"`
}

// FileListConfig 控制原生 POST /update/diff 的 file_list 待下载条数硬顶。
type FileListConfig struct {
	// MaxFiles 项目可下调、不得超出；加载后 <1 回退 DefaultFileListMaxFiles。
	MaxFiles int `mapstructure:"max_files"`
}

// ChangelogConfig 控制原生 GET /changelog 的条数窗口。
type ChangelogConfig struct {
	// DefaultEntries 无 from_version 时取最新几条；加载后 <1 回退 DefaultChangelogDefaultEntries。
	DefaultEntries int `mapstructure:"default_entries"`
	// MaxEntries 有 from_version 时区间截断上限；加载后 <1 回退 DefaultChangelogMaxEntries。
	MaxEntries int `mapstructure:"max_entries"`
}

// PlaneConfig 来自 admin.yaml / client.yaml：该平面的监听、TLS、日志；admin 另有 StaticDir。
type PlaneConfig struct {
	Addr    string `mapstructure:"addr"`
	Mode    string `mapstructure:"mode"`
	TLSCert string `mapstructure:"tls_cert"`
	TLSKey  string `mapstructure:"tls_key"`
	// Enabled 控制是否监听该平面。仅 admin.yaml / KIRIVERS_ADMIN_ENABLED 使用；默认 true。
	// false 时进程不绑定管理地址（无管理 API、不托管 SPA、无 admin→client 反代）。
	Enabled bool `mapstructure:"enabled"`
	// TrustedProxies 是该平面信任的反向代理（IP 或 CIDR）。空则不信任 X-Forwarded-For，ClientIP 为直连地址。
	TrustedProxies []string `mapstructure:"trusted_proxies"`
	// StaticDir 是编译后管理台静态根，仅 admin 平面使用（client 保持零值，cmd 忽略）。
	// 默认 frontend/dist；空字符串表示不托管 UI。相对路径相对进程 CWD。
	StaticDir string         `mapstructure:"static_dir"`
	Log       PlaneLogConfig `mapstructure:"log"`
}

// TLSEnabled 表示该平面证书与私钥均已配置，应走 HTTPS。无进程级 TLS 回退。
func (p PlaneConfig) TLSEnabled() bool {
	return strings.TrimSpace(p.TLSCert) != "" && strings.TrimSpace(p.TLSKey) != ""
}

// PlaneLogConfig 是平面内系统流与访问流。
type PlaneLogConfig struct {
	System StreamConfig `mapstructure:"system"`
	Access StreamConfig `mapstructure:"access"`
}

// StreamConfig 控制一条日志流的级别、控制台与滚动文件。
type StreamConfig struct {
	Level   string        `mapstructure:"level"`
	Console ConsoleConfig `mapstructure:"console"`
	File    FileConfig    `mapstructure:"file"`
}

// ConsoleConfig 映射 zerolog.ConsoleWriter（stderr）。NoColor 透传 ConsoleWriter.NoColor。
type ConsoleConfig struct {
	Enabled bool `mapstructure:"enabled"`
	NoColor bool `mapstructure:"no_color"`
}

// FileConfig 映射 lumberjack 滚动 JSON 文件。file.enabled 时 Dir 或 Filename 任一为空则失败（不会落到 TempDir）。
type FileConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Dir        string `mapstructure:"dir"`
	Filename   string `mapstructure:"filename"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
	LocalTime  bool   `mapstructure:"local_time"`
}

// PostgresConfig 保存 GORM 连接串。
type PostgresConfig struct {
	DSN string `mapstructure:"dsn"`
}

// StorageConfig 选择 LocalFS 或 S3 兼容后端。
// S3 是公共产物桶；Private 是仅后端对象（GeoIP MMDB）。driver=local 时两类都落在 local.root。
type StorageConfig struct {
	Driver  string             `mapstructure:"driver"`
	Local   LocalStorageConfig `mapstructure:"local"`
	S3      S3StorageConfig    `mapstructure:"s3"`
	Private S3StorageConfig    `mapstructure:"private"`
}

// LocalStorageConfig 是本地磁盘根目录。
type LocalStorageConfig struct {
	Root string `mapstructure:"root"`
}

// S3StorageConfig 兼容 MinIO / R2 / AWS S3。
type S3StorageConfig struct {
	Endpoint      string `mapstructure:"endpoint"`
	Region        string `mapstructure:"region"`
	Bucket        string `mapstructure:"bucket"`
	AccessKey     string `mapstructure:"access_key"`
	SecretKey     string `mapstructure:"secret_key"`
	UsePathStyle  bool   `mapstructure:"use_path_style"`
	PublicBaseURL string `mapstructure:"public_base_url"`
}

// SecurityConfig 保存管理员会话 TTL、登录待完成 TTL、TOTP 阈值与 WebAuthn RP。
// 首个管理员由 CLI 创建，不使用引导 Token；管理员鉴权只查缓存会话 Token。
type SecurityConfig struct {
	SessionIdleHours         int      `mapstructure:"session_idle_hours"`
	LoginPendingTTLSeconds   int      `mapstructure:"login_pending_ttl_seconds"`
	TOTPMaxAttemptsPerPeriod int      `mapstructure:"totp_max_attempts_per_period"`
	WebAuthnRPID             string   `mapstructure:"webauthn_rp_id"`
	WebAuthnOrigins          []string `mapstructure:"webauthn_origins"`
}

// JobsConfig 控制进程内 Job worker 数量。
type JobsConfig struct {
	Workers int `mapstructure:"workers"`
}

// CacheConfig 选择进程内内存缓存或 Redis。未知 driver 由 cache.Open 拒绝。
// Redis 只做读缓存，不得用作 Job 队列。
type CacheConfig struct {
	Driver string           `mapstructure:"driver"`
	Redis  RedisCacheConfig `mapstructure:"redis"`
}

// RedisCacheConfig 是单实例 Redis 连接参数（无 Sentinel/Cluster）。
type RedisCacheConfig struct {
	Addr                     string `mapstructure:"addr"`
	Password                 string `mapstructure:"password"`
	DB                       int    `mapstructure:"db"`
	ReconnectIntervalMinutes int    `mapstructure:"reconnect_interval_minutes"`
}

// Load 读取三份 YAML。path 为空时在 configs/ 与当前目录搜索，缺失文件各自回退到 *-example.yaml。
// path 指向目录或系统 YAML 文件时为显式路径：缺文件即失败（服务端需要三份齐全）。
func Load(path string) (*Config, error) {
	explicit, dir, sysFile, err := splitConfigPath(path)
	if err != nil {
		return nil, err
	}

	sysV := newSystemViper()
	if err := readYAML(sysV, explicit, sysFile, "config"); err != nil {
		return nil, err
	}
	sys := SystemConfig{}
	if err := sysV.Unmarshal(&sys); err != nil {
		return nil, fmt.Errorf("unmarshal system config: %w", err)
	}

	adminV := newPlaneViper(envPrefixAdmin, ":8081", "admin-system.log", "admin-access.log")
	// static_dir / enabled 只绑 admin Viper，避免无意义的 KIRIVERS_CLIENT_*。
	adminV.SetDefault("static_dir", "frontend/dist")
	adminV.SetDefault("enabled", true)
	_ = adminV.BindEnv("static_dir")
	_ = adminV.BindEnv("enabled")
	if err := readYAML(adminV, explicit, filepath.Join(dir, adminFileName), "admin"); err != nil {
		return nil, err
	}
	admin := PlaneConfig{}
	if err := adminV.Unmarshal(&admin); err != nil {
		return nil, fmt.Errorf("unmarshal admin config: %w", err)
	}
	admin.TrustedProxies = normalizeTrustedProxies(admin.TrustedProxies)

	clientV := newPlaneViper(envPrefixClient, ":8080", "client-system.log", "client-access.log")
	if err := readYAML(clientV, explicit, filepath.Join(dir, clientFileName), "client"); err != nil {
		return nil, err
	}
	client := PlaneConfig{}
	if err := clientV.Unmarshal(&client); err != nil {
		return nil, fmt.Errorf("unmarshal client config: %w", err)
	}
	client.TrustedProxies = normalizeTrustedProxies(client.TrustedProxies)

	sys.FileList.MaxFiles = normalizeFileListMaxFiles(sys.FileList.MaxFiles)
	normalizeChangelogLimits(&sys.Changelog)
	if err := validateChangelogLimits(sys.Changelog); err != nil {
		return nil, err
	}
	sys.Cluster.Download = normalizeClusterDownload(sys.Cluster.Download)
	return &Config{System: sys, Admin: admin, Client: client}, nil
}

// LoadSystem 只读系统 YAML，供 kirivers admin 使用；不要求平面文件存在。
func LoadSystem(path string) (*SystemConfig, error) {
	explicit, _, sysFile, err := splitConfigPath(path)
	if err != nil {
		return nil, err
	}
	v := newSystemViper()
	if err := readYAML(v, explicit, sysFile, "config"); err != nil {
		return nil, err
	}
	cfg := &SystemConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal system config: %w", err)
	}
	cfg.FileList.MaxFiles = normalizeFileListMaxFiles(cfg.FileList.MaxFiles)
	normalizeChangelogLimits(&cfg.Changelog)
	if err := validateChangelogLimits(cfg.Changelog); err != nil {
		return nil, err
	}
	cfg.Cluster.Download = normalizeClusterDownload(cfg.Cluster.Download)
	return cfg, nil
}

func normalizeFileListMaxFiles(n int) int {
	if n < 1 {
		return DefaultFileListMaxFiles
	}
	return n
}

func normalizeChangelogLimits(c *ChangelogConfig) {
	if c == nil {
		return
	}
	if c.DefaultEntries < 1 {
		c.DefaultEntries = DefaultChangelogDefaultEntries
	}
	if c.MaxEntries < 1 {
		c.MaxEntries = DefaultChangelogMaxEntries
	}
}

func validateChangelogLimits(c ChangelogConfig) error {
	if c.DefaultEntries > c.MaxEntries {
		return fmt.Errorf("changelog.default_entries (%d) must not exceed changelog.max_entries (%d)", c.DefaultEntries, c.MaxEntries)
	}
	return nil
}

func normalizeClusterDownload(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ClusterDownloadLocal:
		return ClusterDownloadLocal
	default:
		return ClusterDownloadS3
	}
}

// ClusterActive 为 true 当且仅当公共存储是远程（s3）且缓存是 Redis。
// 单机 local / memory 不启用节点登记、下载方式分支与按节点可见性。产物键始终走 {slug}/{sha256}。
func ClusterActive(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	return ClusterActiveSystem(&cfg.System)
}

// ClusterActiveSystem 对仅加载系统 YAML 的路径判定多节点门闩。
func ClusterActiveSystem(sys *SystemConfig) bool {
	if sys == nil {
		return false
	}
	driver := strings.ToLower(strings.TrimSpace(sys.Storage.Driver))
	cache := strings.ToLower(strings.TrimSpace(sys.Cache.Driver))
	return driver == "s3" && cache == "redis"
}

// PrivateStorageConfigured 表示 storage.private 已配齐桶名，可打开私有后端。
func PrivateStorageConfigured(sys SystemConfig) bool {
	return strings.TrimSpace(sys.Storage.Private.Bucket) != ""
}

// splitConfigPath 解析 -config / KIRIVERS_CONFIG：空为默认搜索；目录或文件为显式。
func splitConfigPath(path string) (explicit bool, dir, sysFile string, err error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, "", "", nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false, "", "", fmt.Errorf("read config: %w", err)
	}
	if fi.IsDir() {
		return true, path, filepath.Join(path, systemFileName), nil
	}
	return true, filepath.Dir(path), path, nil
}

func readYAML(v *viper.Viper, explicit bool, explicitFile, searchName string) error {
	if explicit {
		v.SetConfigFile(explicitFile)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("read config: %w", err)
		}
		return nil
	}
	v.SetConfigType("yaml")
	v.AddConfigPath("configs")
	v.AddConfigPath(".")
	v.SetConfigName(searchName)
	if err := v.ReadInConfig(); err != nil {
		v.SetConfigName(searchName + "-example")
		if err2 := v.ReadInConfig(); err2 != nil {
			return fmt.Errorf("read config: %w", err)
		}
	}
	return nil
}

func newSystemViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix(envPrefixSystem)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setStreamDefaults(v, "log", "kirivers.log")
	v.SetDefault("storage.driver", "local")
	v.SetDefault("storage.local.root", "./data/storage")
	v.SetDefault("jobs.workers", 2)
	v.SetDefault("security.session_idle_hours", 72)
	v.SetDefault("security.login_pending_ttl_seconds", 600)
	v.SetDefault("security.totp_max_attempts_per_period", 10)
	v.SetDefault("security.webauthn_rp_id", "")
	v.SetDefault("security.webauthn_origins", []string{})
	v.SetDefault("cache.driver", "memory")
	v.SetDefault("cache.redis.addr", "127.0.0.1:6379")
	v.SetDefault("cache.redis.password", "")
	v.SetDefault("cache.redis.db", 0)
	v.SetDefault("cache.redis.reconnect_interval_minutes", 10)
	v.SetDefault("dynamic_pack.max_bytes", DefaultDynamicPackMaxBytes)
	v.SetDefault("file_list.max_files", DefaultFileListMaxFiles)
	v.SetDefault("changelog.default_entries", DefaultChangelogDefaultEntries)
	v.SetDefault("changelog.max_entries", DefaultChangelogMaxEntries)
	v.SetDefault("node.display_name", "")
	v.SetDefault("cluster.download", ClusterDownloadS3)

	bindStreamEnv(v, "log")
	for _, key := range []string{
		"postgres.dsn",
		"storage.driver",
		"storage.local.root",
		"storage.s3.endpoint",
		"storage.s3.region",
		"storage.s3.bucket",
		"storage.s3.access_key",
		"storage.s3.secret_key",
		"storage.s3.use_path_style",
		"storage.s3.public_base_url",
		"storage.private.endpoint",
		"storage.private.region",
		"storage.private.bucket",
		"storage.private.access_key",
		"storage.private.secret_key",
		"storage.private.use_path_style",
		"storage.private.public_base_url",
		"node.display_name",
		"cluster.download",
		"security.session_idle_hours",
		"security.login_pending_ttl_seconds",
		"security.totp_max_attempts_per_period",
		"security.webauthn_rp_id",
		"security.webauthn_origins",
		"jobs.workers",
		"url_signing_secret",
		"cache.driver",
		"cache.redis.addr",
		"cache.redis.password",
		"cache.redis.db",
		"cache.redis.reconnect_interval_minutes",
		"dynamic_pack.max_bytes",
		"file_list.max_files",
		"changelog.default_entries",
		"changelog.max_entries",
	} {
		_ = v.BindEnv(key)
	}
	return v
}

func newPlaneViper(envPrefix, defaultAddr, systemFile, accessFile string) *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("addr", defaultAddr)
	v.SetDefault("mode", "debug")
	v.SetDefault("tls_cert", "")
	v.SetDefault("tls_key", "")
	v.SetDefault("trusted_proxies", []string{})
	setStreamDefaults(v, "log.system", systemFile)
	setStreamDefaults(v, "log.access", accessFile)

	for _, key := range []string{"addr", "mode", "tls_cert", "tls_key", "trusted_proxies"} {
		_ = v.BindEnv(key)
	}
	bindStreamEnv(v, "log.system")
	bindStreamEnv(v, "log.access")
	return v
}

func setStreamDefaults(v *viper.Viper, prefix, filename string) {
	v.SetDefault(prefix+".level", "info")
	v.SetDefault(prefix+".console.enabled", true)
	v.SetDefault(prefix+".console.no_color", false)
	v.SetDefault(prefix+".file.enabled", true)
	v.SetDefault(prefix+".file.dir", "./logs")
	v.SetDefault(prefix+".file.filename", filename)
	v.SetDefault(prefix+".file.max_size_mb", 100)
	v.SetDefault(prefix+".file.max_backups", 10)
	v.SetDefault(prefix+".file.max_age_days", 28)
	v.SetDefault(prefix+".file.compress", true)
	v.SetDefault(prefix+".file.local_time", true)
}

// normalizeTrustedProxies 把 YAML 列表或环境变量整串（常为单个 "a,b" 元素）规范成 IP/CIDR 切片。
func normalizeTrustedProxies(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, part)
		}
	}
	return out
}

func bindStreamEnv(v *viper.Viper, prefix string) {
	for _, key := range []string{
		"level",
		"console.enabled",
		"console.no_color",
		"file.enabled",
		"file.dir",
		"file.filename",
		"file.max_size_mb",
		"file.max_backups",
		"file.max_age_days",
		"file.compress",
		"file.local_time",
	} {
		_ = v.BindEnv(prefix + "." + key)
	}
}
