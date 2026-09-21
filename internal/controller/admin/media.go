package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// RegisterMedia 挂载管理端 Markdown 媒体上传（project:admin）。media 为 nil 时不注册。
func RegisterMedia(rg *gin.RouterGroup, admins *service.AdminService, projects *service.ProjectService, media *service.MediaService) {
	if media == nil || projects == nil || admins == nil {
		return
	}
	h := &mediaHandler{projects: projects, media: media}
	g := rg.Group("/projects/:project_ref")
	g.Use(middleware.ProjectAccess(admins, projects, model.ScopeProjectAdmin))
	g.POST("/media", h.upload)
}

type mediaHandler struct {
	projects *service.ProjectService
	media    *service.MediaService
}

func (h *mediaHandler) upload(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "expected multipart form", nil)
		return
	}
	headers := form.File["file[]"]
	if len(headers) == 0 {
		headers = form.File["file"]
	}
	if len(headers) == 0 {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "missing multipart file[]", nil)
		return
	}

	succMap := map[string]string{}
	errFiles := make([]string, 0)
	files := make([]gin.H, 0, len(headers))
	for _, fh := range headers {
		f, err := fh.Open()
		if err != nil {
			errFiles = append(errFiles, fh.Filename)
			continue
		}
		res, err := h.media.Put(c.Request.Context(), p, service.MediaPutInput{
			FileName:    fh.Filename,
			ContentType: fh.Header.Get("Content-Type"),
			Body:        f,
		})
		_ = f.Close()
		if err != nil {
			errFiles = append(errFiles, fh.Filename)
			continue
		}
		succMap[fh.Filename] = res.URL
		files = append(files, gin.H{
			"id":           res.Media.ID,
			"file_name":    res.Media.FileName,
			"size":         res.Media.Size,
			"content_type": res.Media.ContentType,
			"sha256":       res.Media.SHA256,
			"md5":          res.Media.MD5,
			"sha512":       res.Media.SHA512,
			"url":          res.URL,
		})
	}

	code := 0
	msg := ""
	if len(succMap) == 0 {
		code = 1
		msg = "upload failed"
		if len(errFiles) == 0 {
			msg = "no files"
		}
	}
	response.JSON(c, http.StatusOK, gin.H{
		"code": code,
		"msg":  msg,
		"data": gin.H{
			"succMap":  succMap,
			"errFiles": errFiles,
			"files":    files,
		},
	})
}
