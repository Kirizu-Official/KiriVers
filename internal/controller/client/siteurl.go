package client

import (
	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

func markdownOrigin(c *gin.Context) string {
	return service.OriginFromReferer(c.GetHeader("Referer"), middleware.RequestOrigin(c))
}

func expandChangelogBody(c *gin.Context, body *update.ChangelogResponse) {
	if body == nil {
		return
	}
	origin := middleware.RequestOrigin(c)
	body.Changelog = service.ExpandSiteURL(body.Changelog, origin)
	for i := range body.ChangelogVersions {
		body.ChangelogVersions[i].Changelog = service.ExpandSiteURL(body.ChangelogVersions[i].Changelog, origin)
	}
}
