// Package store 是商店协议 feed（docs/app-init.md §9 / §9.2）的共享投影框架。
//
// 原生 JSON API 是权威；每种商店协议（Sparkle、electron-updater、Tauri、
// Squirrel 等）单独适配：按 §4.4 匿名口径选出该渠道客户端可见的目标版本，
// 再投影为协议自己的 schema。本包只沉淀九个 feed 协议共用的骨架：
//
//   - Request / Response / Adapter 接口与 Registry（按协议名注册）；
//   - AnonymousVisible 匿名可见性过滤（§4.4 / §9：灰度未 100% 的非强制版本
//     永不出现在公开 feed）；
//   - 渲染产物的 ETag 与 Cache-Control / Vary 计算口径。
//
// HTTP 层（internal/controller/client/store）统一处理路由、StoreAuth 鉴权、
// listing 解析失败 → 404、ETag / If-None-Match 304 / 缓存头；Adapter
// 只做投影，永不接触 HTTP 头。
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// ProtocolSparkle 是 Sparkle / WinSparkle / NetSparkle appcast 的协议名
// （对应 Project.StoreProtocols 的 jsonb 键）。
const ProtocolSparkle = "sparkle"

// ProtocolElectron 是 electron-updater generic provider 的协议名
// （对应 Project.StoreProtocols 的 jsonb 键）。
const ProtocolElectron = "electron"

// ProtocolWinGet 是 Microsoft WinGet REST 源的协议名
// （对应 Project.StoreProtocols 的 jsonb 键）。
const ProtocolWinGet = "winget"

// 错误定义：HTTP 层统一映射为错误包，Adapter 不感知状态码。
var (
	// ErrMissingParam 请求缺少必填 query 参数（os / arch 等）→ HTTP 400。
	ErrMissingParam = errors.New("missing required query parameter")
	// ErrUnknownChannel 渠道不存在（§4.3；渠道默认 stable，未知渠道 400）。
	ErrUnknownChannel = errors.New("unknown channel")
	// ErrUnknownPath 协议文档路径未知（如 electron 的 latest.yml 之外任意
	// 文件名）→ HTTP 纯文本 404（§9.2：绝不冒充协议产物，也不回退 zip）。
	ErrUnknownPath = errors.New("unknown document path")
	// ErrNoRelease 该渠道+平台匿名可见集为空（latest 型 feed 无可投影版本）
	// → HTTP 纯文本 404（不发空 yml，客户端将其解读为「无更新」会掩盖配置
	// 问题，与 feed 约定的 404 语义一致）。
	ErrNoRelease = errors.New("no visible release")
)

// ArtifactReader 是 feed 适配器按需读取产物字节的最小存储合同
// （实现：storage.Backend）。Sparkle 的 Ed25519 文件签名与 electron 的
// SHA-512 兜底计算需要（列表类渲染本身不读文件）。
type ArtifactReader interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}

// Deps 是 Adapter Render 需要的进程级依赖集合。字段均可为 nil：
//   - Updates 为 nil 时 Adapter 无法加载目录（构造期保证非 nil）；
//   - Signer 为 nil 或公开项目时不签名下载 URL（§13.7 / C15-2）；
//   - Storage 为 nil 时跳过文件字节级计算（Sparkle 文件签名降级为未签名，
//     electron 的 SHA-512 兜底降级为空值；公开项目无需签名器也不读文件）。
type Deps struct {
	Updates    *update.Service
	Signer     update.URLSigner
	Storage    ArtifactReader
	ObjectURL  func(slug, sha256, storageKey string) string
	LocalProxy bool
}

// Request 是一次 feed 渲染请求（HTTP 层完成原始绑定与规范化）。
type Request struct {
	// Project 是 StoreAuth 已解析的项目；恒非 nil。
	Project *model.Project
	// Channel 是目标渠道 slug；HTTP 层默认 stable，未知渠道由 Adapter 报错。
	Channel string
	// OS / Arch 是已按 §4.1–4.2 别名表规范化（darwin→macos、amd64→x86_64 等）
	// 的请求平台，与目录加载平台一致。
	OS, Arch string
	// HWRev 是请求硬件代号；空串 = 默认变体。Sparkle 等无法表达 hw_rev 的
	// 协议恒按默认变体投影（C16-8），Adapter 自行裁决是否使用。
	HWRev string
	// Path 是协议段之后的文档路径（如 appcast.xml / latest.yml / latest-mac.yml）。
	// 路由型协议（electron：文件名决定 os）据此分发；单文档协议（Sparkle）
	// 忽略。未知路径由 Adapter 报 ErrUnknownPath（→ 404）。
	Path string
	// CurrentVersion 是路径/请求形态中的当前版本（Tauri updater 需要）；
	// 列表型 feed（Sparkle 等）为空，不参与渲染。
	CurrentVersion string
	// ChangelogLocale / Locale / AcceptLanguage 组成语言回退链
	//（update.BuildLocaleChain，与原生 check 同口径）。
	ChangelogLocale string
	Locale          string
	AcceptLanguage  string
	// Method 是 HTTP 请求方法（GET / POST 等，用于 WinGet 等协议）。
	Method string
	// RawBody 是请求体原始内容（若存在）。
	RawBody []byte
	// QueryParams 携带请求的附加 Query 键值对。
	QueryParams map[string]string
	// Listing 是框架按 protocol+slug 解析的上架行；HTTP 层保证非 nil 且 enabled。
	Listing *model.StoreListing
}

// Response 是渲染结果：HTTP 层据此写状态码、Content-Type 与 body。
// ETag / Cache-Control 不由 Adapter 产出——由 HTTP 层对 Body 统一计算，
// 保证所有协议缓存语义一致（§9.2）。
type Response struct {
	// Body 是完整渲染产物（XML / YAML / JSON 等）。
	Body []byte
	// ContentType 是协议自己的 MIME（如 application/xml; charset=utf-8）。
	ContentType string
	// Status 目前为 200 / 204（Tauri 无更新 204）。框架保证 200 才写 body。
	Status int
}

// Adapter 是单个商店协议的投影实现。注册进 Registry 后由 HTTP 层按
// /store/{protocol}/{path...} 第一段分发。
type Adapter interface {
	// Protocol 返回协议名（如 "sparkle"），与 StoreListing.Protocol 对齐。
	Protocol() string
	// Enabled 保留给单测；HTTP 层以 listing 解析为闸，未命中 slug 不会进入 Adapter。
	Enabled(p *model.Project) bool
	// Render 加载目录并投影。目录 / 存储 / 签名失败返回 error（→ 500），
	// 参数问题返回 ErrMissingParam / ErrUnknownChannel（→ 400）。
	Render(ctx context.Context, deps *Deps, req Request) (*Response, error)
}

// Registry 是协议名 → Adapter 的并发安全注册表。
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

// Register 注册一个 Adapter；同名覆盖（进程内装配只发生一次，测试可重装）。
func (r *Registry) Register(a Adapter) {
	r.adapters[a.Protocol()] = a
}

// Get 按协议名取 Adapter；未注册返回 false（HTTP 层 → 404）。
func (r *Registry) Get(protocol string) (Adapter, bool) {
	a, ok := r.adapters[protocol]
	return a, ok
}

// DefaultRegistry 返回内置全部协议 Adapter 的注册表（sparkle、electron、
// tauri、squirrel、clickonce、appimage、winget、msix 与 fdroid；已覆盖 docs/app-init.md §9 全部 8 种商店协议）。
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewSparkleAdapter())
	r.Register(NewElectronAdapter())
	r.Register(NewTauriAdapter())
	r.Register(NewSquirrelAdapter())
	r.Register(NewClickOnceAdapter())
	r.Register(NewAppImageAdapter())
	r.Register(NewWinGetAdapter())
	r.Register(NewMSIXAdapter())
	r.Register(NewFDroidAdapter())
	return r
}

// BodyETag 由渲染 body 计算强 ETag（格式 `"hex"`，SHA-256 前 16 字节 hex）。
// 所有 feed 协议共用：内容变化 → ETag 变化，签名 URL 与请求 query 不参与。
func BodyETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// CacheControlFor 计算 feed 的 Cache-Control（§9.2 / C15-1）：
//   - 本机代拉：private, no-store——各节点可见集不同，不得进共享 CDN（与原生 check 同源）；
//   - 私有存储项目：private, no-store——body 内的签名 URL 随 TTL 过期，
//     不得进共享 CDN（HTTP 层同时跳过 304 短路）；
//   - 其余：public, s-maxage=<项目配置，缺省 60>, stale-while-revalidate=30。
func CacheControlFor(p *model.Project, localProxy bool) string {
	if localProxy || (p != nil && p.StorageVisibility == model.StorageVisibilityPrivate) {
		return "private, no-store"
	}
	sMaxage := 0
	if p != nil {
		sMaxage = p.CacheSMaxageSeconds
	}
	if sMaxage <= 0 {
		sMaxage = model.DefaultCacheSMaxageSeconds
	}
	return fmt.Sprintf("public, s-maxage=%d, stale-while-revalidate=30", sMaxage)
}

// enabledStoreProtocol 已废弃：HTTP 闸是 listing 解析。非 nil 项目恒 true。
func enabledStoreProtocol(p *model.Project, _ string) bool {
	return p != nil
}
