package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type ciReleaseJSONReq struct {
	Version         string                 `json:"version"`
	Channel         string                 `json:"channel"`
	Changelog       *string                `json:"changelog"`
	Publish         *bool                  `json:"publish"`
	AutoPublishWhen *model.AutoPublishRule `json:"auto_publish_when"`
	BundleBase64    string                 `json:"bundle_base64"`
	ArchiveBase64   string                 `json:"archive_base64"`
}

func (h *projectHandler) ciRelease(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}

	var (
		versionStr      string
		channelStr      string
		changelog       *string
		publish         = true // 默认 true (C07-2)
		autoPublishWhen *model.AutoPublishRule
		archiveData     []byte
		archiveFilename = "bundle.zip"
	)

	contentType := c.ContentType()

	if strings.HasPrefix(contentType, "multipart/form-data") {
		fileHeader, err := c.FormFile("file")
		if err != nil || fileHeader == nil {
			fileHeader, err = c.FormFile("bundle")
		}
		if err != nil || fileHeader == nil {
			fileHeader, _ = c.FormFile("archive")
		}
		if fileHeader != nil {
			archiveFilename = fileHeader.Filename
			f, err := fileHeader.Open()
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to open form file", nil)
				return
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to read form file", nil)
				return
			}
			archiveData = data
		}

		// 解析元数据（支持单独 form 字段或 metadata/json 字符串字段）
		if metaStr := c.PostForm("metadata"); metaStr != "" {
			var m ciReleaseJSONReq
			if json.Unmarshal([]byte(metaStr), &m) == nil {
				versionStr = m.Version
				channelStr = m.Channel
				changelog = m.Changelog
				if m.Publish != nil {
					publish = *m.Publish
				}
				autoPublishWhen = m.AutoPublishWhen
			}
		} else if jsonStr := c.PostForm("json"); jsonStr != "" {
			var m ciReleaseJSONReq
			if json.Unmarshal([]byte(jsonStr), &m) == nil {
				versionStr = m.Version
				channelStr = m.Channel
				changelog = m.Changelog
				if m.Publish != nil {
					publish = *m.Publish
				}
				autoPublishWhen = m.AutoPublishWhen
			}
		}

		if versionStr == "" {
			versionStr = c.PostForm("version")
		}
		if channelStr == "" {
			channelStr = c.PostForm("channel")
		}
		if changelog == nil {
			if cl := c.PostForm("changelog"); cl != "" {
				changelog = &cl
			}
		}
		if pStr := c.PostForm("publish"); pStr != "" {
			if strings.EqualFold(pStr, "false") || pStr == "0" {
				publish = false
			}
		}
		if autoPublishWhen == nil {
			if apStr := c.PostForm("auto_publish_when"); apStr != "" {
				var rule model.AutoPublishRule
				if json.Unmarshal([]byte(apStr), &rule) == nil {
					autoPublishWhen = &rule
				}
			}
		}
	} else if strings.HasPrefix(contentType, "application/json") {
		var req ciReleaseJSONReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
			return
		}
		versionStr = req.Version
		channelStr = req.Channel
		changelog = req.Changelog
		if req.Publish != nil {
			publish = *req.Publish
		}
		autoPublishWhen = req.AutoPublishWhen

		b64 := req.BundleBase64
		if b64 == "" {
			b64 = req.ArchiveBase64
		}
		if b64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid base64 archive data", nil)
				return
			}
			archiveData = decoded
		}
	} else {
		// raw binary 压缩包流
		data, err := io.ReadAll(c.Request.Body)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to read body", nil)
			return
		}
		archiveData = data
		versionStr = c.Query("version")
		channelStr = c.Query("channel")
		if cl := c.Query("changelog"); cl != "" {
			changelog = &cl
		}
		if pStr := c.Query("publish"); pStr != "" {
			if strings.EqualFold(pStr, "false") || pStr == "0" {
				publish = false
			}
		}
	}

	if len(archiveData) == 0 {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "missing bundle archive file or body", nil)
		return
	}
	if strings.TrimSpace(versionStr) == "" {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "version is required", nil)
		return
	}

	in := service.CreateBundleReleaseInput{
		Version:         versionStr,
		Channel:         channelStr,
		Changelog:       changelog,
		Publish:         &publish,
		AutoPublishWhen: autoPublishWhen,
		IdempotencyKey:  c.GetHeader("Idempotency-Key"),
		ArchiveData:     archiveData,
		ArchiveFilename: archiveFilename,
	}

	jobID, _, err := h.projects.CreateBundleJob(c.Request.Context(), p.ID, in)
	if err != nil {
		writeVersionErr(c, err)
		return
	}

	// 异步触发执行以支持无独立 worker 进程时的快速轮询
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = h.projects.ExecuteBundleJob(context.Background(), jobID)
	}()

	response.JSON(c, http.StatusAccepted, gin.H{
		"job_id": jobID,
		"status": model.JobStatusQueued,
	})
}

func (h *projectHandler) uploadBundle(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}

	versionRef := c.Param("version")
	var archiveData []byte
	archiveFilename := "bundle.zip"

	contentType := c.ContentType()
	if strings.HasPrefix(contentType, "multipart/form-data") {
		fileHeader, err := c.FormFile("file")
		if err != nil || fileHeader == nil {
			fileHeader, err = c.FormFile("bundle")
		}
		if fileHeader != nil {
			archiveFilename = fileHeader.Filename
			f, err := fileHeader.Open()
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to open form file", nil)
				return
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to read form file", nil)
				return
			}
			archiveData = data
		}
	} else {
		data, err := io.ReadAll(c.Request.Body)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to read body", nil)
			return
		}
		archiveData = data
	}

	if len(archiveData) == 0 {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "missing bundle archive file or body", nil)
		return
	}

	publish := false
	if pStr := c.Query("publish"); pStr != "" {
		if strings.EqualFold(pStr, "true") || pStr == "1" {
			publish = true
		}
	}

	in := service.CreateBundleReleaseInput{
		Version:         versionRef,
		Publish:         &publish,
		IdempotencyKey:  c.GetHeader("Idempotency-Key"),
		ArchiveData:     archiveData,
		ArchiveFilename: archiveFilename,
	}

	jobID, _, err := h.projects.CreateBundleJob(c.Request.Context(), p.ID, in)
	if err != nil {
		writeVersionErr(c, err)
		return
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = h.projects.ExecuteBundleJob(context.Background(), jobID)
	}()

	response.JSON(c, http.StatusAccepted, gin.H{
		"job_id": jobID,
		"status": model.JobStatusQueued,
	})
}

func (h *projectHandler) getJob(c *gin.Context) {
	jobIDStr := c.Param("job_id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid job_id uuid", nil)
		return
	}

	var callerProjectID *uuid.UUID
	if val, exists := c.Get("caller_project_id"); exists {
		if pid, ok := val.(uuid.UUID); ok {
			callerProjectID = &pid
		}
	}

	isInstanceAdmin := false
	if val, exists := c.Get("is_instance_admin"); exists {
		if b, ok := val.(bool); ok {
			isInstanceAdmin = b
		}
	}

	if admin := middleware.AdminFrom(c); admin != nil {
		job, err := h.projects.GetJobAsAdmin(c.Request.Context(), admin, jobID)
		if err != nil {
			writeVersionErr(c, err)
			return
		}
		response.JSON(c, http.StatusOK, publicJob(job))
		return
	}

	job, err := h.projects.GetJob(c.Request.Context(), callerProjectID, isInstanceAdmin, jobID)
	if err != nil {
		writeVersionErr(c, err)
		return
	}

	response.JSON(c, http.StatusOK, publicJob(job))
}

func publicJob(j *model.Job) gin.H {
	h := gin.H{
		"id":         j.ID,
		"job_id":     j.ID,
		"type":       j.Type,
		"status":     j.Status,
		"progress":   j.Progress,
		"created_at": formatTimeUTC(j.CreatedAt),
		"updated_at": formatTimeUTC(j.UpdatedAt),
	}
	if j.ProjectID != nil {
		h["project_id"] = *j.ProjectID
	}
	if j.ErrorMessage != "" {
		h["error_message"] = j.ErrorMessage
	}
	if len(j.Result) > 0 {
		var raw any
		if json.Unmarshal(j.Result, &raw) == nil {
			h["result"] = raw
		} else {
			h["result"] = string(j.Result)
		}
	}
	return h
}
