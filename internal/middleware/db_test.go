package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

func TestDBUnavailableNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	down := false
	engine := gin.New()
	engine.Use(DBUnavailableNotFound(func() bool { return !down }))
	engine.GET("/api/v1/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "ready": false}) })
	engine.GET("/api/v1/openapi.json", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"openapi": "3"}) })
	engine.GET("/api/v1/projects/:ref", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	engine.GET("/login", func(c *gin.Context) { c.String(http.StatusOK, "ui") })

	assertStatus := func(path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != want {
			t.Fatalf("%s status=%d want %d body=%s", path, w.Code, want, w.Body.String())
		}
	}

	assertStatus("/api/v1/projects/demo", http.StatusOK)
	down = true
	assertStatus("/api/v1/health", http.StatusOK)
	assertStatus("/api/v1/openapi.json", http.StatusOK)
	assertStatus("/login", http.StatusOK)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects/demo", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("api status=%d want 404", w.Code)
	}
	var env response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Fatalf("code=%q", env.Error.Code)
	}

	engineNil := gin.New()
	engineNil.Use(DBUnavailableNotFound(nil))
	engineNil.GET("/api/v1/projects/:ref", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w = httptest.NewRecorder()
	engineNil.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects/demo", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("nil available status=%d", w.Code)
	}
}
