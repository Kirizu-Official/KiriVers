package store

import (
	"context"
	"path"
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// listingIdentifiers 读取 listing 协议包名袋；无 listing 时返回空 map。
func listingIdentifiers(req Request) map[string]string {
	if req.Listing == nil || req.Listing.Identifiers == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(req.Listing.Identifiers))
	for k, v := range req.Listing.Identifiers {
		out[k] = v
	}
	return out
}

// listingSlug 返回 feed URL 中的 listing slug（测试直调 Adapter 时缺省 default）。
func listingSlug(req Request) string {
	if req.Listing != nil && req.Listing.Slug != "" {
		return req.Listing.Slug
	}
	return "default"
}

func listingPin(listing *model.StoreListing, dim string) string {
	if listing == nil {
		return ""
	}
	switch dim {
	case "os":
		if listing.OS != nil {
			return strings.TrimSpace(*listing.OS)
		}
	case "arch":
		if listing.Arch != nil {
			return strings.TrimSpace(*listing.Arch)
		}
	case "channel":
		if listing.Channel != nil {
			return strings.TrimSpace(*listing.Channel)
		}
	}
	return ""
}

// resolveStorePackage 按 listing 完整包选择器取出该版本的 enclosure 产物。
// 缺失文件时 ok=false，调用方跳过该版本；不得 500。
func resolveStorePackage(ctx context.Context, deps *Deps, req Request, line *update.LineState, full *update.ArtifactInfo) (*update.ArtifactInfo, bool) {
	if full == nil {
		return nil, false
	}
	src := model.PackageSourceLineFull
	manifestPath := ""
	if req.Listing != nil {
		if req.Listing.PackageSource != "" {
			src = req.Listing.PackageSource
		}
		manifestPath = strings.TrimSpace(req.Listing.ManifestPath)
	}
	if src != model.PackageSourceManifestPath {
		if pkg := matchStoreFull(line, full); pkg != nil {
			return pkg, true
		}
		// 单文件线没有哈希根 zip，也没有 store_full：继续用 kind=full（安装包本身）。
		// 多文件线（RootHash 非空）不得把哈希根 kind=full 当作 line_full（AC16）。
		if line == nil || strings.TrimSpace(line.RootHash) == "" {
			return full, true
		}
		return nil, false
	}
	if manifestPath == "" || line == nil || deps == nil || deps.Updates == nil {
		return nil, false
	}
	detail, err := deps.Updates.LineDetails(ctx, line.ID)
	if err != nil || detail == nil {
		return nil, false
	}
	file := matchManifestPathFile(detail, manifestPath)
	if file == nil {
		return nil, false
	}
	out := *full
	out.FileName = file.FileName
	if out.FileName == "" {
		out.FileName = path.Base(manifestPath)
	}
	out.SHA256 = file.SHA256
	out.MD5 = file.MD5
	out.Size = file.Size
	out.StorageKey = file.StorageKey
	out.SHA512 = ""
	out.ContentType = service.DetectContentType(out.FileName)
	return &out, true
}

// matchStoreFull 按 hw 变体选取 kind=store_full；无线上 store_full 时返回 nil。
func matchStoreFull(line *update.LineState, full *update.ArtifactInfo) *update.ArtifactInfo {
	if line == nil || len(line.StoreFullPkgs) == 0 {
		return nil
	}
	hw := ""
	if full != nil && full.HwRev != nil {
		hw = strings.TrimSpace(*full.HwRev)
	}
	for i := range line.StoreFullPkgs {
		p := &line.StoreFullPkgs[i]
		pHw := ""
		if p.HwRev != nil {
			pHw = strings.TrimSpace(*p.HwRev)
		}
		if pHw == hw {
			return p
		}
	}
	return &line.StoreFullPkgs[0]
}

func matchManifestPathFile(detail *update.LineDetail, manifestPath string) *update.FileArtifactInfo {
	want := strings.TrimPrefix(path.Clean(strings.ReplaceAll(manifestPath, "\\", "/")), "/")
	base := path.Base(want)
	for i := range detail.Files {
		f := &detail.Files[i]
		got := strings.TrimPrefix(path.Clean(strings.ReplaceAll(f.Path, "\\", "/")), "/")
		if got != "" && got == want {
			return f
		}
		if f.FileName == want || f.FileName == base {
			return f
		}
	}
	var sha string
	for _, e := range detail.Manifest {
		got := strings.TrimPrefix(path.Clean(strings.ReplaceAll(e.Path, "\\", "/")), "/")
		if got == want || e.Path == want || path.Base(e.Path) == base {
			sha = e.SHA256
			break
		}
	}
	if sha == "" {
		return nil
	}
	for i := range detail.Files {
		if strings.EqualFold(detail.Files[i].SHA256, sha) {
			return &detail.Files[i]
		}
	}
	return nil
}

func enclosureURL(signing urlSigning, slug, sha256, storageKey string) string {
	return signing.artifactURL(slug, sha256, storageKey)
}
