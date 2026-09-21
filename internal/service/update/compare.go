package update

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/semver"
)

// 版本引用解析说明：
//
// 本文件实现的 parseVersionRef 与 internal/service.ParseVersionRef 语义一致
// （整段十进制正整数走 integer 路径，"010"="10"；否则 SemVer 2.0 规范化，
// 去 v、去 +build）。之所以不复用 service 包：CatalogLoader 的 GORM 实现位于
// internal/repository（task design §B2），而 internal/service 依赖
// internal/repository；若 update → service 会形成
// repository → update → service → repository 的导入环。
// 两侧语义由外部测试包 parse_parity_test.go 逐例对照锁定。

var intVersionRefRegex = regexp.MustCompile(`^[0-9]+$`)

// versionRef 是解析后的版本引用：整数或规范 SemVer 二选一。
type versionRef struct {
	isInteger bool
	integer   int64
	semver    string
}

// parseVersionRef 解析 API 传入的 current_version / min_source_version /
// minimum_supported_version 字符串。
// 解析失败（空、非法 SemVer、非正整数）返回 ok=false，由调用方决定语义
// （current_version → ENGINE_MISMATCH；floor 类配置 → 视为未配置）。
func parseVersionRef(raw string) (versionRef, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return versionRef{}, false
	}
	if intVersionRefRegex.MatchString(s) {
		val, err := strconv.ParseInt(s, 10, 64)
		if err != nil || val <= 0 {
			return versionRef{}, false
		}
		return versionRef{isInteger: true, integer: val}, true
	}
	canon, err := semver.Canonical(s)
	if err != nil {
		return versionRef{}, false
	}
	return versionRef{semver: canon}, true
}

// cmpKey 是某个对象在项目 compare_engine 下的比较键。
// 同一引擎下两键恒为同一形态（isInteger 一致）才可比较；
// 形态不齐（例如 semver 引擎下的整数 floor）视为不可比较。
type cmpKey struct {
	isInteger bool
	integer   int64
	semver    string
}

// versionKey 提取 Version 记录在项目引擎下的比较键。
// integer 引擎缺 version_integer、semver 引擎缺规范 SemVer 时 ok=false
// （发布闸门已保证 Published 版本齐备，此处兜底）。
func versionKey(engine string, v *model.Version) (cmpKey, bool) {
	if engine == model.CompareEngineInteger {
		if v.VersionInteger == nil {
			return cmpKey{}, false
		}
		return cmpKey{isInteger: true, integer: *v.VersionInteger}, true
	}
	if v.VersionSemverCanonical == nil || *v.VersionSemverCanonical == "" {
		return cmpKey{}, false
	}
	return cmpKey{semver: *v.VersionSemverCanonical}, true
}

// refKey 把解析后的引用转成引擎下的比较键；引擎与引用形态不齐时 ok=false
// （例如 semver 引擎收到整数 floor → 该配置按未配置处理，不参与比较）。
func refKey(engine string, ref versionRef) (cmpKey, bool) {
	if engine == model.CompareEngineInteger {
		if !ref.isInteger {
			return cmpKey{}, false
		}
		return cmpKey{isInteger: true, integer: ref.integer}, true
	}
	if ref.isInteger {
		return cmpKey{}, false
	}
	return cmpKey{semver: ref.semver}, true
}

// compareKeys 比较同一形态的两个比较键，返回 -1/0/1。形态不同返回 0 且 ok=false。
func compareKeys(a, b cmpKey) (int, bool) {
	if a.isInteger != b.isInteger {
		return 0, false
	}
	if a.isInteger {
		switch {
		case a.integer < b.integer:
			return -1, true
		case a.integer > b.integer:
			return 1, true
		}
		return 0, true
	}
	c, err := semver.Compare(a.semver, b.semver)
	if err != nil {
		// 快照内的规范 SemVer 均已在写入时校验，此处兜底不参与比较。
		return 0, false
	}
	return c, true
}

// CompareVersions 按项目 compare_engine 比较两个版本记录（c > 0 表示 a 更新）。
// 供商店协议 feed 层对可见版本集做「新→旧」排序复用，避免在 update 包外
// 重新实现引擎语义；任一侧缺引擎要求的比较键时 ok=false（调用方跳过）。
func CompareVersions(engine string, a, b *model.Version) (c int, ok bool) {
	ka, okA := versionKey(engine, a)
	kb, okB := versionKey(engine, b)
	if !okA || !okB {
		return 0, false
	}
	return compareKeys(ka, kb)
}

// osVersionAtLeast 判断客户端 os_version 是否不低于最低要求 minOS。
// 按点分字段逐段比较：数字段按数值，非数字段按字符串；任一侧存在无法
// 解析为版本形态的输入（空、非点分数字开头）时不阻挡（返回 true），
// 避免脏输入造成误 409。
func osVersionAtLeast(osVersion, minOS string) bool {
	osVersion, minOS = strings.TrimSpace(osVersion), strings.TrimSpace(minOS)
	if osVersion == "" || minOS == "" {
		return true
	}
	if !isDottedNumeric(osVersion) || !isDottedNumeric(minOS) {
		return true
	}
	a := strings.Split(osVersion, ".")
	b := strings.Split(minOS, ".")
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		ai, bi := segmentAt(a, i), segmentAt(b, i)
		if ai != bi {
			return ai > bi
		}
	}
	return true
}

// segmentAt 取点分版本的第 i 段，缺段按 0。
func segmentAt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(parts[i]))
	if err != nil {
		return 0
	}
	return v
}

// isDottedNumeric 形如 "10"、"10.0"、"11.2.1" 的纯点分数字。
func isDottedNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			return false
		}
		for _, r := range seg {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
