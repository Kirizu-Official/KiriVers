package update

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

// integrity 阈值常量。
const (
	// IntegrityIncludeFileURLsMaxPage 仅当全部文件条数 ≤ 该值才附带逐文件 url；
	// 超过阈值一律只给顶层 full_package_url。
	IntegrityIncludeFileURLsMaxPage = 16
)

// hash_algo 取值（§8：sha256 默认 / md5 / both）。
const (
	HashAlgoSHA256 = "sha256"
	HashAlgoMD5    = "md5"
	HashAlgoBoth   = "both"
)

// IntegrityInput 是 GET versions/:version/integrity 的请求参数
// （OS/Arch 已由 HTTP 层规范化）。
type IntegrityInput struct {
	// Version 是路径参数中的版本引用（整数或 SemVer）。
	Version string
	// OS / Arch 是已规范化的请求平台（HTTP 层经 platform.CanonicalOS/Arch）。
	OS, Arch string
	// HashAlgo 决定条目输出哪些哈希；空视为 sha256；非法值 → ErrInvalidQuery。
	HashAlgo string
	// Compact 为 true 时省略 md5 字段（§8）；显式请求 hash_algo=md5 时仍保留主哈希。
	Compact bool
	// IncludeFileURLs 为 true 且全部文件 ≤IntegrityIncludeFileURLsMaxPage 时附逐文件 url。
	IncludeFileURLs bool
	// HwRev 可选硬件代号（匹配 full 包变体）。
	HwRev string
	// Channel 可选；传入则必须等于该 Version.ChannelSlug（C09-10）。
	Channel string

	// signing 由 Service 包装层按项目可见性注入（私有项目签名，§13.7 / C15-1）。
	signing urlSigning
}

// IntegrityFile 是文件元数据条目（§8：路径、大小、哈希、install_policy、
// integrity_check；KEEP 行 integrity_check=false）。
type IntegrityFile struct {
	Path           string `json:"path"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256,omitempty"`
	MD5            string `json:"md5,omitempty"`
	InstallPolicy  string `json:"install_policy"`
	IntegrityCheck bool   `json:"integrity_check"`
	URL string `json:"url,omitempty"`
}

// IntegrityVolume 是发布时预切的固定分卷（§7.5.5）。分卷集合对所有客户端相同，
// 由发布流程落库后填充；当前模型尚无分卷载体，字段恒缺省。
type IntegrityVolume struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// IntegrityResponse 是 200 响应体（§8 / §10.2 / C09-1 / C09-9）。
type IntegrityResponse struct {
	VersionInteger *int64  `json:"version_integer"`
	VersionSemver  *string `json:"version_semver"`
	Channel        string  `json:"channel"`
	PackageType    string  `json:"package_type"`
	RootHash       string  `json:"root_hash"`
	// FullPackageURL 指向该线 kind=full 产物（稳定文件名路由 /packages/{sha256}），
	// 修复下载恒走这一个归档（§7.4.5 / §8）。
	FullPackageURL string            `json:"full_package_url"`
	FileName       string            `json:"file_name"`
	Size           int64             `json:"size"`
	SHA256         string            `json:"sha256"`
	Volumes        []IntegrityVolume `json:"volumes,omitempty"`
	Files          []IntegrityFile   `json:"files"`
	// Signature 与 check 同一套算法与密钥（§12.1）：
	// 载荷 = 双号 \n root_hash \n full_package_url \n size \n sha256。
	Signature string `json:"signature,omitempty"`
}

// IntegrityResult 是 integrity 编排结果：HTTP 层据此写状态码、ETag、缓存头与 body。
type IntegrityResult struct {
	Body         *IntegrityResponse
	ETag         string
	CacheControl string
	Vary         []string
	// Private 标记私有存储项目：HTTP 层据此跳过 304 短路并恒回新鲜 200
	//（每次带新签名 URL，§13.7 / C15-1）。
	Private bool
}

// Integrity 执行完整性查询（HTTP 层便捷入口）：加载目录后调用纯函数版 Integrity。
func (s *Service) Integrity(ctx context.Context, projectID uuid.UUID, os, arch string, in IntegrityInput) (*IntegrityResult, error) {
	cat, err := s.loadCatalog(ctx, projectID, os, arch)
	if err != nil {
		return nil, err
	}
	// 私有项目注入短时签名装配（§13.7 / C15-1）；公开项目零值原样返回。
	in.signing = s.signingFor(cat)
	return Integrity(ctx, cat, s.details, in)
}

// Integrity 是纯函数编排：状态闸 → channel 校验 → 平台线解析 → 全量文件组装 →
// 签名注入与 ETag/缓存头。不触 gin；Manifest 经 LineDetailSource 按需读取。
//
// 状态闸（C09-2 / §8）：未知 → VERSION_NOT_FOUND；Draft → VERSION_NOT_VISIBLE；
// Revoked → VERSION_REVOKED（响应无任何下载 URL，错误体天然满足）；Deprecated 开放；
// 无就绪 (os,arch) 线 → VERSION_LINE_NOT_FOUND。
func Integrity(ctx context.Context, cat *Catalog, details LineDetailSource, in IntegrityInput) (*IntegrityResult, error) {
	if cat == nil {
		return nil, ErrInvalidQuery
	}
	if details == nil {
		return nil, ErrLineDetailsUnavailable
	}

	// 1. 解析版本（引用非法视作未知，不泄露引擎差异）。
	ref, ok := parseVersionRef(in.Version)
	if !ok {
		return nil, ErrVersionNotFound
	}
	vs := findVersionState(cat, ref)
	if vs == nil {
		return nil, ErrVersionNotFound
	}

	// 2. 状态闸（§8：Draft 不对客户端开放；Deprecated 开放；Revoked 409 无 URL）。
	switch vs.Version.Status {
	case model.VersionStatusDraft:
		return nil, ErrVersionNotVisible
	case model.VersionStatusRevoked:
		return nil, ErrVersionRevoked
	}

	// 3. channel 校验（C09-10）：传入则必须与 Version 渠道一致。
	if ch := strings.TrimSpace(in.Channel); ch != "" && ch != vs.Version.ChannelSlug {
		return nil, ErrChannelConflict
	}

	// 4. 平台线：不存在或未就绪（pending/failed/disabled/yanked）→ VERSION_LINE_NOT_FOUND。
	line := vs.Line(in.OS, in.Arch)
	if line == nil || line.Status != model.VersionLineStatusReady {
		return nil, ErrVersionLineNotFound
	}
	if line.PacksReadyAt == nil {
		return nil, ErrVersionNotVisible
	}

	// 5. full 包变体（与下载端点同一匹配函数；无兼容变体视作线不存在）。
	pkg := matchHwVariant(cat, cat.Matrix, line, strings.TrimSpace(in.HwRev))
	if pkg == nil {
		return nil, ErrVersionLineNotFound
	}

	// 6. hash_algo 校验（非法 → 400，ErrInvalidQuery）。
	algo := HashAlgoSHA256
	if s := strings.TrimSpace(strings.ToLower(in.HashAlgo)); s != "" {
		algo = s
	}
	switch algo {
	case HashAlgoSHA256, HashAlgoMD5, HashAlgoBoth:
	default:
		return nil, fmt.Errorf("%w: hash_algo must be sha256, md5 or both", ErrInvalidQuery)
	}

	// 7. 全量文件组装（无分页）。
	files, err := buildIntegrityFiles(ctx, cat, details, line, pkg, algo, in)
	if err != nil {
		return nil, err
	}

	// full_package_url：私有项目附加短时签名（§13.7 / C15-1）；签名先于
	// §12.1 载荷拼装，客户端可对完整下载地址验签。
	fullURL := in.signing.artifactURL(cat.Project.Slug, pkg.SHA256, pkg.StorageKey)
	resp := &IntegrityResponse{
		VersionInteger: vs.Version.VersionInteger,
		VersionSemver:  vs.Version.VersionSemverCanonical,
		Channel:        vs.Version.ChannelSlug,
		PackageType:    matrixOrEmpty(cat.Matrix).PackageType,
		RootHash:       line.RootHash,
		FullPackageURL: fullURL,
		FileName:       pkg.FileName,
		Size:           pkg.Size,
		SHA256:         pkg.SHA256,
		Files:          files,
	}

	// 9. 签名注入（§12.1，与 check 同一套载荷与密钥）。
	if cat.Project.SigningPrivateKey != "" {
		payload := signature.BuildCheckPayload(
			int64OrEmpty(vs.Version.VersionInteger),
			stringOrEmpty(vs.Version.VersionSemverCanonical),
			line.RootHash,
			fullURL,
			strconv.FormatInt(pkg.Size, 10),
			pkg.SHA256,
		)
		sig, err := signature.SignPayload(cat.Project.SigningAlgo, cat.Project.SigningPrivateKey, payload)
		if err != nil {
			return nil, err
		}
		resp.Signature = sig
	}

	// 10. ETag = 线 RootHash（强 ETag；产物变更必然改 RootHash 或换线）。
	// 单文件线 RootHash 留空（模型约定），退化为全包 SHA-256 维持 304 语义。
	// ETag 输入不变：URL/签名不入快照；私有项目改用 private, no-store 并由
	// HTTP 层跳过 304（C15-1）。
	etagSource := line.RootHash
	if etagSource == "" {
		etagSource = pkg.SHA256
	}
	private := cat.Project.StorageVisibility == model.StorageVisibilityPrivate
	cacheControl := cacheControlPublic(cat.Project.CacheSMaxageSeconds)
	if private {
		cacheControl = cacheControlNoStore
	}
	return &IntegrityResult{
		Body:         resp,
		ETag:         `"` + etagSource + `"`,
		CacheControl: cacheControl,
		Vary:         buildVary(cat.Project),
		Private:      private,
	}, nil
}

// buildIntegrityFiles 组装全部文件条目（一次返回）。
//
//   - 多文件线：读取 Manifest，按 Path 字节升序确定性排序（与 Root Hash 计算一致）；
//   - 单文件线（§10.2）：files 恒一项，即全包哈希行，不走 Manifest；
//   - include_file_urls=true 且全部文件 ≤IntegrityIncludeFileURLsMaxPage 时，
//     按 Path 匹配 kind=file 产物附逐文件 url，匹配不到的条目省略 url。
func buildIntegrityFiles(ctx context.Context, cat *Catalog, details LineDetailSource, line *LineState, pkg *ArtifactInfo, algo string, in IntegrityInput) ([]IntegrityFile, error) {
	if matrixOrEmpty(cat.Matrix).PackageType == model.PackageTypeSingleFile {
		entry := integrityFileFromFullPkg(pkg, algo, in.Compact)
		return []IntegrityFile{entry}, nil
	}

	detail, err := details.LineDetails(ctx, line.ID)
	if err != nil {
		return nil, err
	}

	entries := detail.Manifest
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	fileArts := detail.Files
	attachURLs := in.IncludeFileURLs && len(entries) <= IntegrityIncludeFileURLsMaxPage && len(fileArts) > 0
	fileURLByName := map[string]string{}
	if attachURLs {
		for i := range fileArts {
			if fileArts[i].Path != "" {
				fileURLByName[fileArts[i].Path] = in.signing.artifactURL(cat.Project.Slug, fileArts[i].SHA256, fileArts[i].StorageKey)
			}
		}
	}

	files := make([]IntegrityFile, 0, len(entries))
	for _, e := range entries {
		f := IntegrityFile{
			Path:           e.Path,
			Size:           e.Size,
			InstallPolicy:  e.InstallPolicy,
			IntegrityCheck: e.IntegrityCheck,
		}
		wantSHA := algo == HashAlgoSHA256 || algo == HashAlgoBoth
		wantMD5 := (algo == HashAlgoMD5 || algo == HashAlgoBoth) && !in.Compact
		if wantSHA {
			f.SHA256 = e.SHA256
		}
		if wantMD5 {
			f.MD5 = e.MD5
		}
		if u, ok := fileURLByName[e.Path]; ok {
			f.URL = u
		}
		files = append(files, f)
	}
	return files, nil
}

// integrityFileFromFullPkg 把 kind=full 产物投影为单文件线的唯一 files 条目。
// 哈希输出与多文件条目同规则：按 hash_algo 取舍，compact=true 省 md5。
func integrityFileFromFullPkg(pkg *ArtifactInfo, algo string, compact bool) IntegrityFile {
	f := IntegrityFile{
		Path:           pkg.FileName,
		Size:           pkg.Size,
		InstallPolicy:  model.InstallPolicyOverwrite,
		IntegrityCheck: true,
	}
	if algo == HashAlgoSHA256 || algo == HashAlgoBoth {
		f.SHA256 = pkg.SHA256
	}
	if (algo == HashAlgoMD5 || algo == HashAlgoBoth) && !compact {
		f.MD5 = pkg.MD5
	}
	return f
}
