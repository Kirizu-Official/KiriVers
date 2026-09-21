package buildinfo

import (
	"regexp"
	"testing"
)

// platformRe 是 Platform() 的期望形态：goos/goarch（如 windows/amd64、linux/386）。
var platformRe = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+$`)

// TestFallbackWithoutInjection 覆盖未注入分支（AC1 的廉价证明）：
// 本地 `go build .` 与常规 `go test ./...` 都不带 -X，此时三值必须是可辨识回退值，
// 不得空串、不得 panic。显式注入构建（见 TestInjectedValuesVerbatim）跳过本用例。
func TestFallbackWithoutInjection(t *testing.T) {
	if version != "" || commit != "" || buildTime != "" {
		t.Skipf("this build carries injected values (version=%q commit=%q buildTime=%q)", version, commit, buildTime)
	}

	if got := Version(); got != FallbackVersion {
		t.Errorf("Version() = %q, want fallback %q", got, FallbackVersion)
	}
	if got := BuildTime(); got != FallbackBuildTime {
		t.Errorf("BuildTime() = %q, want fallback %q (build time must not be faked)", got, FallbackBuildTime)
	}
	// commit 取决于工具链是否 stamp VCS：本地 go build 给短 SHA（脏树带 -dirty），
	// -buildvcs=false 的发布构建取不到才回退 unknown。两种都不是空串。
	if got := Commit(); got == "" {
		t.Errorf("Commit() = %q, want short SHA or %q", got, FallbackCommit)
	}
}

// TestInjectedValuesVerbatim 是 -X 符号路径的证据：只有用
// `go test -ldflags "-X <pkg>.Version=… -X <pkg>.Commit=… -X <pkg>.BuildTime=…" ./internal/buildinfo`
// 编译时才有断言可做（Go 对拼错的 -X 目标是静默 no-op，故必须逐字比对）；
// 常规构建三个原始变量皆空，本用例自动跳过。
func TestInjectedValuesVerbatim(t *testing.T) {
	if version == "" && commit == "" && buildTime == "" {
		t.Skip("no -X injection in this build")
	}
	checks := []struct {
		name     string
		injected string
		got      string
	}{
		{"Version", version, Version()},
		{"Commit", commit, Commit()},
		{"BuildTime", buildTime, BuildTime()},
	}
	for _, c := range checks {
		if c.injected == "" {
			// 只注入部分字段时，未注入的那侧仍须走回退，不能被空串覆盖。
			continue
		}
		if c.got != c.injected {
			t.Errorf("%s() = %q, want injected %q", c.name, c.got, c.injected)
		}
	}
}

// TestRuntimeFacts 校验不注入的两个字段：Go 版本与平台来自 runtime，恒非空且形态稳定。
func TestRuntimeFacts(t *testing.T) {
	if got := GoVersion(); got == "" {
		t.Error("GoVersion() = \"\", want runtime.Version()")
	}
	if got := Platform(); !platformRe.MatchString(got) {
		t.Errorf("Platform() = %q, want form goos/goarch", got)
	}
}

// TestAccessorsAreStable 确认一次性解析：重复调用返回同一组值（不随时间漂移）。
func TestAccessorsAreStable(t *testing.T) {
	first := Version() + "|" + Commit() + "|" + BuildTime()
	for i := 0; i < 8; i++ {
		if got := Version() + "|" + Commit() + "|" + BuildTime(); got != first {
			t.Fatalf("accessor drift: %q then %q", first, got)
		}
	}
}
