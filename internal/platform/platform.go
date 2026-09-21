// Package platform 提供预置 OS/Arch slug 与别名解析（docs/app-init.md §4.1–4.2）。
// 纯函数，不访问数据库；项目是否单独启用 ipados 由调用方传入矩阵中的 os 集合。
package platform

import (
	"regexp"
	"slices"
	"strings"
)

const (
	// OSMacOS 是 darwin/osx 的规范名。
	OSMacOS = "macos"
	// OSiOS 是 ipados 默认检索目标。
	OSiOS = "ios"
	// OSiPadOS 仅当项目矩阵单独登记时才作为规范 os。
	OSiPadOS = "ipados"

	// ArchX86 是 i386/i686 的规范名。
	ArchX86 = "x86"
	// ArchX86_64 是 amd64 的规范名。
	ArchX86_64 = "x86_64"
	// ArchARMv7 是 armv7l/armv7a 的规范名。
	ArchARMv7 = "armv7"
	// ArchARM64 是 aarch64 的规范名。
	ArchARM64 = "arm64"
)

// osArchSlugRE 与项目 Slug 相同字符集，写入前已转小写。OS/Arch 最短 3 字符。
var osArchSlugRE = regexp.MustCompile(`^[a-z0-9_-]{3,64}$`)

// hwRevSlugRE 是硬件代号独立规则：规范化后允许单字符（如 v1），最长 64。
var hwRevSlugRE = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// PresetOS 是 §4.1 预置操作系统（含 ipados；默认检索仍映射到 ios）。
var PresetOS = []string{
	"windows", "macos", "linux", "freebsd", "openbsd", "netbsd", "dragonfly",
	"solaris", "illumos", "aix", "haiku",
	"android", "ios", "ipados", "tvos", "watchos", "visionos", "harmonyos", "chromeos",
	"embedded", "rtos", "zephyr", "freertos", "wasm", "fuchsia",
}

// PresetArch 是 §4.2 预置处理器架构。
var PresetArch = []string{
	"x86", "x86_64", "arm", "armv6", "armv7", "arm64",
	"riscv32", "riscv64", "loongarch64",
	"mips", "mipsel", "mips64", "mips64el",
	"ppc", "ppc64", "ppc64le", "s390x", "sparc64",
	"wasm32", "thumb", "universal", "any",
}

// OSAliases 将输入别名映射到规范 OS。不含 ipados（见 CanonicalOS）。
var OSAliases = map[string]string{
	"darwin": OSMacOS,
	"osx":    OSMacOS,
}

// ArchAliases 将输入别名映射到规范 Arch。
var ArchAliases = map[string]string{
	"amd64":   ArchX86_64,
	"i386":    ArchX86,
	"i686":    ArchX86,
	"aarch64": ArchARM64,
	"armv7l":  ArchARMv7,
	"armv7a":  ArchARMv7,
}

// CatalogEntry 是管理端 platforms/catalog 的一项。
type CatalogEntry struct {
	Slug    string   `json:"slug"`
	Aliases []string `json:"aliases"`
}

// Normalize 将 OS/Arch 输入转为小写并去掉首尾空白。
func Normalize(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// ValidOSArchSlug 判断规范化后的自定义 os/arch 是否与项目 Slug 字符集一致。
func ValidOSArchSlug(slug string) bool {
	return osArchSlugRE.MatchString(slug)
}

// ValidHwRevSlug 判断规范化后的硬件代号。不套用 OS/Arch 最短 3 字符限制。
func ValidHwRevSlug(slug string) bool {
	return hwRevSlugRE.MatchString(slug)
}

// CanonicalOS 解析 OS 别名供 check / CI bundle 复用。
// darwin/osx → macos；ipados → ios，除非 projectEnabledOS 含 ipados；其余小写原样返回（允许自定义）。
func CanonicalOS(projectEnabledOS []string, raw string) string {
	s := Normalize(raw)
	if s == "" {
		return s
	}
	if s == OSiPadOS {
		if containsFold(projectEnabledOS, OSiPadOS) {
			return OSiPadOS
		}
		return OSiOS
	}
	if canon, ok := OSAliases[s]; ok {
		return canon
	}
	return s
}

// CanonicalOSWrite 用于矩阵写入：darwin/osx 存 macos；ipados 原样保存以启用独立矩阵行。
func CanonicalOSWrite(raw string) string {
	s := Normalize(raw)
	if s == OSiPadOS {
		return OSiPadOS
	}
	if canon, ok := OSAliases[s]; ok {
		return canon
	}
	return s
}

// CanonicalArch 解析 Arch 别名：amd64→x86_64，i386/i686→x86，aarch64→arm64，armv7l/armv7a→armv7。
func CanonicalArch(raw string) string {
	s := Normalize(raw)
	if s == "" {
		return s
	}
	if canon, ok := ArchAliases[s]; ok {
		return canon
	}
	return s
}

// OSCatalog 返回预置 OS 及别名列表（ipados 单独一项，无别名）。
func OSCatalog() []CatalogEntry {
	rev := reverseAliases(OSAliases)
	out := make([]CatalogEntry, 0, len(PresetOS))
	for _, slug := range PresetOS {
		aliases := rev[slug]
		if aliases == nil {
			aliases = []string{}
		}
		out = append(out, CatalogEntry{Slug: slug, Aliases: aliases})
	}
	return out
}

// ArchCatalog 返回预置 Arch 及别名列表。
func ArchCatalog() []CatalogEntry {
	rev := reverseAliases(ArchAliases)
	out := make([]CatalogEntry, 0, len(PresetArch))
	for _, slug := range PresetArch {
		aliases := rev[slug]
		if aliases == nil {
			aliases = []string{}
		}
		out = append(out, CatalogEntry{Slug: slug, Aliases: aliases})
	}
	return out
}

func reverseAliases(aliases map[string]string) map[string][]string {
	out := map[string][]string{}
	for alias, canon := range aliases {
		out[canon] = append(out[canon], alias)
	}
	for canon := range out {
		slices.Sort(out[canon])
	}
	return out
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), want) {
			return true
		}
	}
	return false
}
