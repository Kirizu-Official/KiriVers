package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func TestProjectLanguageHTTPAcceptance(t *testing.T) {
	r, _, auditStore, adminTok := setupAnnouncementHTTP(t)

	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]any{
		"slug": "lang-omit",
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("omit default_locale=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]any{
		"slug":           "lang-app",
		"default_locale": "zh-CN",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/lang-app/languages", adminTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	var listBody struct {
		Languages []struct {
			Code      string `json:"code"`
			IsDefault bool   `json:"is_default"`
		} `json:"languages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Languages) != 1 || listBody.Languages[0].Code != "zh-CN" || !listBody.Languages[0].IsDefault {
		t.Fatalf("seeded=%s", w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/lang-app/languages", adminTok, map[string]any{
		"code":         "ja",
		"display_name": "日本語",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("add ja=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/lang-app/languages", adminTok, map[string]any{
		"code": "JA",
	})
	if w.Code != http.StatusConflict || decodeErr(t, w) != "LANGUAGE_TAKEN" {
		t.Fatalf("dup ja=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodDelete, "/api/v1/admin/projects/lang-app/languages/zh-CN", adminTok, nil)
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("delete default=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/lang-app/languages/ja", adminTok, map[string]any{
		"is_default": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("set default ja=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/lang-app/announcements", adminTok, map[string]any{
		"language": "zh-CN", "title": "你好", "content": "中文",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create announcement=%d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/lang-app/announcements", adminTok, map[string]any{
		"language": "ja", "title": "こんにちは", "content": "日本語",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create ja announcement=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/lang-app/announcements/"+id, adminTok, map[string]any{
		"title": "你好", "content": "中文",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch announcement=%d %s", w.Code, w.Body.String())
	}
	var patched map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched["language"] != "zh-CN" || patched["content"] != "中文" {
		t.Fatalf("zh row=%s", w.Body.String())
	}
	listed := doJSON(r, http.MethodGet, "/api/v1/admin/projects/lang-app/announcements", adminTok, nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"こんにちは"`) || strings.Contains(listed.Body.String(), `"日本語"`) {
		t.Fatalf("ja index row should keep title and omit content: %s", listed.Body.String())
	}

	readTok := issueReadToken(t, r, adminTok, "lang-app")
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/lang-app/languages", readTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("read token GET languages=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/lang-app/languages", readTok, map[string]any{"code": "ko"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("read token POST language=%d %s", w.Code, w.Body.String())
	}

	got := auditActions(auditStore)
	wantCreate, wantUpdate := false, false
	for _, action := range got {
		if action == model.AuditActionLanguageCreate {
			wantCreate = true
		}
		if action == model.AuditActionLanguageUpdate {
			wantUpdate = true
		}
	}
	if !wantCreate || !wantUpdate {
		t.Fatalf("audit actions=%v", got)
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/lang-app/languages/missing", adminTok, map[string]any{
		"display_name": "nope",
	})
	if w.Code != http.StatusNotFound || decodeErr(t, w) != "LANGUAGE_NOT_FOUND" {
		t.Fatalf("unknown code=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodDelete, "/api/v1/admin/projects/lang-app/languages/zh-CN", adminTok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete previous default=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodDelete, "/api/v1/admin/projects/lang-app/languages/ja", adminTok, nil)
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("delete last/default=%d %s", w.Code, w.Body.String())
	}
}

func TestLanguageHTTPUnauthorized(t *testing.T) {
	r, _, _, _ := setupProjectHTTP(t)
	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects/missing/languages", "", nil)
	if w.Code != http.StatusUnauthorized || decodeErr(t, w) != "UNAUTHORIZED" {
		t.Fatalf("unauth=%d %s", w.Code, w.Body.String())
	}
}
