package admin

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type geoipHandler struct {
	geoip *service.GeoipService
}

type geoipPatchReq struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
	Rank    *int    `json:"rank"`
}

func publicGeoip(row *model.GeoipDatabase) gin.H {
	return gin.H{
		"id":         row.ID,
		"name":       row.Name,
		"file_name":  row.FileName,
		"size":       row.Size,
		"enabled":    row.Enabled,
		"rank":       row.Rank,
		"created_at": formatTimeUTC(row.CreatedAt),
		"updated_at": formatTimeUTC(row.UpdatedAt),
	}
}

func (h *geoipHandler) list(c *gin.Context) {
	list, err := h.geoip.List(c.Request.Context())
	if err != nil {
		writeGeoipErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicGeoip(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"databases": items})
}

func (h *geoipHandler) upload(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(model.GeoipMaxFileBytes + 4096); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "expected multipart form", nil)
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "missing multipart file", nil)
		return
	}
	if fh.Size > model.GeoipMaxFileBytes {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "file exceeds 256MiB", nil)
		return
	}
	f, err := fh.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "cannot read file", nil)
		return
	}
	defer f.Close()
	name := stringsOr(c.PostForm("name"), fh.Filename)
	rankRaw := strings.TrimSpace(c.PostForm("rank"))
	row, err := h.geoip.Upload(c.Request.Context(), name, fh.Filename, io.LimitReader(f, model.GeoipMaxFileBytes+1), fh.Size)
	if err != nil {
		writeGeoipErr(c, err)
		return
	}
	if rankRaw != "" {
		if rank, convErr := strconv.Atoi(rankRaw); convErr == nil {
			patched, patchErr := h.geoip.Patch(c.Request.Context(), row.ID, service.GeoipWrite{Rank: &rank})
			if patchErr == nil {
				row = patched
			}
		}
	}
	response.JSON(c, http.StatusCreated, publicGeoip(row))
}

func (h *geoipHandler) patch(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
		return
	}
	var req geoipPatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	row, err := h.geoip.Patch(c.Request.Context(), id, service.GeoipWrite{Name: req.Name, Enabled: req.Enabled, Rank: req.Rank})
	if err != nil {
		writeGeoipErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicGeoip(row))
}

func (h *geoipHandler) remove(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
		return
	}
	if err := h.geoip.Delete(c.Request.Context(), id); err != nil {
		writeGeoipErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeGeoipErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrGeoipNotFound):
		response.Error(c, http.StatusNotFound, "GEOIP_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, service.ErrGeoipLimit), errors.Is(err, service.ErrInvalidGeoip):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	default:
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}

func stringsOr(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}
