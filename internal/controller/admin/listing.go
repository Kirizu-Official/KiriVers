package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type storeListingWriteReq struct {
	Protocol      *string              `json:"protocol"`
	Slug          *string              `json:"slug"`
	Enabled       *bool                `json:"enabled"`
	OS            *string              `json:"os"`
	Arch          *string              `json:"arch"`
	Channel       *string              `json:"channel"`
	Identifiers   *model.IdentifierMap `json:"identifiers"`
	PackageSource *string              `json:"package_source"`
	ManifestPath  *string              `json:"manifest_path"`
}

func (r storeListingWriteReq) toInput() service.StoreListingWrite {
	in := service.StoreListingWrite{
		Protocol:      r.Protocol,
		Slug:          r.Slug,
		Enabled:       r.Enabled,
		Identifiers:   r.Identifiers,
		PackageSource: r.PackageSource,
		ManifestPath:  r.ManifestPath,
	}
	if r.OS != nil {
		if *r.OS == "" {
			in.ClearOS = true
		} else {
			in.OS = r.OS
		}
	}
	if r.Arch != nil {
		if *r.Arch == "" {
			in.ClearArch = true
		} else {
			in.Arch = r.Arch
		}
	}
	if r.Channel != nil {
		if *r.Channel == "" {
			in.ClearChannel = true
		} else {
			in.Channel = r.Channel
		}
	}
	return in
}

func (h *projectHandler) listStoreListings(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	list, err := h.projects.ListStoreListings(c.Request.Context(), p.ID)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	out := make([]gin.H, 0, len(list))
	for i := range list {
		out = append(out, publicStoreListing(&list[i], p.Slug))
	}
	response.JSON(c, http.StatusOK, gin.H{"listings": out})
}

func (h *projectHandler) createStoreListing(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req storeListingWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	row, err := h.projects.CreateStoreListing(c.Request.Context(), p.ID, req.toInput())
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicStoreListing(row, p.Slug))
}

func (h *projectHandler) patchStoreListing(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	id, err := uuid.Parse(c.Param("listing_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid listing id", nil)
		return
	}
	var req storeListingWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	row, err := h.projects.PatchStoreListing(c.Request.Context(), p.ID, id, req.toInput())
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicStoreListing(row, p.Slug))
}

func (h *projectHandler) deleteStoreListing(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	id, err := uuid.Parse(c.Param("listing_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid listing id", nil)
		return
	}
	if err := h.projects.DeleteStoreListing(c.Request.Context(), p.ID, id); err != nil {
		writeProjectErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func publicStoreListing(row *model.StoreListing, projectSlug string) gin.H {
	ids := row.Identifiers
	if ids == nil {
		ids = model.IdentifierMap{}
	}
	return gin.H{
		"id":             row.ID,
		"project_id":     row.ProjectID,
		"protocol":       row.Protocol,
		"slug":           row.Slug,
		"enabled":        row.Enabled,
		"os":             row.OS,
		"arch":           row.Arch,
		"channel":        row.Channel,
		"identifiers":    ids,
		"package_source": row.PackageSource,
		"manifest_path":  row.ManifestPath,
		"store_url":      storeURLForListing(projectSlug, row.Protocol, row.Slug),
		"created_at":     formatTimeUTC(row.CreatedAt),
		"updated_at":     formatTimeUTC(row.UpdatedAt),
	}
}

func storeURLForListing(projectSlug, protocol, slug string) string {
	doc := defaultStoreDoc(protocol)
	return "/api/v1/projects/" + projectSlug + "/store/" + protocol + "/" + slug + "/" + doc
}

func defaultStoreDoc(protocol string) string {
	switch protocol {
	case "sparkle":
		return "appcast.xml"
	case "electron":
		return "latest.yml"
	case "squirrel":
		return "RELEASES"
	case "tauri":
		return "update"
	case "appimage":
		return "latest"
	default:
		return "index"
	}
}
