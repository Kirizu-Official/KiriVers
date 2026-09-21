package service

import (
	"path/filepath"
	"regexp"
	"strings"
)

// packageRefRE 解析下载 :ref：前导 64 hex（大小写不敏感）+ 可选装饰后缀（.{ext} / .blockmap）。
var packageRefRE = regexp.MustCompile(`(?i)^([0-9a-f]{64})(.*)$`)

// ParsePackageRef 从 /packages/:ref 取出小写 SHA-256 与装饰后缀。非法则 ok=false。
func ParsePackageRef(ref string) (sha256Hex, suffix string, ok bool) {
	m := packageRefRE.FindStringSubmatch(strings.TrimSpace(ref))
	if m == nil {
		return "", "", false
	}
	return strings.ToLower(m[1]), m[2], true
}

// PackageRefSuffix 是 D7 装饰扩展名（含前导点）；无扩展名时为空。
func PackageRefSuffix(fileName string) string {
	name := strings.TrimSpace(fileName)
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz"} {
		if strings.HasSuffix(lower, ext) {
			return name[len(name)-len(ext):]
		}
	}
	return filepath.Ext(name)
}

// DecoratedPackageName 是商店协议本地文件名：{sha256}{ext}（electron path / Squirrel RELEASES）。
func DecoratedPackageName(sha256, fileName string) string {
	sha := strings.ToLower(strings.TrimSpace(sha256))
	return sha + PackageRefSuffix(fileName)
}

func isBlockmapSuffix(suffix string) bool {
	return strings.HasSuffix(strings.ToLower(suffix), ".blockmap")
}

func isBlockmapFileName(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".blockmap")
}
