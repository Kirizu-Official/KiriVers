package store

import "github.com/Kirizu-Official/KiriVers/internal/model"

func testListing(protocol string, ids map[string]string) *model.StoreListing {
	m := model.IdentifierMap{}
	for k, v := range ids {
		m[k] = v
	}
	return &model.StoreListing{
		Protocol:      protocol,
		Slug:          "default",
		Enabled:       true,
		PackageSource: model.PackageSourceLineFull,
		Identifiers:   m,
	}
}
