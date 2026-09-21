package controller

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// adminPathPrefix 管理平面路由前缀（与 internal/controller/admin.Register 的挂载点一致）。
const adminPathPrefix = "/api/v1/admin"

// sharedSpecPaths 双平面都要提供的公共端点：探活与契约自身。
var sharedSpecPaths = []string{
	"/api/v1/health",
	"/api/v1/openapi.json",
}

// TestPlaneSpecsValid 校验两份平面契约：路径分区、共享端点、$ref 闭包、info.title。
// 契约源就是仓库内的 openapi.admin.json / openapi.client.json，不再从合并 master 派生。
func TestPlaneSpecsValid(t *testing.T) {
	adminSpec, err := os.ReadFile("openapi.admin.json")
	if err != nil {
		t.Fatal(err)
	}
	clientSpec, err := os.ReadFile("openapi.client.json")
	if err != nil {
		t.Fatal(err)
	}
	assertPlaneSpec(t, "admin", adminSpec)
	assertPlaneSpec(t, "client", clientSpec)
}

// assertPlaneSpec 校验单份平面契约：路径分区与共享端点、$ref 闭包完整性、info.title。
func assertPlaneSpec(t *testing.T, plane string, spec []byte) {
	t.Helper()
	var doc struct {
		Info struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas         map[string]json.RawMessage `json:"schemas"`
			Parameters      map[string]json.RawMessage `json:"parameters"`
			SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatalf("%s spec: %v", plane, err)
	}

	wantTitle := "KiriVers Admin API"
	if plane == "client" {
		wantTitle = "KiriVers Client API"
	}
	if doc.Info.Title != wantTitle {
		t.Errorf("%s spec title = %q, want %q", plane, doc.Info.Title, wantTitle)
	}

	for _, shared := range sharedSpecPaths {
		if _, ok := doc.Paths[shared]; !ok {
			t.Errorf("%s spec missing shared path %s", plane, shared)
		}
	}
	for p := range doc.Paths {
		isAdmin := strings.HasPrefix(p, adminPathPrefix)
		if plane == "admin" && !isAdmin && !containsString(sharedSpecPaths, p) {
			t.Errorf("%s spec has non-admin path %s", plane, p)
		}
		if plane == "client" && isAdmin {
			t.Errorf("%s spec has admin path %s", plane, p)
		}
	}

	resolvable := func(typ, name string) bool {
		switch typ {
		case "schemas":
			_, ok := doc.Components.Schemas[name]
			return ok
		case "parameters":
			_, ok := doc.Components.Parameters[name]
			return ok
		case "securitySchemes":
			_, ok := doc.Components.SecuritySchemes[name]
			return ok
		}
		return false
	}
	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				typ, name, ok := parseComponentRef(ref)
				if ok && !resolvable(typ, name) {
					t.Errorf("%s spec has unresolvable $ref %s", plane, ref)
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	var generic any
	if err := json.Unmarshal(spec, &generic); err != nil {
		t.Fatalf("%s spec reparse: %v", plane, err)
	}
	walk(generic)
}

// parseComponentRef 解析 `#/components/<type>/<name>` 形式的内部引用。
func parseComponentRef(ref string) (typ, name string, ok bool) {
	const prefix = "#/components/"
	if !strings.HasPrefix(ref, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(ref, prefix)
	typ, name, found := strings.Cut(rest, "/")
	if !found || typ == "" || name == "" {
		return "", "", false
	}
	return typ, name, true
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
