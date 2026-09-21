// Package frontend 把 Vite 生产目录 dist 在编译期打进二进制。
// go:embed 不能引用包目录之外的路径，因此本文件必须放在 frontend/ 下。
package frontend

import (
	"embed"
	"io/fs"
)

// distFS 包含 frontend/dist 整棵树（all: 才能嵌入 .gitkeep）。
// dist 在编译时必须存在且至少有一个文件；占位文件是 dist/.gitkeep。
//
//go:embed all:dist
var distFS embed.FS

// Dist 返回以 dist 为根的文件系统（index.html 在根路径）。
// 仅占位、尚未 yarn build 时没有 index.html，管理平面按 API-only 处理。
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return distFS
	}
	return sub
}
