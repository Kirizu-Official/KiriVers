package store

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"gopkg.in/yaml.v3"
)

// ---------- latest*.yml 解析夹具（对照 electron-updater generic 文档字段集） ----------

// electronFile 是 files 数组单项（electron-updater ≥6 读取）。
type electronFile struct {
	URL    string
	SHA512 string
	Size   int64
}

// electronDoc 是 latest*.yml 的解析目标；字段集 = electron-updater generic
// provider 文档（version/path/sha512/size/releaseDate/files[].{url,sha512,size}）。
// 逐行解析与 renderElectronYAML 的固定排版一一对应；解析同时校验出现顺序。
type electronDoc struct {
	Version     string
	Path        string
	SHA512      string
	Size        int64
	ReleaseDate string
	Files       []electronFile
	// order 记录顶层键出现顺序（version → path → sha512 → size → releaseDate → files）。
	order []string
}

// parseElectronYAML 解析适配器渲染的 latest*.yml（行式结构，非通用 YAML 解析器：
// go.mod 无直接 YAML 依赖，测试侧只解析自身固定排版）。
func parseElectronYAML(t *testing.T, body []byte) *electronDoc {
	t.Helper()
	doc := &electronDoc{}
	inFiles := false
	for _, line := range strings.Split(string(body), "\n") {
		if line == "" {
			continue
		}
		switch {
		case line == "files:":
			inFiles = true
			doc.order = append(doc.order, "files")
		case strings.HasPrefix(line, "  - url: "):
			if !inFiles {
				t.Fatalf("files entry before files key:\n%s", body)
			}
			doc.Files = append(doc.Files, electronFile{URL: unquoteYAML(t, strings.TrimPrefix(line, "  - url: "))})
		case strings.HasPrefix(line, "    sha512: "):
			if len(doc.Files) == 0 {
				t.Fatalf("file sha512 without url:\n%s", body)
			}
			doc.Files[len(doc.Files)-1].SHA512 = unquoteYAML(t, strings.TrimPrefix(line, "    sha512: "))
		case strings.HasPrefix(line, "    size: "):
			if len(doc.Files) == 0 {
				t.Fatalf("file size without url:\n%s", body)
			}
			n, err := strconv.ParseInt(strings.TrimPrefix(line, "    size: "), 10, 64)
			if err != nil {
				t.Fatalf("file size not integer: %q", line)
			}
			doc.Files[len(doc.Files)-1].Size = n
		default:
			key, value, ok := strings.Cut(line, ": ")
			if !ok || strings.HasPrefix(line, " ") {
				t.Fatalf("unexpected line:\n%s", body)
			}
			inFiles = false
			switch key {
			case "version":
				doc.Version = unquoteYAML(t, value)
			case "path":
				doc.Path = unquoteYAML(t, value)
			case "sha512":
				doc.SHA512 = unquoteYAML(t, value)
			case "size":
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					t.Fatalf("size not integer: %q", line)
				}
				doc.Size = n
			case "releaseDate":
				doc.ReleaseDate = unquoteYAML(t, value)
			default:
				t.Fatalf("unexpected key %q (not in electron-updater field set):\n%s", key, body)
			}
			doc.order = append(doc.order, key)
		}
	}
	return doc
}

// unquoteYAML 还原 yamlScalar 的双引号标量；无引号值原样返回。
func unquoteYAML(t *testing.T, v string) string {
	t.Helper()
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		s, err := strconv.Unquote(v)
		if err != nil {
			t.Fatalf("invalid quoted scalar %q: %v", v, err)
		}
		return s
	}
	return v
}

// assertElectronFixture 断言 electron-updater 文档字段集齐全且值正确
// （验收项 1：yml 可被 electron-updater 文档字段集解析）。sha512 是可选
// 降级项（测试装配无存储依赖时缺省）；出现时必须语义正确且与 files 一致。
func assertElectronFixture(t *testing.T, doc *electronDoc) {
	t.Helper()
	if doc.Version == "" || doc.Path == "" || doc.Size <= 0 || doc.ReleaseDate == "" {
		t.Fatalf("incomplete electron yml: %+v", doc)
	}
	if len(doc.Files) != 1 {
		t.Fatalf("files entries = %d, want 1", len(doc.Files))
	}
	f := doc.Files[0]
	if f.URL == "" || f.Size != doc.Size {
		t.Fatalf("files entry incomplete or inconsistent: %+v (top size %d)", f, doc.Size)
	}
	if doc.SHA512 != "" {
		if f.SHA512 != doc.SHA512 {
			t.Fatalf("files sha512 %q != top sha512 %q", f.SHA512, doc.SHA512)
		}
		// sha512 语义：base64(原始 512-bit 摘要)——解码后必须恰好 64 字节。
		raw, err := base64.StdEncoding.DecodeString(doc.SHA512)
		if err != nil {
			t.Fatalf("sha512 not base64: %v", err)
		}
		if len(raw) != sha512.Size {
			t.Fatalf("sha512 decoded to %d bytes, want 64", len(raw))
		}
	}
}

// renderElectron 装配默认依赖并渲染 latest*.yml，返回解析后的文档与原始 body。
func renderElectron(t *testing.T, cat *update.Catalog, path string, mutate func(*Deps, *Request)) (*electronDoc, []byte) {
	t.Helper()
	adapter := NewElectronAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		OS:      "windows", // HTTP 层对 electron 请求无 os query；Adapter 以文件名为准
		Arch:    "x86_64",
		Path:    path,
	}
	if mutate != nil {
		mutate(deps, &req)
	}
	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}
	if resp.ContentType != electronContentType {
		t.Fatalf("content type = %q", resp.ContentType)
	}
	return parseElectronYAML(t, resp.Body), resp.Body
}

// newElectronCatalog 构造单版本 windows/x86_64 目录：默认变体
// {slug}-{ver}-windows-x86_64.exe（kind=full），可注入产物级与版本级调整。
type electronOpt func(*model.Version, *update.LineState, *update.ArtifactInfo)

// newElectronCatalog 生成 stable 渠道、rollout=100、semver+int 双号的目录。
func newElectronCatalog(t *testing.T, slug, semver string, versionInt int64, opts ...electronOpt) *update.Catalog {
	t.Helper()
	v := model.Version{
		ID:          uuid.New(),
		ProjectID:   testProjectID,
		ChannelSlug: "stable",
		Status:      model.VersionStatusPublished,
		PublishTime: &testPublishTime,
	}
	applyFeedGray(&v, 100, false)
	if semver != "" {
		v.VersionSemver = &semver
		v.VersionSemverCanonical = &semver
	}
	if versionInt > 0 {
		v.VersionInteger = &versionInt
	}
	fileName := slug + "-" + displayRef(semver, versionInt) + "-windows-x86_64.exe"
	line := update.LineState{
		ID:           uuid.New(),
		OS:           "windows",
		Arch:         "x86_64",
		Status:       model.VersionLineStatusReady,
		PacksReadyAt: packsReady(),
		FullPkgs:     []update.ArtifactInfo{},
	}
	pkg := update.ArtifactInfo{
		FileName:    fileName,
		Size:        123456,
		SHA256:      "sha-" + fileName,
		ContentType: "application/x-msdownload",
		StorageKey:  slug + "/sha-" + fileName,
	}
	line.FullPkgs = append(line.FullPkgs, pkg)
	for _, opt := range opts {
		opt(&v, &line, &line.FullPkgs[0])
	}
	return &update.Catalog{
		Project: newTestProject(slug),
		Channels: []update.ChannelInfo{
			{Slug: "stable", StabilityRank: 30, Enabled: true},
			{Slug: "beta", StabilityRank: 20, Enabled: true},
		},
		Matrix:    &update.MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile},
		EnabledOS: []string{"windows"},
		Versions:  []update.VersionState{{Version: v, Lines: []update.LineState{line}}},
	}
}

// TestElectronLatestYMLFixture 官方字段集 + 双号映射 + sha512 base64 语义
// （验收项 1；C17-2 / C17-3）。产物登记 SHA512（hex）→ yml 输出
// base64(原始摘要)，与对产物字节独立计算的 crypto/sha512 比对。
func TestElectronLatestYMLFixture(t *testing.T) {
	fileBytes := []byte("electron-installer-bytes-for-sha512-fixture-test")
	digest := sha512.Sum512(fileBytes)
	cat := newElectronCatalog(t, "demo", "1.1.0", 11, func(_ *model.Version, _ *update.LineState, pkg *update.ArtifactInfo) {
		pkg.SHA512 = hex.EncodeToString(digest[:])
	})

	doc, body := renderElectron(t, cat, "latest.yml", nil)
	assertElectronFixture(t, doc)

	// C17-2：version = version_semver。
	if doc.Version != "1.1.0" {
		t.Fatalf("version = %q", doc.Version)
	}
	// C17-3：path = 默认变体 kind=full 稳定文件名；size 一致。
	if doc.Path != "sha-demo-1.1.0-windows-x86_64.exe.exe" {
		t.Fatalf("path = %q", doc.Path)
	}
	if doc.Size != 123456 {
		t.Fatalf("size = %d", doc.Size)
	}
	// releaseDate = PublishTime RFC3339。
	if want := testPublishTime.UTC().Format("2006-01-02T15:04:05Z07:00"); doc.ReleaseDate != want {
		t.Fatalf("releaseDate = %q, want %q", doc.ReleaseDate, want)
	}
	// sha512 语义（§5.8）：对 fixture 字节独立验证 base64(digest)。
	if want := base64.StdEncoding.EncodeToString(digest[:]); doc.SHA512 != want {
		t.Fatalf("sha512 = %q, want %q", doc.SHA512, want)
	}
	// files[].url = /packages/ 稳定下载 URL。
	if want := "/api/v1/projects/demo/packages/sha-demo-1.1.0-windows-x86_64.exe"; doc.Files[0].URL != want {
		t.Fatalf("files url = %q, want %q", doc.Files[0].URL, want)
	}
	// 顶层字段顺序（electron-updater 不关心，但输出保持固定便于人工核对）。
	wantOrder := "version,path,sha512,size,releaseDate,files"
	if got := strings.Join(doc.order, ","); got != wantOrder {
		t.Fatalf("field order = %q, want %q\n%s", got, wantOrder, body)
	}
}

// TestElectronComputedSHA512FromStorage 产物未登记 SHA512（旧行）：首请求从
// 存储流式计算，与对字节独立计算的摘要一致；LRU 缓存后第二次渲染不再读文件。
// 刻意不回填数据库（见 electron.go 类型级注释）。
func TestElectronComputedSHA512FromStorage(t *testing.T) {
	fileBytes := []byte("legacy-artifact-bytes-without-registered-sha512")
	cat := newElectronCatalog(t, "legacy", "1.0.0", 10)
	key := cat.Versions[0].Lines[0].FullPkgs[0].StorageKey
	storage := newFakeStorage(map[string][]byte{key: fileBytes})

	adapter := NewElectronAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable", OS: "windows", Arch: "x86_64", Path: "latest.yml",
	}

	render := func() *electronDoc {
		resp, err := adapter.Render(context.Background(), deps, req)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		return parseElectronYAML(t, resp.Body)
	}

	doc := render()
	assertElectronFixture(t, doc)
	digest := sha512.Sum512(fileBytes)
	if want := base64.StdEncoding.EncodeToString(digest[:]); doc.SHA512 != want {
		t.Fatalf("computed sha512 = %q, want %q", doc.SHA512, want)
	}
	// 第二次渲染：LRU 命中，文件只读一次。
	if got := render().SHA512; got != doc.SHA512 {
		t.Fatal("sha512 must be stable across renders")
	}
	if n := storage.readCount(key); n != 1 {
		t.Fatalf("artifact must be read once (LRU), got %d", n)
	}
	// 数据库行不回填：目录快照中的 SHA512 仍为空。
	if cat.Versions[0].Lines[0].FullPkgs[0].SHA512 != "" {
		t.Fatal("computed sha512 must not be backfilled into the catalog")
	}
}

// TestElectronYAMLParsesWithRealParser 用真实 YAML 解析器（gopkg.in/yaml.v3，
// 模块既有间接依赖，测试引用不产生 go.mod 变更）独立验证渲染产物可解析且类型
// 正确：version 恒为字符串（整数构建号 "10" 不被解析成数字）、files 数组结构、
// 无未知键（验收项 1 的解析器级证明）。
func TestElectronYAMLParsesWithRealParser(t *testing.T) {
	// 整数构建号（无 semver）是最容易出类型问题的夹具：不加引号会被 YAML
	// 解析成 int 而非 string。
	cat := newElectronCatalog(t, "demo", "", 10, func(_ *model.Version, _ *update.LineState, pkg *update.ArtifactInfo) {
		pkg.SHA512 = hex.EncodeToString(sha512Sum512([]byte("real-parser-fixture-bytes")))
	})
	_, body := renderElectron(t, cat, "latest.yml", nil)

	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("real YAML parser failed:\n%s\nerr=%v", body, err)
	}
	wantKeys := []string{"version", "path", "sha512", "size", "releaseDate", "files"}
	if len(doc) != len(wantKeys) {
		t.Fatalf("top-level keys = %v, want exactly %v", keysOf(doc), wantKeys)
	}
	for _, k := range wantKeys {
		if _, ok := doc[k]; !ok {
			t.Fatalf("missing key %q in:\n%s", k, body)
		}
	}
	if v, ok := doc["version"].(string); !ok || v != "10" {
		t.Fatalf("version must parse as string \"10\", got %T %#v", doc["version"], doc["version"])
	}
	if _, ok := doc["path"].(string); !ok {
		t.Fatalf("path must parse as string, got %T", doc["path"])
	}
	if _, ok := doc["size"].(int); !ok {
		t.Fatalf("size must parse as integer, got %T", doc["size"])
	}
	files, ok := doc["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("files must parse as one-entry array, got %T %#v", doc["files"], doc["files"])
	}
	f, ok := files[0].(map[string]any)
	if !ok {
		t.Fatalf("files[0] must parse as mapping, got %T", files[0])
	}
	if _, ok := f["url"].(string); !ok {
		t.Fatalf("files[0].url must parse as string, got %T", f["url"])
	}
	// files[0].sha512 与顶层 sha512 一致（electron-updater ≥6 语义）。
	if f["sha512"] != doc["sha512"] {
		t.Fatalf("files sha512 %v != top sha512 %v", f["sha512"], doc["sha512"])
	}
}

// sha512Sum512 测试辅助：返回字节的 SHA-512 摘要。
func sha512Sum512(b []byte) []byte {
	s := sha512.Sum512(b)
	return s[:]
}

// keysOf 返回 map 的键列表（错误信息用）。
func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// TestElectronNoSemverIntegerVersion 无 SemVer 时 version 为整数字符串
// （验收项 2，C17-2），且必须带引号防止 YAML 解析成数字。
func TestElectronNoSemverIntegerVersion(t *testing.T) {
	cat := newElectronCatalog(t, "demo", "", 10)
	doc, body := renderElectron(t, cat, "latest.yml", nil)
	assertElectronFixture(t, doc)
	if doc.Version != "10" {
		t.Fatalf("version = %q, want \"10\"", doc.Version)
	}
	if !strings.Contains(string(body), `version: "10"`) {
		t.Fatalf("integer version must be quoted to stay a string:\n%s", body)
	}
	if !strings.Contains(string(body), "path: sha-demo-10-windows-x86_64.exe.exe") {
		t.Fatalf("path must use integer version ref:\n%s", body)
	}
}

// TestElectronGrayFallthrough 灰度 <100% 的非强制版本不参与 latest 选择，
// 回退到下一个可见版本（C17-5 / §4.4 匿名口径）。
func TestElectronGrayFallthrough(t *testing.T) {
	cat := newElectronCatalog(t, "demo", "1.2.0", 12, func(v *model.Version, _ *update.LineState, _ *update.ArtifactInfo) {
		v.GrayCompletedAt = nil
		v.GrayStartedAt = &testPublishTime
		v.GrayStartPercent = 50
	})
	older := newElectronCatalog(t, "demo", "1.1.0", 11)
	cat.Versions = append(cat.Versions, older.Versions[0])

	doc, _ := renderElectron(t, cat, "latest.yml", nil)
	assertElectronFixture(t, doc)
	if doc.Version != "1.1.0" {
		t.Fatalf("latest must fall through to next visible version, got %q", doc.Version)
	}
}

// TestElectronAllGray404 可见集为空 → 404（不发空 yml）。
func TestElectronAllGray404(t *testing.T) {
	cat := newElectronCatalog(t, "demo", "1.2.0", 12, func(v *model.Version, _ *update.LineState, _ *update.ArtifactInfo) {
		v.GrayCompletedAt = nil
		v.GrayStartedAt = &testPublishTime
		v.GrayStartPercent = 50
	})
	adapter := NewElectronAdapter()
	_, err := adapter.Render(context.Background(),
		&Deps{Updates: update.NewService(&fakeLoader{cat: cat})},
		Request{Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
			Channel: "stable", OS: "windows", Arch: "x86_64", Path: "latest.yml"})
	if err == nil {
		t.Fatal("empty visible set must fail")
	}
}

// TestElectronUnknownFileName 未知文件名 / 空路径 → ErrUnknownPath（HTTP 404）。
func TestElectronUnknownFileName(t *testing.T) {
	cat := newElectronCatalog(t, "demo", "1.1.0", 11)
	adapter := NewElectronAdapter()
	for _, path := range []string{"", "latest-beta.yml", "latest-old.yml", "appcast.xml"} {
		_, err := adapter.Render(context.Background(),
			&Deps{Updates: update.NewService(&fakeLoader{cat: cat})},
			Request{Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
				Channel: "stable", OS: "windows", Arch: "x86_64", Path: path})
		if err == nil {
			t.Fatalf("path %q must fail", path)
		}
	}
}

// TestElectronFileNameToOSMapping 三个文件名分别加载 windows/macos/linux 目录
// （C17-1）：latest-mac.yml 在仅登记 windows 版本的项目上 → 404（无可见集）。
func TestElectronFileNameToOSMapping(t *testing.T) {
	cat := newElectronCatalog(t, "demo", "1.1.0", 11)
	adapter := NewElectronAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable", Arch: "x86_64", Path: "latest-mac.yml",
	}
	if _, err := adapter.Render(context.Background(), deps, req); err == nil {
		t.Fatal("mac feed on windows-only catalog must 404")
	}

	// macos 目录存在时 latest-mac.yml 正常出 yml（os 由文件名决定，req.OS 不参与）。
	mac := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, nil, "")
	resp, err := NewElectronAdapter().Render(context.Background(),
		&Deps{Updates: update.NewService(&fakeLoader{cat: mac})},
		Request{Project: &model.Project{ID: mac.Project.ID, Slug: mac.Project.Slug},
			Channel: "stable", Arch: "x86_64", Path: "latest-mac.yml"})
	if err != nil {
		t.Fatalf("latest-mac.yml on macos catalog: %v", err)
	}
	doc := parseElectronYAML(t, resp.Body)
	assertElectronFixture(t, doc)
	if doc.Path != "aa.dmg" {
		t.Fatalf("mac feed path = %q", doc.Path)
	}
}

// TestElectronPrivateSignedURLAndDefaultVariant 私有项目：files[].url 带短时
// 签名而 path 保持裸稳定文件名（C17-3 / §13.7）；存在 hw 变体时仍指向默认
// 变体（C17-7，验收项 4 的 path 部分）。
func TestElectronPrivateSignedURLAndDefaultVariant(t *testing.T) {
	cat := newElectronCatalog(t, "demo", "1.1.0", 11, func(_ *model.Version, line *update.LineState, _ *update.ArtifactInfo) {
		hw := "revb"
		line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{
			FileName:    "demo-1.1.0-windows-x86_64-revb.exe",
			Size:        999,
			SHA256:      "sha-revb",
			ContentType: "application/x-msdownload",
			StorageKey:  "demo/sha-revb",
			HwRev:       &hw,
		})
	})
	cat.Project.StorageVisibility = model.StorageVisibilityPrivate

	doc, body := renderElectron(t, cat, "latest.yml", func(deps *Deps, _ *Request) { deps.Signer = stubSigner{} })
	assertElectronFixture(t, doc)
	// path：默认变体稳定文件名，不带签名 query。
	if doc.Path != "sha-demo-1.1.0-windows-x86_64.exe.exe" {
		t.Fatalf("path must be the default variant stable filename, got %q", doc.Path)
	}
	// files[].url：私有项目签名。
	if want := "/api/v1/projects/demo/packages/sha-demo-1.1.0-windows-x86_64.exe?exp=123&sig=abc"; doc.Files[0].URL != want {
		t.Fatalf("files url = %q, want signed %q", doc.Files[0].URL, want)
	}
	if strings.Contains(string(body), "revb") {
		t.Fatalf("non-default hw variant must not appear:\n%s", body)
	}
}

// TestElectronChannelQuery 非 stable 渠道可投影（渠道是分发权威，yml 无渠道
// 字段；渠道间靠 URL 区分）。
func TestElectronChannelQuery(t *testing.T) {
	cat := newElectronCatalog(t, "demo", "1.1.0-beta.1", 11, func(v *model.Version, _ *update.LineState, _ *update.ArtifactInfo) {
		v.ChannelSlug = "beta"
	})
	doc, _ := renderElectron(t, cat, "latest.yml", func(_ *Deps, req *Request) { req.Channel = "beta" })
	assertElectronFixture(t, doc)
	if doc.Version != "1.1.0-beta.1" {
		t.Fatalf("beta version = %q", doc.Version)
	}
}

// ---------- blockmap 回归（C17-4 / §7.2）：blockmap 不得进入差量选择 ----------

// fakeDetailSource 是 update.LineDetailSource 的测试实现。
type fakeDetailSource struct{ detail *update.LineDetail }

func (f *fakeDetailSource) LineDetails(context.Context, uuid.UUID) (*update.LineDetail, error) {
	return f.detail, nil
}

// TestElectronBlockmapNeverSelectedAsDiff blockmap 产物（kind=file，文件名 =
// 全量包 + ".blockmap"）在目录中存在时，diff 裁决必须仍走 full_package 且
// 响应不引用 blockmap（C17-4：不把 blockmap 当通用差量）。
func TestElectronBlockmapNeverSelectedAsDiff(t *testing.T) {
	src := newElectronCatalog(t, "demo", "1.0.0", 10)
	tgt := newElectronCatalog(t, "demo", "1.1.0", 11)
	cat := src
	cat.Versions = append(cat.Versions, tgt.Versions[0])
	cat.Versions[1].Version.PublishTime = &testPublishTime

	fullName := "demo-1.1.0-windows-x86_64.exe"
	blockmapName := fullName + ".blockmap"
	// blockmap 以 kind=file 身份落在 Line 明细（不上目录 FullPkgs）：
	// 若未来任何改动把 kind=file 映射进 Deltas/Patches，本断言即失败。
	details := &fakeDetailSource{detail: &update.LineDetail{
		Files: []update.FileArtifactInfo{{FileName: blockmapName, Size: 42, SHA256: "sha-bm"}},
	}}

	in := update.DiffInput{
		SourceVersion: "1.0.0", TargetVersion: "1.1.0",
		OS: "windows", Arch: "x86_64",
		// 最大差量意愿：显式能力 + 算法 + 纯净基线哈希，仍必须全量。
		Capabilities:       []string{"binary_delta"},
		AcceptedDeltaAlgos: []string{"bsdiff"},
		LocalSHA256:        "sha-" + "demo-1.0.0-windows-x86_64.exe",
	}
	res, err := update.Diff(context.Background(), cat, details, in)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if res.Body.DiffMode != update.DiffModeFullPackage {
		t.Fatalf("blockmap presence must not change diff mode, got %s", res.Body.DiffMode)
	}
	if !strings.HasSuffix(res.Body.PackageURL, "/sha-"+fullName) {
		t.Fatalf("package_url must stay the full package, got %q", res.Body.PackageURL)
	}
	body, err := json.Marshal(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "blockmap") {
		t.Fatalf("blockmap must not leak into diff response: %s", body)
	}
}
