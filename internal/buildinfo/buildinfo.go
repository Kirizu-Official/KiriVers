// Package buildinfo 提供本进程二进制的编译期构建信息（版本 / commit / 构建时间）。
//
// 用途：管理台"关于"界面与运维排障需要回答"这台实例跑的是哪个构建"。信息在链接期
// 由 `-ldflags "-X <符号路径>=<值>"` 注入，运行期只读；三处构建入口
// （dev/build/kirivers_build/build.py、dev/build/Dockerfile、CI 各 job）共用同一组
// 符号路径。改下面三个注入变量的名字，必须同步改 `build.py` 的 `_BUILDINFO_PKG` +
// `_STAMP_VARS` 与 `dev/build/Dockerfile` 的 `-X` 串：cmd/link 对拼错的 `-X` 目标
// 是静默忽略，不会报错。
//
// 回退原则（关键）：老产物或本地 `go build .` 根本没有注入，此时必须给出**可辨识**的
// 回退值，不得返回空串、不得 panic、更不得用进程启动时间伪造编译时间。
// CGO 状态不经注入：它由 `CGO_ENABLED` 决定的构建标签天然携带，见 cgo.go / nocgo.go。
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// 未注入时的回退值。unknown 表示"这条构建链路没有提供该字段"，与 dev（开发态构建）
// 语义区分：前者是信息缺失，后者是构建来源已知但未标版。
const (
	// unknown 是"字段确实不可得"的统一表达（commit / build_time 共用）。
	unknown = "unknown"

	FallbackVersion   = "dev"
	FallbackCommit    = unknown
	FallbackBuildTime = unknown

	// commitShortLen 与 `git rev-parse --short` 的常见输出宽度一致，
	// 用于把 VCS stamp 里的完整 SHA 裁成与注入值同形态的短 SHA。
	commitShortLen = 7
)

// version/commit/buildTime 是注入目标：包级 string 变量，链接期由 `-X` 写值。
// 保持未导出，对外只暴露下面的访问器函数，避免调用方各自判空。
var (
	version   string
	commit    string
	buildTime string
)

// info 是一次性解析后的构建信息快照。
type info struct {
	version   string
	commit    string
	buildTime string
}

// cached 把回退解析（debug.ReadBuildInfo 遍历 build settings）收敛为进程内一次，
// 之后每个请求只是字段读取，无锁无分配（sync.OnceValue 内部只有首次才慢路径）。
var cached = sync.OnceValue(load)

// load 解析三值：注入优先，缺失时按 design §2 的回退规则补齐。
func load() info {
	bi, _ := debug.ReadBuildInfo() // nil 表示构建信息不可得（如裁剪过的二进制），走纯回退。
	return info{
		version:   fallback(version, moduleVersion(bi)),
		commit:    fallback(commit, vcsRevision(bi)),
		buildTime: fallback(buildTime, FallbackBuildTime),
	}
}

// fallback 在注入值为空时采用回退值；回退值为空（防御性：调用方给不出回退语义时）
// 一律退回 unknown，绝不退回空串。
func fallback(injected, fallbackValue string) string {
	if injected != "" {
		return injected
	}
	if fallbackValue != "" {
		return fallbackValue
	}
	return unknown
}

// moduleVersion 取主模块版本作为版本回退：本地未打 tag 的构建是 "(devel)" 或空串，
// 一律映射为 dev；只有以版本化模块方式构建时才会拿到形如 v0.1.0 的值（去掉 v 前缀，
// 与 KIRIVERS_VERSION 的无前缀约定一致）。
func moduleVersion(bi *debug.BuildInfo) string {
	if bi == nil {
		return FallbackVersion
	}
	switch v := bi.Main.Version; v {
	case "", "(devel)":
		return FallbackVersion
	default:
		return strings.TrimPrefix(v, "v")
	}
}

// vcsRevision 从 VCS stamp 还原短 SHA：本地 `go build .`（默认 -buildvcs=auto）会带上
// vcs.revision，工作树有未提交改动时 vcs.modified=true，追加 -dirty 尾巴。
// 发布链路显式使用 -buildvcs=false，这里必然取不到，只能依赖 -X 注入。
func vcsRevision(bi *debug.BuildInfo) string {
	if bi == nil {
		return FallbackCommit
	}
	revision := ""
	modified := false
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return FallbackCommit
	}
	short := revision
	if len(short) > commitShortLen {
		short = short[:commitShortLen]
	}
	if modified {
		short += "-dirty"
	}
	return short
}

// Version 返回本次构建的版本号（不含 v 前缀）；未注入时为 dev。
func Version() string { return cached().version }

// Commit 返回本次构建的提交标识：注入的短 SHA，或 VCS stamp 短 SHA（脏工作树带
// -dirty），两者都取不到时为 unknown。
func Commit() string { return cached().commit }

// BuildTime 返回 RFC3339 UTC 构建时间；未注入时为 unknown（不伪造）。
func BuildTime() string { return cached().buildTime }

// GoVersion 返回运行本进程的 Go 工具链版本，如 go1.27.0。这是运行期事实，不注入。
func GoVersion() string { return runtime.Version() }

// Platform 返回 goos/goarch，如 windows/amd64。这是运行期事实，不注入。
func Platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// CgoEnabled 报告本次构建是否链接了 C 工具链产物（官方服务端恒为 true）。
// 值来自构建约束常量（cgo.go / nocgo.go），不经 -X 注入。
func CgoEnabled() bool { return cgoEnabled }
