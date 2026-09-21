package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

func TestStoreAuthIndependentToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	svc := service.NewProjectService(store)
	slug := "feed-app"
	feed := "feed-secret-token"
	locale := "en"
	_, _, err := svc.Create(t.Context(), service.CreateProjectInput{
		Slug:          &slug,
		DefaultLocale: &locale,
		StoreToken:     &feed,
	})
	if err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	engine.GET("/api/v1/feeds/:project_ref", StoreAuth(svc), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feeds/feed-app", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing feed token=%d %s", w.Code, w.Body.String())
	}
	var env response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("code=%q", env.Error.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/feeds/feed-app", nil)
	req.Header.Set("Authorization", "Bearer "+feed)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("feed token=%d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/feeds/missing", nil)
	req.Header.Set("Authorization", "Bearer "+feed)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing project=%d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "PROJECT_NOT_FOUND" {
		t.Fatalf("code=%q", env.Error.Code)
	}
}

func TestClientProjectResolveSkipsToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	svc := service.NewProjectService(store)
	slug := "media-app"
	requireToken := true
	locale := "en"
	_, _, err := svc.Create(t.Context(), service.CreateProjectInput{
		Slug:               &slug,
		DefaultLocale:      &locale,
		RequireClientToken: &requireToken,
	})
	if err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	engine.GET("/api/v1/projects/:project_ref/media/:id", ClientProjectResolve(svc), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/media-app/media/"+uuidDummy(), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("resolve without token=%d %s", w.Code, w.Body.String())
	}

	auth := gin.New()
	auth.GET("/api/v1/projects/:project_ref/x", ClientProjectAuth(svc), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/media-app/x", nil)
	w = httptest.NewRecorder()
	auth.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("auth without token=%d %s", w.Code, w.Body.String())
	}
}

func TestRequestOriginUsesForwardedProto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/x", func(c *gin.Context) {
		c.String(http.StatusOK, RequestOrigin(c))
	})

	req := httptest.NewRequest(http.MethodGet, "http://api.test/x", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Body.String() != "http://api.test" {
		t.Fatalf("plain origin=%q", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "http://api.test/x", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Body.String() != "https://api.test" {
		t.Fatalf("forwarded origin=%q", w.Body.String())
	}
}

func uuidDummy() string {
	return "00000000-0000-0000-0000-000000000001"
}
