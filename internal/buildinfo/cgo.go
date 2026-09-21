//go:build cgo

// 本文件覆盖"cgo 构建"这一侧：`cgo` 构建标签由工具链在 CGO_ENABLED=1 且目标平台
// 支持 cgo 时自动置位，因此 CGO 状态天然是编译期事实，不需要 -X 注入
// （注入只会多出一个 cgo_enabled=unknown 的不可能态）。与 nocgo.go 成对。
package buildinfo

// cgoEnabled 供 CgoEnabled() 返回：此构建链接了 C/C++ 产物（如 internal/delta/hdiffc）。
const cgoEnabled = true
