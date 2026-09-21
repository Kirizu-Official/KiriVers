//go:build !cgo

// 本文件覆盖"纯 Go 构建"这一侧：CGO_ENABLED=0 或不支持 cgo 的目标只有这里编译。
// 官方服务端产物不会走这条路（build-cgo 显式拒绝 CGO_ENABLED=0，且
// `CGO_ENABLED=0 go build .` 必须失败），但 internal/buildinfo 作为叶子包会被
// 各种测试与工具链组合编译，因此必须两侧都有定义。与 cgo.go 成对。
package buildinfo

// cgoEnabled 报告本次构建没有链接任何 C/C++ 产物。
const cgoEnabled = false
