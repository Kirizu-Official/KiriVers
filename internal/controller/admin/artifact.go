package admin

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/delta"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

func publicArtifact(a *model.Artifact) gin.H {
	h := gin.H{
		"id":              a.ID,
		"project_id":      a.ProjectID,
		"version_id":      a.VersionID,
		"version_line_id": a.VersionLineID,
		"kind":            a.Kind,
		"file_name":       a.FileName,
		"size":            a.Size,
		"sha256":          a.SHA256,
		"md5":             a.MD5,
		"content_type":    a.ContentType,
		"created_at":      formatTimeUTC(a.CreatedAt),
		"updated_at":      formatTimeUTC(a.UpdatedAt),
	}
	if a.Compression != "" {
		h["compression"] = a.Compression
	}
	if a.FilesetSHA256 != "" {
		h["fileset_sha256"] = a.FilesetSHA256
	}
	if a.SHA512 != "" {
		h["sha512"] = a.SHA512
	}
	if a.ArtifactSignature != "" {
		h["artifact_signature"] = a.ArtifactSignature
	}
	if a.HwRev != nil {
		h["hw_rev"] = *a.HwRev
	}
	if a.MinHwRev != nil {
		h["min_hw_rev"] = *a.MinHwRev
	}
	if a.MaxHwRev != nil {
		h["max_hw_rev"] = *a.MaxHwRev
	}
	if len(a.CompatibleHwRevs) > 0 {
		h["compatible_hw_revs"] = a.CompatibleHwRevs
	}
	return h
}

func writeArtifactErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrChecksumMismatch):
		response.Error(c, http.StatusBadRequest, "CHECKSUM_MISMATCH", err.Error(), nil)
	case errors.Is(err, service.ErrArtifactImmutable):
		response.Error(c, http.StatusConflict, "ARTIFACT_IMMUTABLE", err.Error(), nil)
	case errors.Is(err, service.ErrUploadIncomplete):
		response.Error(c, http.StatusConflict, "UPLOAD_INCOMPLETE", err.Error(), nil)
	case errors.Is(err, service.ErrHwRevUnknown):
		response.Error(c, http.StatusBadRequest, "HW_REV_UNKNOWN", err.Error(), nil)
	case errors.Is(err, service.ErrHwRevIncompatible):
		response.Error(c, http.StatusConflict, "HW_REV_INCOMPATIBLE", err.Error(), nil)
	case errors.Is(err, service.ErrOffsetMismatch):
		response.Error(c, http.StatusConflict, "OFFSET_MISMATCH", err.Error(), nil)
	case errors.Is(err, service.ErrUploadSessionNotFound):
		response.Error(c, http.StatusNotFound, "UPLOAD_SESSION_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrArtifactNotFound):
		response.Error(c, http.StatusNotFound, "ARTIFACT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrReuseSourceRevoked):
		// §5.1：吊销版本上的对象禁止被复用（C14-1）。
		response.Error(c, http.StatusConflict, "VERSION_REVOKED", err.Error(), nil)
	case errors.Is(err, service.ErrReuseAmbiguousSHA256):
		// sha256 引用在项目内命中多个 kind=full 产物 → 要求改用 artifact_id（C14-1）。
		response.Error(c, http.StatusBadRequest, "AMBIGUOUS_SHA256", err.Error(), nil)
	case errors.Is(err, service.ErrReuseSourceRequired):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	case errors.Is(err, service.ErrDeltaBaselineMissing):
		response.Error(c, http.StatusNotFound, "ARTIFACT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrDeltaAlgoUnsupported):
		// C10-8 / §7.2：未知差量算法 → 400 DELTA_ALGO_UNSUPPORTED。
		response.Error(c, http.StatusBadRequest, "DELTA_ALGO_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, service.ErrDeltaSameVersion):
		response.Error(c, http.StatusBadRequest, "DELTA_SAME_VERSION", err.Error(), nil)
	case errors.Is(err, service.ErrStorageUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "NOT_READY", err.Error(), nil)
	default:
		writeVersionErr(c, err)
	}
}

// uploadArtifact 处理 PUT/POST 小文件上传 (C05-1, C05-4, C05-5, C05-7, C05-11)。
func (h *projectHandler) uploadArtifact(c *gin.Context) {
	filename := c.GetHeader("X-Filename")
	if filename == "" {
		filename = c.Query("filename")
	}

	hwRev := c.GetHeader("X-Hw-Rev")
	if hwRev == "" {
		hwRev = c.Query("hw_rev")
	}
	var hwRevPtr *string
	if strings.TrimSpace(hwRev) != "" {
		trimmed := strings.TrimSpace(hwRev)
		hwRevPtr = &trimmed
	}

	var minHwPtr *string
	if minHw := strings.TrimSpace(c.Query("min_hw_rev")); minHw != "" {
		minHwPtr = &minHw
	}
	var maxHwPtr *string
	if maxHw := strings.TrimSpace(c.Query("max_hw_rev")); maxHw != "" {
		maxHwPtr = &maxHw
	}
	var compHws []string
	if rawComp := c.Query("compatible_hw_revs"); rawComp != "" {
		for _, s := range strings.Split(rawComp, ",") {
			if tr := strings.TrimSpace(s); tr != "" {
				compHws = append(compHws, tr)
			}
		}
	}

	sig := c.GetHeader("X-Artifact-Signature")
	if sig == "" {
		sig = c.Query("artifact_signature")
	}

	var body io.Reader = c.Request.Body
	size := c.Request.ContentLength

	// 支持 multipart/form-data 或 raw binary
	contentType := c.ContentType()
	if strings.HasPrefix(contentType, "multipart/form-data") {
		fileHeader, err := c.FormFile("file")
		if err == nil && fileHeader != nil {
			if filename == "" {
				filename = fileHeader.Filename
			}
			f, err := fileHeader.Open()
			if err != nil {
				response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to open form file", nil)
				return
			}
			defer f.Close()
			body = f
			size = fileHeader.Size
		}
	}

	in := service.UploadArtifactInput{
		Filename:          filename,
		ExpectedSHA256:    c.GetHeader("X-Content-SHA256"),
		IdempotencyKey:    c.GetHeader("Idempotency-Key"),
		HwRev:             hwRevPtr,
		MinHwRev:          minHwPtr,
		MaxHwRev:          maxHwPtr,
		CompatibleHwRevs:  compHws,
		ArtifactSignature: sig,
		Size:              size,
		ContentType:       c.GetHeader("Content-Type"),
	}

	art, err := h.projects.UploadArtifact(c.Request.Context(), c.Param("project_ref"), c.Param("version"), c.Param("os"), c.Param("arch"), in, body)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	response.JSON(c, http.StatusOK, publicArtifact(art))
}

// createTusUpload 初始化 TUS 1.0 上传会话 (C05-2)。
func (h *projectHandler) createTusUpload(c *gin.Context) {
	uploadLengthStr := c.GetHeader("Upload-Length")
	if uploadLengthStr == "" {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Upload-Length header required", nil)
		return
	}
	uploadLength, err := strconv.ParseInt(uploadLengthStr, 10, 64)
	if err != nil || uploadLength < 0 {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid Upload-Length", nil)
		return
	}

	metadataHeader := c.GetHeader("Upload-Metadata")
	meta := parseTusMetadata(metadataHeader)

	var hwRevPtr *string
	if val, ok := meta["hw_rev"].(string); ok && strings.TrimSpace(val) != "" {
		trimmed := strings.TrimSpace(val)
		hwRevPtr = &trimmed
	}
	var minHwPtr *string
	if val, ok := meta["min_hw_rev"].(string); ok && strings.TrimSpace(val) != "" {
		trimmed := strings.TrimSpace(val)
		minHwPtr = &trimmed
	}
	var maxHwPtr *string
	if val, ok := meta["max_hw_rev"].(string); ok && strings.TrimSpace(val) != "" {
		trimmed := strings.TrimSpace(val)
		maxHwPtr = &trimmed
	}
	var compHws []string
	if val, ok := meta["compatible_hw_revs"].(string); ok && strings.TrimSpace(val) != "" {
		for _, s := range strings.Split(val, ",") {
			if tr := strings.TrimSpace(s); tr != "" {
				compHws = append(compHws, tr)
			}
		}
	}

	var filename string
	if val, ok := meta["filename"].(string); ok {
		filename = val
	}
	var expectedSHA string
	if val, ok := meta["sha256"].(string); ok {
		expectedSHA = val
	}
	var sig string
	if val, ok := meta["artifact_signature"].(string); ok {
		sig = val
	}

	in := service.CreateTusUploadInput{
		Size:              uploadLength,
		Filename:          filename,
		ExpectedSHA256:    expectedSHA,
		IdempotencyKey:    c.GetHeader("Idempotency-Key"),
		HwRev:             hwRevPtr,
		MinHwRev:          minHwPtr,
		MaxHwRev:          maxHwPtr,
		CompatibleHwRevs:  compHws,
		ArtifactSignature: sig,
		Metadata:          meta,
	}

	sess, err := h.projects.CreateTusUpload(c.Request.Context(), c.Param("project_ref"), c.Param("version"), c.Param("os"), c.Param("arch"), in)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	loc := fmt.Sprintf("/api/v1/admin/projects/%s/artifacts/tus/%s", c.Param("project_ref"), sess.ID)
	c.Header("Location", loc)
	c.Header("Tus-Resumable", "1.0.0")
	c.Header("Tus-Version", "1.0.0")
	c.Header("Tus-Extension", "creation,termination")
	c.Status(http.StatusCreated)
}

// getTusUpload 查询当前 TUS 上传偏移量 (HEAD)。
func (h *projectHandler) getTusUpload(c *gin.Context) {
	uploadID, err := uuid.Parse(c.Param("upload_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid upload id", nil)
		return
	}

	sess, err := h.projects.GetTusUpload(c.Request.Context(), c.Param("project_ref"), uploadID)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	c.Header("Upload-Offset", strconv.FormatInt(sess.Offset, 10))
	c.Header("Upload-Length", strconv.FormatInt(sess.Size, 10))
	c.Header("Tus-Resumable", "1.0.0")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
}

// patchTusUpload 追加写入 TUS 分片数据 (PATCH)。
func (h *projectHandler) patchTusUpload(c *gin.Context) {
	uploadID, err := uuid.Parse(c.Param("upload_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid upload id", nil)
		return
	}

	offsetStr := c.GetHeader("Upload-Offset")
	if offsetStr == "" {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Upload-Offset header required", nil)
		return
	}
	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil || offset < 0 {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid Upload-Offset", nil)
		return
	}

	sess, _, err := h.projects.WriteTusChunk(c.Request.Context(), c.Param("project_ref"), uploadID, offset, c.Request.Body)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	c.Header("Upload-Offset", strconv.FormatInt(sess.Offset, 10))
	c.Header("Tus-Resumable", "1.0.0")
	c.Status(http.StatusNoContent)
}

// deleteTusUpload 中止 TUS 上传会话 (DELETE)。
func (h *projectHandler) deleteTusUpload(c *gin.Context) {
	uploadID, err := uuid.Parse(c.Param("upload_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid upload id", nil)
		return
	}

	if err := h.projects.AbortTusUpload(c.Request.Context(), c.Param("project_ref"), uploadID); err != nil {
		writeArtifactErr(c, err)
		return
	}

	c.Header("Tus-Resumable", "1.0.0")
	c.Status(http.StatusNoContent)
	// 成功后写审计（§11.2 / C15-3，artifact.delete）。
	h.audit.recordAudit(c, h.projectIDFromRef(c), model.AuditActionArtifactDelete, "upload_session", uploadID.String(), nil)
}

// presignUpload 生成 S3 预签名直传 URL 或 API 代传 URL (C05-3)。
func (h *projectHandler) presignUpload(c *gin.Context) {
	var in service.UploadArtifactInput
	_ = c.ShouldBindJSON(&in)
	if in.Filename == "" {
		in.Filename = c.Query("filename")
	}

	out, err := h.projects.PresignUpload(c.Request.Context(), c.Param("project_ref"), c.Param("version"), c.Param("os"), c.Param("arch"), in)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	response.JSON(c, http.StatusOK, out)
}

// cleanupArtifacts 清理过期或中止的上传对象 (C05-10)。
func (h *projectHandler) cleanupArtifacts(c *gin.Context) {
	days := 90
	if daysStr := c.Query("retention_days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}
	count, err := h.projects.CleanupArtifacts(c.Request.Context(), c.Param("project_ref"), days)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"cleaned_count": count})
	// 成功后写审计（§11.2 / C15-3，artifact.delete）。
	h.audit.recordAudit(c, h.projectIDFromRef(c), model.AuditActionArtifactDelete, "artifact", "cleanup",
		gin.H{"retention_days": days, "cleaned_count": count})
}

// createDeltaJob 处理管理端差量生成请求（C10-7 / §7.2）：
// POST /versions/:version/artifacts/delta
// 校验失败按错误类型返回（未知算法 → 400 DELTA_ALGO_UNSUPPORTED；
// source==target → 400 DELTA_SAME_VERSION）；成功 → 202 + job_id（delta_generate）。
func (h *projectHandler) createDeltaJob(c *gin.Context) {
	var req createDeltaJobReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}

	// 端点缺省算法为空 → 由服务端取矩阵 delta_algo（不在此另造默认）。
	var hwRevPtr *string
	if hw := strings.TrimSpace(req.HwRev); hw != "" {
		hwRevPtr = &hw
	}
	in := service.CreateDeltaJobInput{
		SourceVersion:  req.SourceVersion,
		OS:             req.OS,
		Arch:           req.Arch,
		Algo:           req.Algo,
		HwRev:          hwRevPtr,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
	}

	jobID, created, err := h.projects.CreateDeltaJob(c.Request.Context(), c.Param("project_ref"), c.Param("version"), in)
	if err != nil {
		writeArtifactErr(c, err)
		return
	}

	response.JSON(c, http.StatusAccepted, gin.H{
		"job_id":  jobID,
		"created": created,
		// 引擎可用性快照（design §6）：三种算法及当前实现形态（官方 CLI / 纯 Go 回退）。
		"engines": delta.DescribeAvailable(),
	})
}

// createDeltaJobReq 是差量生成请求体（C10-7 / §7.2）。
type createDeltaJobReq struct {
	// SourceVersion 源版本引用（必填）；目标版本 = 路径中的 :version。
	SourceVersion string `json:"source_version"`
	// OS / Arch 目标平台（必填）。
	OS   string `json:"os"`
	Arch string `json:"arch"`
	// Algo 差量算法（可选；缺省取矩阵 delta_algo）。
	Algo string `json:"algo"`
	// HwRev 可选硬件代号变体。
	HwRev string `json:"hw_rev"`
}

// parseTusMetadata 解析 TUS 1.0 Upload-Metadata 头：
// 格式："filename aW1hZ2UucG5n,filetype aW1hZ2UvcG5n"
func parseTusMetadata(header string) map[string]any {
	res := make(map[string]any)
	header = strings.TrimSpace(header)
	if header == "" {
		return res
	}
	pairs := strings.Split(header, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, " ", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		if len(parts) == 1 {
			res[key] = ""
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			res[key] = parts[1]
		} else {
			res[key] = string(decoded)
		}
	}
	return res
}

// reuseArtifactReq 是产物复用请求体（C14-1，§5.8）：
// 在目标版本的 (os,arch[,hw]) 线上引用已有存储对象，不复制字节。
type reuseArtifactReq struct {
	OS     string  `json:"os"`
	Arch   string  `json:"arch"`
	HwRev  *string `json:"hw_rev"`
	Source struct {
		ArtifactID *uuid.UUID `json:"artifact_id"`
		SHA256     string     `json:"sha256"`
	} `json:"source"`
}

// reuseArtifact 处理 POST /versions/:version/artifacts/reuse（C14-1，§5.8）。
// 单文件线返回 1 条新引用行；多文件线克隆 Manifest 并复用预生成 zip 对象。
func (h *projectHandler) reuseArtifact(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req reuseArtifactReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	if strings.TrimSpace(req.OS) == "" || strings.TrimSpace(req.Arch) == "" {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "os and arch are required", nil)
		return
	}
	result, err := h.projects.ReuseArtifacts(c.Request.Context(), p.Slug, c.Param("version"), req.OS, req.Arch, service.ReuseArtifactsInput{
		OS:    req.OS,
		Arch:  req.Arch,
		HwRev: req.HwRev,
		Source: service.ReuseSource{
			ArtifactID: req.Source.ArtifactID,
			SHA256:     req.Source.SHA256,
		},
	})
	if err != nil {
		writeArtifactErr(c, err)
		return
	}
	artifacts := make([]gin.H, 0, len(result.Artifacts))
	for i := range result.Artifacts {
		artifacts = append(artifacts, publicArtifact(&result.Artifacts[i]))
	}
	body := gin.H{"artifacts": artifacts}
	if result.Manifest != nil {
		body["manifest"] = gin.H{
			"root_hash": result.Manifest.RootHash,
			"count":     result.Manifest.Count,
			"entries":   result.Manifest.Entries,
		}
	}
	response.JSON(c, http.StatusCreated, body)
	// 成功后写审计（C14-1 产物复用，§11.2 / C15-3）。
	h.audit.recordAudit(c, &p.ID, model.AuditActionArtifactReuse, "artifact", c.Param("version"),
		gin.H{"os": req.OS, "arch": req.Arch, "reused": len(result.Artifacts)})
}
