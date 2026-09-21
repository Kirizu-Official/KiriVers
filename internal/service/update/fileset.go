package update

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

// FilesetEntry 是规范化后的 fileset 成员：NFC + `/` 路径与目标 Manifest 文件哈希。
type FilesetEntry struct {
	Path           string
	SHA256         string
	Size           int64
	InstallPolicy  string
	IntegrityCheck bool
}

// FilesetResult 是 needed_paths 相对目标 Manifest 的规范化结果。
type FilesetResult struct {
	// Entries 去重后按路径 UTF-8 升序的 Manifest 命中项。
	Entries []FilesetEntry
	// SHA256 是排序行 `{path}\n{file_sha256}\n` 的 SHA-256 hex（AC19）。
	SHA256 string
	// InvalidPaths 归一化失败被忽略的原始条目。
	InvalidPaths []string
	// Unknown 表示存在至少一条合法路径不在目标 Manifest 上（整单 full_package）。
	Unknown bool
	// NeededSum 去重 needed 条目的 Manifest size 合计（D6/D7 分子）。
	NeededSum int64
}

// CanonicalFilesetSHA256 对已规范化、已去重的条目计算 fileset 哈希。
// 行格式 `{nfc_path}\n{manifest_file_sha256}\n`，按路径 UTF-8 排序后送入 SHA-256。
func CanonicalFilesetSHA256(entries []FilesetEntry) string {
	if len(entries) == 0 {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:])
	}
	sorted := append([]FilesetEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	for _, e := range sorted {
		h.Write([]byte(e.Path))
		h.Write([]byte("\n"))
		h.Write([]byte(strings.ToLower(strings.TrimSpace(e.SHA256))))
		h.Write([]byte("\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ResolveNeededFileset 将客户端 needed_paths 对目标 Manifest 规范化。
// 路径先 `\`→`/` 再 NFC（pathutil）；非法计入 InvalidPaths；合法但不在 Manifest → Unknown。
func ResolveNeededFileset(raw []string, manifest []model.ManifestEntry) FilesetResult {
	byPath := make(map[string]model.ManifestEntry, len(manifest))
	for _, e := range manifest {
		byPath[e.Path] = e
	}
	seen := make(map[string]FilesetEntry)
	var invalid []string
	unknown := false
	for _, rawPath := range raw {
		if strings.TrimSpace(rawPath) == "" {
			continue
		}
		norm, err := pathutil.NormalizeAndValidatePath(rawPath)
		if err != nil {
			invalid = append(invalid, strings.TrimSpace(rawPath))
			continue
		}
		m, ok := byPath[norm]
		if !ok {
			unknown = true
			continue
		}
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = FilesetEntry{
			Path:           m.Path,
			SHA256:         strings.ToLower(strings.TrimSpace(m.SHA256)),
			Size:           m.Size,
			InstallPolicy:  m.InstallPolicy,
			IntegrityCheck: m.IntegrityCheck,
		}
	}
	entries := make([]FilesetEntry, 0, len(seen))
	var neededSum int64
	for _, e := range seen {
		entries = append(entries, e)
		neededSum += e.Size
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return FilesetResult{
		Entries:      entries,
		SHA256:       CanonicalFilesetSHA256(entries),
		InvalidPaths: invalid,
		Unknown:      unknown,
		NeededSum:    neededSum,
	}
}

// ManifestUncompressedSum 返回 Manifest 全部条目 size 之和（含 KEEP），即打包前总体积（D7 分母）。
func ManifestUncompressedSum(manifest []model.ManifestEntry) int64 {
	var sum int64
	for _, e := range manifest {
		sum += e.Size
	}
	return sum
}

// ExceedsUncompressedGates 在入队前同时做 D6 硬顶与 D7 70%（均用未压缩 Manifest size，永不读 zip Size）。
func ExceedsUncompressedGates(neededSum, fullManifestSum, maxBytes int64) bool {
	if maxBytes > 0 && neededSum >= maxBytes {
		return true
	}
	if fullManifestSum > 0 && float64(neededSum) >= PatchPackageMaxSizeRatio*float64(fullManifestSum) {
		return true
	}
	return false
}

// FilesetEntriesFromManifest 把 Manifest 子集投影为 FilesetEntry（预热 needed 集合）。
func FilesetEntriesFromManifest(entries []model.ManifestEntry) []FilesetEntry {
	out := make([]FilesetEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if _, ok := seen[e.Path]; ok {
			continue
		}
		seen[e.Path] = struct{}{}
		out = append(out, FilesetEntry{
			Path:           e.Path,
			SHA256:         strings.ToLower(strings.TrimSpace(e.SHA256)),
			Size:           e.Size,
			InstallPolicy:  e.InstallPolicy,
			IntegrityCheck: e.IntegrityCheck,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// DynamicPackIdempotencyKey 是客户端动态打包 Job 幂等键：line + hw + fileset。
func DynamicPackIdempotencyKey(lineID string, hw, filesetSHA string) string {
	return "dynamic-pack:" + lineID + ":" + strings.TrimSpace(hw) + ":" + strings.ToLower(strings.TrimSpace(filesetSHA))
}
