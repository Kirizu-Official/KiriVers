package admin

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type putManifestReq struct {
	Entries []service.ManifestEntryInput `json:"entries"`
}

func publicManifestEntry(e model.ManifestEntry) gin.H {
	return gin.H{
		"path":            e.Path,
		"size":            e.Size,
		"sha256":          e.SHA256,
		"md5":             e.MD5,
		"install_policy":  e.InstallPolicy,
		"integrity_check": e.IntegrityCheck,
	}
}

func publicManifestEntries(entries []model.ManifestEntry) []gin.H {
	res := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		res = append(res, publicManifestEntry(e))
	}
	return res
}

// putManifest 提交或更新多文件 Version Line 的 Manifest 并自动计算 Root Hash（C06-1, C06-2, C06-3）。
func (h *projectHandler) putManifest(c *gin.Context) {
	var req putManifestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body", err.Error())
		return
	}

	res, err := h.projects.SetManifest(
		c.Request.Context(),
		c.Param("project_ref"),
		c.Param("version"),
		c.Param("os"),
		c.Param("arch"),
		req.Entries,
	)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	response.JSON(c, http.StatusOK, gin.H{
		"root_hash": res.RootHash,
		"count":     res.Count,
		"entries":   publicManifestEntries(res.Entries),
	})
}

// getManifest 查询多文件 Version Line 的 Manifest 条目列表与 Root Hash（C06-1, C06-3）。
func (h *projectHandler) getManifest(c *gin.Context) {
	res, err := h.projects.GetManifest(
		c.Request.Context(),
		c.Param("project_ref"),
		c.Param("version"),
		c.Param("os"),
		c.Param("arch"),
	)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	response.JSON(c, http.StatusOK, gin.H{
		"root_hash": res.RootHash,
		"count":     res.Count,
		"entries":   publicManifestEntries(res.Entries),
	})
}

// buildArchive 提交 zip 归档包或通过请求体生成全量归档包产物并更新平台切片 Manifest 与就绪状态（C06-4, C06-6）。
func (h *projectHandler) buildArchive(c *gin.Context) {
	var zipBytes []byte

	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		file, err := c.FormFile("file")
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "missing multipart file 'file'", err.Error())
			return
		}
		f, err := file.Open()
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "cannot open uploaded file", err.Error())
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "read uploaded file failed", err.Error())
			return
		}
		zipBytes = data
	} else {
		// 直接读取二进制请求体
		data, err := io.ReadAll(c.Request.Body)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "read request body failed", err.Error())
			return
		}
		zipBytes = data
	}

	if len(zipBytes) == 0 {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "zip payload is empty", nil)
		return
	}

	art, mRes, err := h.projects.BuildArchiveFromZipBuffer(
		c.Request.Context(),
		c.Param("project_ref"),
		c.Param("version"),
		c.Param("os"),
		c.Param("arch"),
		zipBytes,
	)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	response.JSON(c, http.StatusCreated, gin.H{
		"artifact": publicArtifact(art),
		"manifest": gin.H{
			"root_hash": mRes.RootHash,
			"count":     mRes.Count,
			"entries":   publicManifestEntries(mRes.Entries),
		},
	})
}
