package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/logger"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

func TestRecoveryWritesJSONNotHTML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Recovery(zerolog.Nop()))
	engine.GET("/boom", func(c *gin.Context) {
		panic("boom")
	})

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type=%q", ct)
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "<html") {
		t.Fatalf("html body: %s", w.Body.String())
	}
	var env response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("code=%q", env.Error.Code)
	}
}

func TestAccessLogIncludesRequestAndContextFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, zerolog.InfoLevel).With().
		Str(logger.FieldCat, logger.CatAccess).
		Str(logger.FieldPlane, logger.PlaneAdmin).
		Logger()
	engine := gin.New()
	engine.Use(RequestID(), AccessLog(log))
	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x?device_id=PLAINTEXT-DEVICE", nil)
	req.Header.Set("X-Request-Id", "rid-1")
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}

	var evt map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &evt); err != nil {
		t.Fatalf("log json: %v raw=%s", err, buf.String())
	}
	if evt["request_id"] != "rid-1" || evt["method"] != http.MethodGet || evt["path"] != "/x" {
		t.Fatalf("request fields=%v", evt)
	}
	if _, hasMod := evt[logger.FieldMod]; hasMod {
		t.Fatalf("access log must not set mod: %v", evt)
	}
	if strings.Contains(buf.String(), "PLAINTEXT-DEVICE") {
		t.Fatal("access log must not include raw query device_id")
	}
	status, ok := evt["status"].(float64)
	if !ok || int(status) != http.StatusNoContent {
		t.Fatalf("status=%v", evt["status"])
	}
	if _, ok := evt["elapsed"]; !ok {
		t.Fatalf("elapsed missing: %v", evt)
	}
	if evt[logger.FieldCat] != logger.CatAccess || evt[logger.FieldPlane] != logger.PlaneAdmin {
		t.Fatalf("context fields=%v", evt)
	}
}
