package controller

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/urlsign"
)

func fullTestEngines(t *testing.T) (clientEngine, adminEngine *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	projStore := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(projStore, backend)
	jobRepo := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobRepo)
	projSvc.SetInstallPolicyStore(repository.NewMemoryInstallPolicyRuleStore())
	projSvc.SetWebhookStore(repository.NewMemoryWebhookStore())

	adminStore := repository.NewMemoryAdminStore()
	adminSvc := service.NewTestAdminService(adminStore)

	telemStore := repository.NewMemoryTelemetryStore()
	telemSvc := service.NewTelemetryService(telemStore)

	auditStore := repository.NewMemoryAuditStore()
	auditSvc := service.NewAuditService(auditStore, zerolog.Nop())

	announceSvc := service.NewAnnouncementService(repository.NewMemoryAnnouncementStore(), projStore)
	mediaSvc := service.NewMediaService(repository.NewMemoryProjectMediaStore(), backend, nil)
	geoipSvc := service.NewGeoipService(repository.NewMemoryGeoipStore(), backend, t.TempDir())
	projSvc.SetGeoipLookup(geoipSvc.LookupFn())

	catRepo := repository.NewMemoryUpdateCatalog(projStore)
	updates := update.NewService(catRepo)

	limiter := middleware.NewLimiter()
	signer := urlsign.NewSigner("signer-secret", time.Hour)

	deps := Deps{
		Admin:         adminSvc,
		Project:       projSvc,
		Updates:       updates,
		Telemetry:     telemSvc,
		Limiter:       limiter,
		Signer:        signer,
		Audit:         auditSvc,
		Announcements: announceSvc,
		Media:         mediaSvc,
		GeoIP:         geoipSvc,
		Ready:         func(ctx context.Context) error { return nil },
	}
	clientEngine = gin.New()
	RegisterClient(clientEngine, deps)
	adminEngine = gin.New()
	RegisterAdmin(adminEngine, deps)
	return clientEngine, adminEngine
}

// TestOpenAPIRoutesSync 按平面校验路由与契约双向精确匹配：
// client 引擎 ↔ openapi.client.json、admin 引擎 ↔ openapi.admin.json。
func TestOpenAPIRoutesSync(t *testing.T) {
	clientEngine, adminEngine := fullTestEngines(t)

	t.Run("client", func(t *testing.T) {
		syncEngineWithSpec(t, clientEngine, clientOpenAPISpec)
	})
	t.Run("admin", func(t *testing.T) {
		syncEngineWithSpec(t, adminEngine, adminOpenAPISpec)
	})
}

// syncEngineWithSpec 断言引擎注册的路由与 spec paths 双向精确一致。
func syncEngineWithSpec(t *testing.T, engine *gin.Engine, spec []byte) {
	t.Helper()

	var doc struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatalf("failed to unmarshal spec: %v", err)
	}

	type RouteKey struct {
		Method string
		Path   string
	}

	openAPIRoutes := make(map[RouteKey]bool)
	for path, ops := range doc.Paths {
		for method := range ops {
			if strings.EqualFold(method, "parameters") {
				continue
			}
			openAPIRoutes[RouteKey{
				Method: strings.ToUpper(method),
				Path:   path,
			}] = true
		}
	}

	// paramRegex converts :name to {name}
	paramRegex := regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

	ginRoutes := make(map[RouteKey]bool)
	for _, route := range engine.Routes() {
		normPath := paramRegex.ReplaceAllString(route.Path, "{$1}")
		if strings.HasSuffix(normPath, "/*doc") {
			normPath = strings.TrimSuffix(normPath, "/*doc") + "/{doc}"
		}

		ginRoutes[RouteKey{
			Method: strings.ToUpper(route.Method),
			Path:   normPath,
		}] = true
	}

	t.Logf("Total Gin routes: %d, Total OpenAPI operations: %d", len(ginRoutes), len(openAPIRoutes))

	// Check for Gin routes missing in OpenAPI
	var missingInOpenAPI []RouteKey
	for rk := range ginRoutes {
		if !openAPIRoutes[rk] {
			missingInOpenAPI = append(missingInOpenAPI, rk)
		}
	}

	// Check for OpenAPI operations missing in Gin
	var missingInGin []RouteKey
	for rk := range openAPIRoutes {
		if !ginRoutes[rk] {
			missingInGin = append(missingInGin, rk)
		}
	}

	for _, rk := range missingInOpenAPI {
		t.Errorf("Route registered in Gin but MISSING in OpenAPI: %s %s", rk.Method, rk.Path)
	}
	for _, rk := range missingInGin {
		t.Errorf("Operation defined in OpenAPI but NOT registered in Gin: %s %s", rk.Method, rk.Path)
	}
}
