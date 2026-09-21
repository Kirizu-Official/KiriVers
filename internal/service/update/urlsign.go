package update

import (
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// 本文件定义私有存储短时签名下载 URL 的装配契约（docs/app-init.md §13.7，
// C15-1/C15-2）。
//
// 设计要点：
//   - 签名在本服务下载 handler 层统一实现（LocalFS/S3 同构，不依赖 S3 Presign）；
//   - 稳定路径与文件名不变，签名只追加在 query（?exp=&sig=），由实现方保证；
//   - 仅私有项目（StorageVisibility=private）签名：Service 包装层在加载目录后
//     按项目可见性把签名器注入 Input（纯函数不触库，签名决策随快照而定）；
//   - 公开项目（public）签名器为 nil，URL 原样返回——既有测试与直链行为不变；
//   - 绝对 http(s) 公共对象 URL（S3 直链）不加本节点 HMAC；
//   - URL/签名永不进入 ETag 目录快照（ETag 输入不变），但会进入 §12.1 载荷
//    （载荷含 package_url，客户端可对完整下载地址验签）。

// URLSigner 为私有项目下载路径附加短时签名 query（§13.7）。实现方：
// pkg/urlsign.Signer（生产）与各测试的 stub。
type URLSigner interface {
	// SignDownload 对稳定下载路径 path 追加 exp/sig 查询参数并返回完整 URL。
	// ttlSeconds <= 0 时实现方使用实例默认 TTL（§13.7 默认 3600s = 1h）。
	SignDownload(path string, ttlSeconds int) string
}

// signingFor 按目录快照的项目可见性返回生效的签名装配。
func (s *Service) signingFor(cat *Catalog) urlSigning {
	u := urlSigning{}
	if s != nil {
		u.objectURL = s.objectURL
		u.localProxy = s.localProxy
	}
	if s == nil || s.signer == nil || cat.Project.StorageVisibility != model.StorageVisibilityPrivate {
		return u
	}
	u.signer = s.signer
	u.ttl = cat.Project.SignedURLTTLSeconds
	return u
}

// urlSigning 捆绑签名器、项目 TTL 与可选公共对象 URL。
type urlSigning struct {
	signer     URLSigner
	ttl        int
	objectURL  func(slug, sha256, storageKey string) string
	localProxy bool
}

// sign 对相对下载路径签名；绝对 http(s) URL 原样返回。
func (u urlSigning) sign(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if u.signer == nil {
		return path
	}
	return u.signer.SignDownload(path, u.ttl)
}

func (u urlSigning) artifactURL(slug, sha256, storageKey string) string {
	raw := packageURL(slug, sha256)
	if u.objectURL != nil {
		if alt := u.objectURL(slug, sha256, storageKey); alt != "" {
			raw = alt
		}
	}
	return u.sign(raw)
}
