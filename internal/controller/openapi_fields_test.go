package controller

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// forbiddenOpenAPITokens must not appear as OpenAPI property names, parameter
// names, or paths keys on either plane. Descriptions must not re-teach them
// unless listed in forbiddenDescriptionExceptions.
var forbiddenOpenAPITokens = []string{
	"display_version",
	"server_protocol",
	"min_client_protocol",
	"store_protocols",
	"protocol_version",
	"dirty_paths",
	"/feed/",
}

// forbiddenDescriptionExceptions is a tight allow-list of description
// substrings that may mention a rejected name (e.g. a 400 that must name the
// query it rejects). Prefer rewriting the description instead of adding to this
// list. Empty after the alignment sweep: check 400 no longer names leftover
// query tokens.
var forbiddenDescriptionExceptions []string

var forbiddenIdent = regexp.MustCompile(`\b(display_version|server_protocol|min_client_protocol|store_protocols|protocol_version|dirty_paths)\b`)

type forbiddenHit struct {
	Kind  string // property|parameter|path|description
	Token string
	Where string
}

func TestOpenAPIForbiddenNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		plane string
		spec  []byte
	}{
		{"admin", adminOpenAPISpec},
		{"client", clientOpenAPISpec},
	} {
		t.Run(tc.plane, func(t *testing.T) {
			hits := collectForbiddenHits(t, tc.spec)
			if len(hits) == 0 {
				return
			}
			for _, h := range hits {
				t.Errorf("%s %s %q at %s", tc.plane, h.Kind, h.Token, h.Where)
			}
		})
	}
}

// TestOpenAPIForbiddenNamesWouldFailOnReinsert is the AC4 negative proof: the
// gate fails if leftover names are reinserted as properties, parameters, or
// path keys. Fixture: testdata/openapi_forbidden_reinsert.json.
func TestOpenAPIForbiddenNamesWouldFailOnReinsert(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("testdata", "openapi_forbidden_reinsert.json"))
	if err != nil {
		t.Fatal(err)
	}
	hits := collectForbiddenHits(t, raw)
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.Token] = true
	}
	for _, tok := range forbiddenOpenAPITokens {
		if !seen[tok] {
			t.Errorf("fixture must trip the gate for %q; hits=%v", tok, hits)
		}
	}
}

func TestOpenAPIFieldTables(t *testing.T) {
	t.Parallel()
	adminDoc := unmarshalSpec(t, adminOpenAPISpec)
	clientDoc := unmarshalSpec(t, clientOpenAPISpec)

	assertSchemaKeys(t, adminDoc, "ClusterNode", ginHKeys(t, filepath.Join("admin", "nodes.go"), "publicNode"), nil)
	assertSchemaKeys(t, adminDoc, "NodeArtifactSync", ginHKeys(t, filepath.Join("admin", "nodes.go"), "publicNodeSync"), nil)
	assertSchemaKeys(t, adminDoc, "StoreListing", ginHKeys(t, filepath.Join("admin", "listing.go"), "publicStoreListing"), nil)
	assertSchemaKeys(t, adminDoc, "ProjectStats", structJSONTags(t, filepath.Join("..", "service", "project_stats.go"), "ProjectStats"), nil)

	assertSchemaKeys(t, adminDoc, "StatusOutput", ginHKeys(t, "register.go", "healthPayload"), nil)
	assertSchemaKeys(t, clientDoc, "StatusOutput", ginHKeys(t, "register.go", "healthPayload"), nil)
	// 服务端编译信息：admin/buildinfo.go 的 buildInfoPayload 必须只有一处 gin.H
	// 字面量且键为字面字符串，否则这里会以 spec extra / go extra 报漂移。
	assertSchemaKeys(t, adminDoc, "BuildInfo", ginHKeys(t, filepath.Join("admin", "buildinfo.go"), "buildInfoPayload"), nil)

	assertSchemaKeys(t, clientDoc, "UpdateCheck200", structJSONTags(t, filepath.Join("..", "service", "update", "check.go"), "CheckResponse"), nil)
	assertSchemaKeys(t, clientDoc, "ProjectPublic", ginHKeys(t, filepath.Join("client", "project.go"), "publicProjectSettings"), nil)
	assertSchemaKeys(t, clientDoc, "Diff200", structJSONTags(t, filepath.Join("..", "service", "update", "diff.go"), "DiffResponse"), nil)
	assertSchemaKeys(t, clientDoc, "Pack200", structJSONTags(t, filepath.Join("..", "service", "update", "pack.go"), "PackResponse"), nil)
	assertSchemaKeys(t, clientDoc, "PackRequest", structJSONTags(t, filepath.Join("client", "pack.go"), "packRequest"), nil)

	changelogKeys := structJSONTags(t, filepath.Join("..", "service", "update", "check.go"), "ChangelogResponse")
	assertResolvedKeys(t, successSchema(t, clientDoc, "/api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}", "get", "200"), changelogKeys, nil, "changelog 200")
}

func collectForbiddenHits(t *testing.T, spec []byte) []forbiddenHit {
	t.Helper()
	var doc any
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var hits []forbiddenHit
	root, _ := doc.(map[string]any)
	if paths, ok := root["paths"].(map[string]any); ok {
		for p := range paths {
			if strings.Contains(p, "/feed/") {
				hits = append(hits, forbiddenHit{Kind: "path", Token: "/feed/", Where: p})
			}
		}
	}
	var walk func(node any, where string)
	walk = func(node any, where string) {
		switch v := node.(type) {
		case map[string]any:
			if props, ok := v["properties"].(map[string]any); ok {
				for name, child := range props {
					if isForbiddenToken(name) {
						hits = append(hits, forbiddenHit{Kind: "property", Token: name, Where: where + ".properties"})
					}
					walk(child, where+".properties."+name)
				}
			}
			if name, _ := v["name"].(string); name != "" {
				if _, hasIn := v["in"]; hasIn && isForbiddenToken(name) {
					hits = append(hits, forbiddenHit{Kind: "parameter", Token: name, Where: where})
				}
			}
			if desc, ok := v["description"].(string); ok {
				hits = append(hits, descriptionForbiddenHits(desc, where)...)
			}
			for k, child := range v {
				if k == "properties" {
					continue
				}
				walk(child, where+"."+k)
			}
		case []any:
			for i, child := range v {
				walk(child, where+"["+strconv.Itoa(i)+"]")
			}
		}
	}
	walk(doc, "$")
	return hits
}

func isForbiddenToken(name string) bool {
	if strings.Contains(name, "/feed/") {
		return true
	}
	for _, tok := range forbiddenOpenAPITokens {
		if tok == "/feed/" {
			continue
		}
		if name == tok {
			return true
		}
	}
	return false
}

func descriptionForbiddenHits(desc, where string) []forbiddenHit {
	if desc == "" {
		return nil
	}
	for _, ex := range forbiddenDescriptionExceptions {
		if ex != "" && strings.Contains(desc, ex) {
			return nil
		}
	}
	var hits []forbiddenHit
	if strings.Contains(desc, "/feed/") {
		hits = append(hits, forbiddenHit{Kind: "description", Token: "/feed/", Where: where})
	}
	for _, m := range forbiddenIdent.FindAllString(desc, -1) {
		hits = append(hits, forbiddenHit{Kind: "description", Token: m, Where: where})
	}
	return hits
}

func unmarshalSpec(t *testing.T, spec []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func assertSchemaKeys(t *testing.T, doc map[string]any, schema string, goKeys []string, writeOnly []string) {
	t.Helper()
	sch := resolveSchema(t, doc, map[string]any{"$ref": "#/components/schemas/" + schema})
	assertResolvedKeys(t, sch, goKeys, writeOnly, schema)
}

func successSchema(t *testing.T, doc map[string]any, path, method, status string) map[string]any {
	t.Helper()
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths[path].(map[string]any)
	op, _ := item[method].(map[string]any)
	resps, _ := op["responses"].(map[string]any)
	resp, _ := resps[status].(map[string]any)
	content, _ := resp["content"].(map[string]any)
	appJSON, _ := content["application/json"].(map[string]any)
	raw, _ := appJSON["schema"].(map[string]any)
	return resolveSchema(t, doc, raw)
}

func assertResolvedKeys(t *testing.T, schema map[string]any, goKeys, writeOnly []string, label string) {
	t.Helper()
	if schema == nil {
		t.Fatalf("%s: schema is nil", label)
	}
	if v, ok := schema["additionalProperties"]; !ok || v != false {
		t.Errorf("%s: additionalProperties must be false, got %#v", label, schema["additionalProperties"])
	}
	props, _ := schema["properties"].(map[string]any)
	specKeys := make([]string, 0, len(props))
	writeOnlySet := toSet(writeOnly)
	for name, raw := range props {
		if writeOnlySet[name] {
			pm, _ := raw.(map[string]any)
			if pm["writeOnly"] != true {
				t.Errorf("%s: %s must be writeOnly in OpenAPI", label, name)
			}
			continue
		}
		specKeys = append(specKeys, name)
	}
	sort.Strings(specKeys)
	want := append([]string(nil), goKeys...)
	sort.Strings(want)
	if !reflect.DeepEqual(specKeys, want) {
		t.Errorf("%s keys mismatch\n  spec extra: %v\n  go extra:   %v\n  spec: %v\n  go:   %v",
			label, subtract(specKeys, want), subtract(want, specKeys), specKeys, want)
	}
}

func resolveSchema(t *testing.T, doc, sch map[string]any) map[string]any {
	t.Helper()
	if sch == nil {
		return nil
	}
	if ref, ok := sch["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		comps, _ := doc["components"].(map[string]any)
		schemas, _ := comps["schemas"].(map[string]any)
		raw, ok := schemas[name].(map[string]any)
		if !ok {
			t.Fatalf("unresolvable schema $ref %s", ref)
		}
		return raw
	}
	return sch
}

func ginHKeys(t *testing.T, relPath, funcName string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, relPath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	seen := map[string]bool{}
	add := func(k string) {
		if k == "" || seen[k] {
			return
		}
		seen[k] = true
		keys = append(keys, k)
	}
	var fn *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if ok && fd.Name.Name == funcName {
			fn = fd
			return false
		}
		return true
	})
	if fn == nil {
		t.Fatalf("function %s not found in %s", funcName, relPath)
	}
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			for _, elt := range x.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if lit, ok := kv.Key.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						add(s)
					}
				}
			}
		case *ast.IndexExpr:
			if lit, ok := x.Index.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					add(s)
				}
			}
		}
		return true
	})
	if len(keys) == 0 {
		t.Fatalf("no gin.H keys extracted from %s %s", relPath, funcName)
	}
	return keys
}

func structJSONTags(t *testing.T, relPath, typeName string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, relPath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != typeName {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
			name := tag.Get("json")
			if name == "" || name == "-" {
				continue
			}
			name = strings.Split(name, ",")[0]
			if name == "" || name == "-" {
				continue
			}
			keys = append(keys, name)
		}
		return false
	})
	if len(keys) == 0 {
		t.Fatalf("no json tags on %s in %s", typeName, relPath)
	}
	return keys
}

func toSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s] = true
	}
	return out
}

func subtract(a, b []string) []string {
	bs := toSet(b)
	var out []string
	for _, s := range a {
		if !bs[s] {
			out = append(out, s)
		}
	}
	return out
}
