// Package semver 在 Masterminds SemVer 2.0 之上提供入库规范化：去掉 v 前缀与 +build。
package semver

import (
	"fmt"
	"strings"

	msemver "github.com/Masterminds/semver/v3"
)

// Canonical 将输入规范为不含 v、不含 build metadata 的 SemVer 2.0 字符串。
// 非法输入返回错误，调用方不得自动改写成其它版本。
func Canonical(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	v, err := msemver.StrictNewVersion(s)
	if err != nil {
		return "", fmt.Errorf("invalid semver %q: %w", raw, err)
	}
	return v.String(), nil
}

// Compare 按 SemVer 2.0 优先级比较两个已规范化版本。
func Compare(a, b string) (int, error) {
	va, err := msemver.StrictNewVersion(a)
	if err != nil {
		return 0, err
	}
	vb, err := msemver.StrictNewVersion(b)
	if err != nil {
		return 0, err
	}
	return va.Compare(vb), nil
}
