package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestErrorShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Error(c, http.StatusBadRequest, "ENGINE_MISMATCH", "invalid semver", map[string]string{"field": "version_semver"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
	var body Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "ENGINE_MISMATCH" || body.Error.Message == "" {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.Error.Details == nil {
		t.Fatal("details missing")
	}
}
