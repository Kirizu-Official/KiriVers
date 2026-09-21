package controller

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// mountAdminStatic 覆盖 admin 引擎的 NoRoute，从磁盘或编译期嵌入的 FS 托管管理台。
// 只走 NoRoute、不注册 gin.Static / StaticFS，以免引擎路由表出现 GET /*filepath
// 导致 TestOpenAPIRoutesSync 失败。client 引擎不得调用本函数。
func mountAdminStatic(engine *gin.Engine, staticDir string, embedded fs.FS) {
	engine.NoRoute(func(c *gin.Context) {
		serveAdminStatic(c, staticDir, embedded)
	})
}

// serveAdminStatic 按设计顺序处理 admin 未知路径：/api 与非 GET/HEAD 保持 JSON
// NOT_FOUND；static_dir 空则关闭 UI（即使有内嵌副本）；磁盘 index.html 优先，
// 否则用 embedded；两者都没有则 API-only。真实文件用 ServeFile / ServeFileFS；
// 带扩展名的缺失资源不回 HTML；其余 GET/HEAD 回根 index.html。
func serveAdminStatic(c *gin.Context, staticDir string, embedded fs.FS) {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		response.NotFound(c)
		return
	}

	urlPath := c.Request.URL.Path
	if urlPath == "" {
		urlPath = "/"
	}
	cleaned := path.Clean(urlPath)
	if cleaned == "." {
		cleaned = "/"
	}
	if isAPIPath(cleaned) {
		response.NotFound(c)
		return
	}

	staticDir = strings.TrimSpace(staticDir)
	if staticDir == "" {
		response.NotFound(c)
		return
	}

	rootAbs, indexPath, diskOK := diskIndex(staticDir)
	embedOK := fsHasIndex(embedded)
	if !diskOK && !embedOK {
		response.NotFound(c)
		return
	}

	if pathHasForbiddenSegments(urlPath) {
		response.NotFound(c)
		return
	}

	if cleaned == "/" {
		serveAdminIndex(c, diskOK, indexPath, embedded)
		return
	}

	rel, err := pathutil.NormalizeAndValidatePath(strings.TrimPrefix(cleaned, "/"))
	if err != nil {
		response.NotFound(c)
		return
	}

	if diskOK {
		serveAdminDiskFile(c, rootAbs, indexPath, rel)
		return
	}
	serveAdminFSFile(c, embedded, rel)
}

func serveAdminIndex(c *gin.Context, diskOK bool, indexPath string, embedded fs.FS) {
	if diskOK {
		http.ServeFile(c.Writer, c.Request, indexPath)
		return
	}
	http.ServeFileFS(c.Writer, c.Request, embedded, "index.html")
}

func serveAdminDiskFile(c *gin.Context, rootAbs, indexPath, rel string) {
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(rel)))
	if err != nil {
		response.NotFound(c)
		return
	}
	relToRoot, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || escapesStaticRoot(relToRoot) {
		response.NotFound(c)
		return
	}

	info, err := os.Stat(targetAbs)
	if err == nil && !info.IsDir() {
		http.ServeFile(c.Writer, c.Request, targetAbs)
		return
	}
	if strings.Contains(path.Base(rel), ".") {
		response.NotFound(c)
		return
	}
	http.ServeFile(c.Writer, c.Request, indexPath)
}

func serveAdminFSFile(c *gin.Context, fsys fs.FS, rel string) {
	info, err := fs.Stat(fsys, rel)
	if err == nil && !info.IsDir() {
		http.ServeFileFS(c.Writer, c.Request, fsys, rel)
		return
	}
	if strings.Contains(path.Base(rel), ".") {
		response.NotFound(c)
		return
	}
	http.ServeFileFS(c.Writer, c.Request, fsys, "index.html")
}

func diskIndex(staticDir string) (rootAbs, indexPath string, ok bool) {
	rootAbs, err := filepath.Abs(staticDir)
	if err != nil {
		return "", "", false
	}
	indexPath = filepath.Join(rootAbs, "index.html")
	info, err := os.Stat(indexPath)
	if err != nil || info.IsDir() {
		return rootAbs, indexPath, false
	}
	return rootAbs, indexPath, true
}

func fsHasIndex(fsys fs.FS) bool {
	if fsys == nil {
		return false
	}
	info, err := fs.Stat(fsys, "index.html")
	return err == nil && !info.IsDir()
}

func isAPIPath(cleaned string) bool {
	return cleaned == "/api" || strings.HasPrefix(cleaned, "/api/")
}

// pathHasForbiddenSegments 在 path.Clean 折叠 .. 之前校验原始 URL，拒绝穿越、
// 控制字符与盘符。pathutil 面向相对路径，须先去掉首尾 /。
func pathHasForbiddenSegments(urlPath string) bool {
	rel := strings.Trim(urlPath, "/")
	if rel == "" {
		return false
	}
	_, err := pathutil.NormalizeAndValidatePath(rel)
	return err != nil
}

func escapesStaticRoot(rel string) bool {
	if rel == ".." || filepath.IsAbs(rel) {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(rel, ".."+sep) || strings.HasPrefix(rel, "../")
}
