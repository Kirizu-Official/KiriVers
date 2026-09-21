package store

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// Squirrel RELEASES 清单投影（docs/app-init.md §9 Squirrel 行 / §9.2，C19-1..C19-6）。
//
// 产出 Squirrel（Windows）可直接消费的 RELEASES 文件：
//   - 端点：GET /store/squirrel/default/RELEASES?channel=&arch=；
//   - os 恒为 windows（Squirrel 仅运行于 Windows 平台）；arch 缺省为 x86_64；
//   - 格式为单行纯文本，行序新→旧：<SHA1> <filename> <size>\n；
//   - filename 为该 Line 默认 hw 变体 kind=full 产物的稳定文件名（§5.9）；
//     注意：私有项目 URL 不进入 RELEASES（Squirrel 按 RELEASES 所在 URL 相对解析文件名，
//     下载经同目录 /packages/ 路由；若私有项目需签名，由整目录反向代理或网关承载，
//     此处恒输出文件名本身）；
//   - SHA-1：Artifact 表无 SHA1 列，首请求从存储流式计算并按产物 SHA-256 走单飞 + LRU 缓存
//     （复用 SignatureCache，独立实例）。不回填数据库（保持读路径无写库依赖，同 electron sha512 先例）；
//   - size 为产物字节数（Artifact.Size）；
//   - 可见性：按 §4.4 匿名口径（AnonymousVisible），灰度未 100% 且非关键版本永不出现；
//   - hw_rev：Squirrel 协议无法表达硬件代号，恒只投影默认变体（C19-6）；
//   - 开关与包标识（C19-2, C19-4）：StoreProtocols["squirrel"].Enabled 关闭时由 HTTP 层返回 404；
//     StoreProtocols["squirrel"].Identifiers（如 releases_name）允许配置且不阻断；
//     可见集为空时返回 ErrNoRelease（HTTP 层返回 404 纯文本，绝不冒充或回退 zip）。
const (
	// ProtocolSquirrel 是 Squirrel 协议名（Project.StoreProtocols 的 jsonb 键）。
	ProtocolSquirrel = "squirrel"

	// squirrelContentType 是 Squirrel RELEASES 文档的 MIME。
	squirrelContentType = "text/plain; charset=utf-8"

	// squirrelDocPath 是 Squirrel 清单文档固定路径（C19-1）。
	squirrelDocPath = "RELEASES"

	// squirrelDefaultArch 是未提供 arch query 时的默认架构。
	squirrelDefaultArch = "x86_64"
)

// SquirrelAdapter 是 Squirrel RELEASES 协议的 Adapter 实现。
type SquirrelAdapter struct {
	// sha1Cache 缓存「产物字节 → 40 字符小写 hex SHA-1 摘要」，避免每次请求读取全量文件。
	// 键 = 产物 SHA-256，独立实例。
	sha1Cache *SignatureCache
}

// compile-time 校验：满足 Adapter 合同。
var _ Adapter = (*SquirrelAdapter)(nil)

// NewSquirrelAdapter 构造 Squirrel Adapter（默认容量 SHA-1 缓存）。
func NewSquirrelAdapter() *SquirrelAdapter {
	return &SquirrelAdapter{sha1Cache: NewSignatureCache(0)}
}

// NewSquirrelAdapterWithCache 以给定缓存构造（测试可注入）。
func NewSquirrelAdapterWithCache(cache *SignatureCache) *SquirrelAdapter {
	return &SquirrelAdapter{sha1Cache: cache}
}

// Protocol 实现 Adapter。
func (a *SquirrelAdapter) Protocol() string { return ProtocolSquirrel }

// Enabled 实现 Adapter：StoreProtocols["squirrel"].Enabled（C19-2）。
func (a *SquirrelAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolSquirrel)
}

// Render 实现 Adapter：校验文档路径 → 加载 windows 目录 → 匿名可见集（新→旧）→ 组装 RELEASES 文本。
func (a *SquirrelAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, fmt.Errorf("feed: update service unavailable")
	}
	if req.Project == nil {
		return nil, fmt.Errorf("%w: project is required", ErrMissingParam)
	}

	path := strings.Trim(req.Path, "/")
	if path != squirrelDocPath {
		return nil, fmt.Errorf("%w: %q", ErrUnknownPath, req.Path)
	}

	arch := req.Arch
	if arch == "" {
		arch = squirrelDefaultArch
	}
	arch = platform.CanonicalArch(arch)

	const osName = "windows"
	cat, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, osName, arch)
	if err != nil {
		return nil, err
	}
	if _, found := cat.Channel(req.Channel); !found {
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, req.Channel)
	}

	items := AnonymousVisible(cat, cat.Project.CompareEngine, req.Channel, osName, arch)
	var lines int
	var b strings.Builder
	for _, it := range items {
		pkg, ok := resolveStorePackage(ctx, deps, req, it.Line, it.Package)
		if !ok || pkg == nil {
			continue
		}
		sha1Hex, err := a.sha1For(ctx, deps, pkg)
		if err != nil {
			return nil, err
		}
		b.WriteString(sha1Hex)
		b.WriteByte(' ')
		b.WriteString(service.DecoratedPackageName(pkg.SHA256, pkg.FileName))
		b.WriteByte(' ')
		b.WriteString(strconv.FormatInt(pkg.Size, 10))
		b.WriteByte('\n')
		lines++
	}
	if lines == 0 {
		return nil, ErrNoRelease
	}

	return &Response{
		Body:        []byte(b.String()),
		ContentType: squirrelContentType,
		Status:      200,
	}, nil
}

// sha1For 计算产物文件的 40 字符小写 hex SHA-1 摘要。
// 首请求从存储流式计算并按产物 SHA-256 走单飞 + LRU 缓存。
func (a *SquirrelAdapter) sha1For(ctx context.Context, deps *Deps, pkg *update.ArtifactInfo) (string, error) {
	if deps.Storage == nil || pkg.StorageKey == "" {
		return "", fmt.Errorf("feed: storage unavailable for artifact %s", pkg.FileName)
	}
	return a.sha1Cache.GetOrCompute(pkg.SHA256, func() (string, error) {
		rc, err := deps.Storage.Get(ctx, pkg.StorageKey)
		if err != nil {
			return "", fmt.Errorf("feed: read artifact for sha1: %w", err)
		}
		defer rc.Close()
		h := sha1.New()
		if _, err := io.Copy(h, rc); err != nil {
			return "", fmt.Errorf("feed: hash artifact for sha1: %w", err)
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	})
}
