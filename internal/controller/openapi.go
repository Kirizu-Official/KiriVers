package controller

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// 双平面 OpenAPI 契约是唯一源：openapi.admin.json / openapi.client.json。
// 各平面引擎只在 /api/v1/openapi.json 暴露本平面路径。
// TestOpenAPIRoutesSync 校验 Gin 路由与契约双向精确匹配；
// TestPlaneSpecsValid 校验路径分区、共享端点与 $ref 闭包；
// TestOpenAPIForbiddenNames / TestOpenAPIFieldTables 校验禁止字段名与
// public*/json tag 键表（路由闸门不够）。
//
//go:embed openapi.admin.json
var adminOpenAPISpec []byte

//go:embed openapi.client.json
var clientOpenAPISpec []byte

// serveOpenAPI 返回本平面嵌入的 OpenAPI 3 JSON。
func serveOpenAPI(spec []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(spec) == 0 {
			response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "openapi spec missing", nil)
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", spec)
	}
}
