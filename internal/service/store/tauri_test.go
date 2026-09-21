package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

// ---------- Tauri updater 测试夹具（§9 Tauri 行，C18-1..C18-7） ----------

// tauriVersionSpec 是单个版本的构造描述：双号 + 灰度 + 强制 + 渠道。
type tauriVersionSpec struct {
	semver      string
	versionInt  int64
	rollout     int
	critical    bool
	channel     string
	changelogEn string
}

// tauriLineSpec 是单个 (os, arch) 切片的构造描述：状态与默认变体产物。
type tauriLineSpec struct {
	os, arch      string
	status        string
	fileNameStem  string // 文件名前缀 {stem}-{ver}-{os}-{arch}.dmg
	withHwVariant bool   // 追加一个非默认 hw 变体（revb）
}

// newTauriCatalog 构造多版本 / 多平台目录：stable 渠道、semver 引擎、
// 每版本在 specs 声明的全部平台上各有切片。默认变体（HwRev=nil）恒存在
// （status=ready 时），withHwVariant 追加非默认变体。
func newTauriCatalog(t *testing.T, slug string, versions []tauriVersionSpec, lines []tauriLineSpec) *update.Catalog {
	t.Helper()
	channels := []update.ChannelInfo{
		{Slug: "stable", StabilityRank: 30, Enabled: true},
		{Slug: "beta", StabilityRank: 20, Enabled: true},
	}
	osSet := make([]string, 0, len(lines))
	for _, l := range lines {
		dup := false
		for _, o := range osSet {
			if o == l.os {
				dup = true
			}
		}
		if !dup {
			osSet = append(osSet, l.os)
		}
	}
	states := make([]update.VersionState, 0, len(versions))
	for _, spec := range versions {
		v := model.Version{
			ID:          uuid.New(),
			ProjectID:   testProjectID,
			ChannelSlug: spec.channel,
			Status:      model.VersionStatusPublished,
			IsCritical:  spec.critical,
			PublishTime: &testPublishTime,
		}
		applyFeedGray(&v, spec.rollout, spec.critical)
		if spec.changelogEn != "" {
			v.Changelog = model.ChangelogMap{"en": {Title: "v" + spec.semver, Markdown: spec.changelogEn}}
		}
		if spec.semver != "" {
			v.VersionSemver = &spec.semver
			v.VersionSemverCanonical = &spec.semver
		}
		if spec.versionInt > 0 {
			v.VersionInteger = &spec.versionInt
		}
		ref := displayRef(spec.semver, spec.versionInt)
		lineStates := make([]update.LineState, 0, len(lines))
		for _, l := range lines {
			line := update.LineState{
				ID:       uuid.New(),
				OS:       l.os,
				Arch:     l.arch,
				Status:   l.status,
				FullPkgs: []update.ArtifactInfo{},
			}
			if l.status == model.VersionLineStatusReady {
				line.PacksReadyAt = packsReady()
			}
			if l.status == model.VersionLineStatusReady {
				fileName := l.fileNameStem + "-" + ref + "-" + l.os + "-" + l.arch + ".dmg"
				line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{
					FileName:    fileName,
					Size:        123456,
					SHA256:      "sha-" + fileName,
					ContentType: "application/x-apple-diskimage",
					StorageKey:  slug + "/" + "sha-" + fileName,
				})
				if l.withHwVariant {
					hw := "revb"
					hwName := l.fileNameStem + "-" + ref + "-" + l.os + "-" + l.arch + "-revb.dmg"
					line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{
						FileName:    hwName,
						Size:        999,
						SHA256:      "sha-" + hwName,
						ContentType: "application/x-apple-diskimage",
						StorageKey:  slug + "/" + "sha-" + hwName,
						HwRev:       &hw,
					})
				}
			}
			lineStates = append(lineStates, line)
		}
		states = append(states, update.VersionState{Version: v, Lines: lineStates})
	}
	return &update.Catalog{
		Project:   newTestProject(slug),
		Channels:  channels,
		Matrix:    &update.MatrixInfo{OS: osSet[0], Arch: lines[0].arch, PackageType: model.PackageTypeSingleFile},
		EnabledOS: osSet,
		Versions:  states,
	}
}

// renderTauriDynamic 装配默认依赖并渲染动态端点，返回响应（调用方自断言）。
func renderTauriDynamic(t *testing.T, cat *update.Catalog, target, arch, current string, mutate func(*Deps, *Request)) (*Response, error) {
	t.Helper()
	adapter := NewTauriAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    target + "/" + arch + "/" + current,
	}
	if mutate != nil {
		mutate(deps, &req)
	}
	return adapter.Render(context.Background(), deps, req)
}

// renderTauriStatic 装配默认依赖并渲染静态 latest.json。
func renderTauriStatic(t *testing.T, cat *update.Catalog, os, arch string, mutate func(*Deps, *Request)) (*Response, error) {
	t.Helper()
	adapter := NewTauriAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    tauriStaticPath,
		OS:      os,
		Arch:    arch,
	}
	if mutate != nil {
		mutate(deps, &req)
	}
	return adapter.Render(context.Background(), deps, req)
}

// parseTauriUpdateDoc 解析动态端点单平台对象并断言官方字段集齐全
// （验收项 1：version/pub_date/url 必有；signature 视签名配置）。
func parseTauriUpdateDoc(t *testing.T, body []byte) *tauriUpdateDoc {
	t.Helper()
	var doc tauriUpdateDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal tauri update doc: %v\n%s", err, body)
	}
	if doc.Version == "" || doc.URL == "" {
		t.Fatalf("incomplete tauri update doc: %+v\n%s", doc, body)
	}
	return &doc
}

// assertMinisignContainer 断言 Tauri 签名是可解析的 minisign 容器
// （C18-4 / 验收项 3）：untrusted comment 行 + base64 行；base64 解码后
// 恰为 74 字节、前缀 "Ed"、key_id 8 字节全 0、尾部 64 字节 Ed25519 签名。
// 返回裸 64 字节签名（供调用方验证）。
func assertMinisignContainer(t *testing.T, sig string) []byte {
	t.Helper()
	lines := strings.Split(sig, "\n")
	if len(lines) != 3 || lines[2] != "" {
		t.Fatalf("minisign container must be comment line + base64 line + trailing newline, got %q", sig)
	}
	if lines[0] != tauriMinisignComment {
		t.Fatalf("untrusted comment = %q, want %q", lines[0], tauriMinisignComment)
	}
	block, err := base64.StdEncoding.DecodeString(lines[1])
	if err != nil {
		t.Fatalf("minisign payload not base64: %v", err)
	}
	if len(block) != 2+8+64 {
		t.Fatalf("minisign block = %d bytes, want 74", len(block))
	}
	if string(block[:2]) != "Ed" {
		t.Fatalf("minisign block prefix = %q, want \"Ed\"", block[:2])
	}
	for _, b := range block[2:10] {
		if b != 0 {
			t.Fatal("minisign key_id bytes must be zero")
		}
	}
	return block[10:]
}

// twoTauriVersions 构造 1.0.0（当前）+ 1.1.0（目标）双版本目录。
func twoTauriVersions(t *testing.T, slug string, lines ...tauriLineSpec) *update.Catalog {
	return newTauriCatalog(t, slug,
		[]tauriVersionSpec{
			{semver: "1.0.0", versionInt: 10, rollout: 100, channel: "stable", changelogEn: "changes for 1.0.0"},
			{semver: "1.1.0", versionInt: 11, rollout: 100, channel: "stable", changelogEn: "changes for 1.1.0"},
		},
		lines)
}

// macosReadyLine 是默认 macOS 切片描述。
func macosReadyLine() tauriLineSpec {
	return tauriLineSpec{os: "macos", arch: "x86_64", status: model.VersionLineStatusReady,
		fileNameStem: "demo"}
}

// ---------- 动态端点：官方字段集 / 204 / 映射 / 当前版本语义 ----------

// TestTauriDynamicUpdateFixture 有更新 → 200 单平台对象，官方字段齐全，
// darwin→macos / x86_64 映射生效（C18-1 / C18-2，验收项 1）。
func TestTauriDynamicUpdateFixture(t *testing.T) {
	cat := twoTauriVersions(t, "demo", macosReadyLine())
	resp, err := renderTauriDynamic(t, cat, "darwin", "x86_64", "1.0.0", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}
	if resp.ContentType != tauriContentType {
		t.Fatalf("content type = %q", resp.ContentType)
	}
	doc := parseTauriUpdateDoc(t, resp.Body)
	// C18-2：version = version_semver。
	if doc.Version != "1.1.0" {
		t.Fatalf("version = %q", doc.Version)
	}
	// notes = 目标版本 changelog 聚合（locale 回退链同 check）。
	if !strings.Contains(doc.Notes, "changes for 1.1.0") {
		t.Fatalf("notes = %q", doc.Notes)
	}
	// pub_date = RFC 3339（UTC）。
	if want := testPublishTime.UTC().Format("2006-01-02T15:04:05Z07:00"); doc.PubDate != want {
		t.Fatalf("pub_date = %q, want %q", doc.PubDate, want)
	}
	// url = 默认变体 kind=full 稳定文件名 URL。
	if want := "/api/v1/projects/demo/packages/sha-demo-1.1.0-macos-x86_64.dmg"; doc.URL != want {
		t.Fatalf("url = %q, want %q", doc.URL, want)
	}
}

// TestTauriDynamicNoUpdate204 已是最新 / 未知当前版本 → 204 无 body（C18-3，
// 验收项 2）。
func TestTauriDynamicNoUpdate204(t *testing.T) {
	cat := twoTauriVersions(t, "demo", macosReadyLine())

	for _, current := range []string{"1.1.0", "9.9.9-not-in-catalog", "not-a-version"} {
		resp, err := renderTauriDynamic(t, cat, "darwin", "x86_64", current, nil)
		if err != nil {
			t.Fatalf("current %q render: %v", current, err)
		}
		if resp.Status != http.StatusNoContent {
			t.Fatalf("current %q status = %d, want 204", current, resp.Status)
		}
		if len(resp.Body) != 0 {
			t.Fatalf("current %q must have no body, got %q", current, resp.Body)
		}
		if resp.ContentType != "" {
			t.Fatalf("current %q must not carry content type", current)
		}
	}

	// 当前比目标新（不存在更高可见版本）→ 204。
	cat2 := twoTauriVersions(t, "demo", macosReadyLine())
	resp, err := renderTauriDynamic(t, cat2, "windows", "x86_64", "1.1.0", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusNoContent {
		t.Fatalf("no-update status = %d, want 204", resp.Status)
	}
}

// TestTauriUnknownTargetArch404 未知 target / arch / 路径形态 →
// ErrUnknownPath（HTTP 404，C18-1）。
func TestTauriUnknownTargetArch404(t *testing.T) {
	cat := twoTauriVersions(t, "demo", macosReadyLine())
	adapter := NewTauriAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}
	for _, path := range []string{
		"freebsd/x86_64/1.0.0",      // 未知 target
		"darwin/i686/1.0.0",         // 未知 arch（Tauri 目标集外）
		"darwin/x86_64",             // 缺 current_version 且无 query
		"",                          // 空路径
		"latest.xml",                // 未知文档
		"darwin/x86_64/1.0.0/extra", // 段数过多
	} {
		_, err := adapter.Render(context.Background(), deps,
			Request{Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
				Channel: "stable", Path: path})
		if err == nil {
			t.Fatalf("path %q must fail", path)
		}
	}

	// 两段 + current_version query 兼容形态可用。
	resp, err := adapter.Render(context.Background(), deps,
		Request{Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
			Channel: "stable", Path: "darwin/x86_64", CurrentVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("two-segment form: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("two-segment form status = %d", resp.Status)
	}
}

// TestTauriIntegerVersion 无 SemVer 时 version 为整数十进制字符串（C18-2，
// 验收项 1）。整数比较键目录：compare_engine = integer。
func TestTauriIntegerVersion(t *testing.T) {
	cat := newTauriCatalog(t, "demo",
		[]tauriVersionSpec{
			{versionInt: 10, rollout: 100, channel: "stable"},
			{versionInt: 11, rollout: 100, channel: "stable"},
		},
		[]tauriLineSpec{macosReadyLine()})
	cat.Project.CompareEngine = model.CompareEngineInteger
	resp, err := renderTauriDynamic(t, cat, "darwin", "x86_64", "10", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	doc := parseTauriUpdateDoc(t, resp.Body)
	if doc.Version != "11" {
		t.Fatalf("version = %q, want \"11\"", doc.Version)
	}
	// JSON 字符串语义：整数构建号不得输出为 JSON 数字。
	if strings.Contains(string(resp.Body), `"version":11`) {
		t.Fatalf("integer version must stay a JSON string:\n%s", resp.Body)
	}
}

// TestTauriUnknownChannel400 未知渠道 → ErrUnknownChannel（400）。
func TestTauriUnknownChannel400(t *testing.T) {
	cat := twoTauriVersions(t, "demo", macosReadyLine())
	adapter := NewTauriAdapter()
	_, err := adapter.Render(context.Background(),
		&Deps{Updates: update.NewService(&fakeLoader{cat: cat})},
		Request{Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
			Channel: "nightly", Path: "darwin/x86_64/1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "unknown channel") {
		t.Fatalf("unknown channel must fail with ErrUnknownChannel, got %v", err)
	}
}

// ---------- 签名（C18-4，验收项 3） ----------

// TestTauriSignatureMinisignContainerOverFileBytes minisign 容器语义：
// 前缀 "Ed" + 8 字节全 0 key_id + 64 字节签名，整体对安装器文件字节可验；
// 且绝不复用原生 check signature 字节（容器可解析 + 裸签名不可当原生签名
// 用）。LRU 缓存：第二次渲染不再读文件。
func TestTauriSignatureMinisignContainerOverFileBytes(t *testing.T) {
	privPEM, pubPEM := genEd25519PEM(t)
	fileBytes := []byte("dmg-bytes-for-tauri-minisign-signature-test")
	cat := twoTauriVersions(t, "demo", macosReadyLine())
	cat.Project.SigningPrivateKey = privPEM
	key := cat.Versions[1].Lines[0].FullPkgs[0].StorageKey
	storage := newFakeStorage(map[string][]byte{key: fileBytes})

	adapter := NewTauriAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable", Path: "darwin/x86_64/1.0.0",
	}

	render := func() *tauriUpdateDoc {
		resp, err := adapter.Render(context.Background(), deps, req)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		return parseTauriUpdateDoc(t, resp.Body)
	}

	doc := render()
	if doc.Signature == "" {
		t.Fatal("ed25519 project must emit signature")
	}
	raw := assertMinisignContainer(t, doc.Signature)

	// 签名本体对安装器文件字节可验（minisign 内嵌裸签名 = base64(64B)）。
	if err := signature.VerifyPayload(signature.AlgoEd25519, pubPEM, string(fileBytes), base64.StdEncoding.EncodeToString(raw)); err != nil {
		t.Fatalf("minisign signature must verify over file bytes: %v", err)
	}

	// 验收项 3：原生 check signature 字符串（元数据载荷签名）不得充当
	// Tauri signature——两字符串不同，且裸文件签名对元数据载荷不可验。
	nativePayload := signature.BuildCheckPayload("11", "1.1.0", "", doc.URL, "123456", "sha-demo-1.1.0-macos-x86_64.dmg")
	nativeSig, err := signature.SignPayload(signature.AlgoEd25519, privPEM, nativePayload)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Signature == nativeSig {
		t.Fatal("tauri signature must never reuse the native check signature bytes")
	}
	if err := signature.VerifyPayload(signature.AlgoEd25519, pubPEM, nativePayload, base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("tauri signature must NOT verify over the native check payload (signs file bytes only)")
	}

	// 第二次渲染：LRU 命中，文件只读一次，签名稳定。
	if got := render().Signature; got != doc.Signature {
		t.Fatal("signature must be stable across renders")
	}
	if n := storage.readCount(key); n != 1 {
		t.Fatalf("artifact must be read once (LRU), got %d", n)
	}
}

// TestTauriRSAProjectNoSignature RSA 项目省略 signature 字段（不冒充）。
func TestTauriRSAProjectNoSignature(t *testing.T) {
	cat := twoTauriVersions(t, "demo", macosReadyLine())
	cat.Project.SigningAlgo = model.SigningAlgoRSASHA256
	resp, err := renderTauriDynamic(t, cat, "darwin", "x86_64", "1.0.0",
		func(deps *Deps, _ *Request) { deps.Storage = newFakeStorage(nil) })
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(string(resp.Body), "signature") {
		t.Fatalf("RSA project must omit signature field:\n%s", resp.Body)
	}
}

// ---------- 静态 latest.json（C18-1）与静态/动态一致性 ----------

// TestTauriStaticLatestJSONConsistentWithDynamic 同一 os/arch 下静态
// platforms 条目的 version/url 与动态端点一致（验收项 1 + 一致性标准）。
func TestTauriStaticLatestJSONConsistentWithDynamic(t *testing.T) {
	cat := twoTauriVersions(t, "demo", macosReadyLine())

	stat, err := renderTauriStatic(t, cat, "macos", "x86_64", nil)
	if err != nil {
		t.Fatalf("static render: %v", err)
	}
	if stat.Status != http.StatusOK || stat.ContentType != tauriContentType {
		t.Fatalf("static status/content = %d/%q", stat.Status, stat.ContentType)
	}
	var doc tauriStaticDoc
	if err := json.Unmarshal(stat.Body, &doc); err != nil {
		t.Fatalf("unmarshal static doc: %v\n%s", err, stat.Body)
	}
	if doc.Version != "1.1.0" {
		t.Fatalf("static version = %q", doc.Version)
	}
	if !strings.Contains(doc.Notes, "changes for 1.1.0") {
		t.Fatalf("static notes = %q", doc.Notes)
	}
	entry, ok := doc.Platforms["darwin-x86_64"]
	if !ok {
		t.Fatalf("platforms must contain darwin-x86_64: %v", doc.Platforms)
	}

	dyn, err := renderTauriDynamic(t, cat, "darwin", "x86_64", "1.0.0", nil)
	if err != nil {
		t.Fatalf("dynamic render: %v", err)
	}
	dynDoc := parseTauriUpdateDoc(t, dyn.Body)
	if dynDoc.URL != entry.URL {
		t.Fatalf("static/dynamic url mismatch: %q vs %q", entry.URL, dynDoc.URL)
	}
	if dynDoc.Version != doc.Version {
		t.Fatalf("static/dynamic version mismatch: %q vs %q", doc.Version, dynDoc.Version)
	}
}

// TestTauriStaticMultiPlatformEnumerates 无 os/arch query：platforms 覆盖
// 矩阵中全部 Tauri 可用 os（macos→darwin、windows→windows），顶层版本为
// 全局最新（C18-1，§9 os/arch 映射）。
func TestTauriStaticMultiPlatformEnumerates(t *testing.T) {
	cat := twoTauriVersions(t, "demo",
		macosReadyLine(),
		tauriLineSpec{os: "windows", arch: "x86_64", status: model.VersionLineStatusReady, fileNameStem: "demo"})

	stat, err := renderTauriStatic(t, cat, "", "", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc tauriStaticDoc
	if err := json.Unmarshal(stat.Body, &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stat.Body)
	}
	if doc.Version != "1.1.0" {
		t.Fatalf("top-level version = %q", doc.Version)
	}
	for _, key := range []string{"darwin-x86_64", "windows-x86_64"} {
		e, ok := doc.Platforms[key]
		if !ok || e.URL == "" {
			t.Fatalf("platforms[%q] missing or empty: %v", key, doc.Platforms)
		}
	}
	if want := "/api/v1/projects/demo/packages/sha-demo-1.1.0-windows-x86_64.dmg"; doc.Platforms["windows-x86_64"].URL != want {
		t.Fatalf("windows url = %q, want %q", doc.Platforms["windows-x86_64"].URL, want)
	}
	if _, ok := doc.Platforms["darwin-aarch64"]; ok {
		t.Fatal("arm64 must not appear when the project has no arm64 line")
	}
}

// TestTauriStaticAllGray404 可见集全空（唯一版本灰度 50% 非强制）→
// ErrNoRelease（404，不发空文档）。
func TestTauriStaticAllGray404(t *testing.T) {
	cat := newTauriCatalog(t, "demo",
		[]tauriVersionSpec{{semver: "1.1.0", versionInt: 11, rollout: 50, channel: "stable"}},
		[]tauriLineSpec{macosReadyLine()})
	_, err := renderTauriStatic(t, cat, "macos", "x86_64", nil)
	if err == nil {
		t.Fatal("empty visible set must fail")
	}
}

// TestTauriStaticArchAlias arch query 别名经 HTTP 层规范化（aarch64→
// arm64，Adapter 收到规范值）后映射平台键为官方 darwin-aarch64。
func TestTauriStaticArchAlias(t *testing.T) {
	cat := twoTauriVersions(t, "demo",
		tauriLineSpec{os: "macos", arch: "arm64", status: model.VersionLineStatusReady, fileNameStem: "demo"})
	stat, err := renderTauriStatic(t, cat, "macos", "arm64", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc tauriStaticDoc
	if err := json.Unmarshal(stat.Body, &doc); err != nil {
		t.Fatal(err)
	}
	e, ok := doc.Platforms["darwin-aarch64"]
	if !ok {
		t.Fatalf("platforms must contain darwin-aarch64: %v", doc.Platforms)
	}
	if want := "/api/v1/projects/demo/packages/sha-demo-1.1.0-macos-arm64.dmg"; e.URL != want {
		t.Fatalf("url = %q, want %q", e.URL, want)
	}
}

// ---------- 灰度 / 默认 hw 变体 / Token 前置语义（C18-5 / C18-7） ----------

// TestTauriGrayProjection 灰度 <100% 的非强制版本对匿名不可见：动态
// 无更新（204），静态回退到下一个可见版本（C18-5，§4.4 匿名口径）。
func TestTauriGrayProjection(t *testing.T) {
	cat := newTauriCatalog(t, "demo",
		[]tauriVersionSpec{
			{semver: "1.0.0", versionInt: 10, rollout: 100, channel: "stable", changelogEn: "g 1.0.0"},
			{semver: "1.1.0", versionInt: 11, rollout: 50, channel: "stable", changelogEn: "g 1.1.0"},
		},
		[]tauriLineSpec{macosReadyLine()})

	// 动态：1.0.0 客户端看不到灰度中的 1.1.0 → 204。
	resp, err := renderTauriDynamic(t, cat, "darwin", "x86_64", "1.0.0", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusNoContent {
		t.Fatalf("gray target must be invisible, status = %d", resp.Status)
	}

	// 静态：顶层回退 1.0.0。
	stat, err := renderTauriStatic(t, cat, "macos", "x86_64", nil)
	if err != nil {
		t.Fatalf("static render: %v", err)
	}
	var doc tauriStaticDoc
	if err := json.Unmarshal(stat.Body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != "1.0.0" {
		t.Fatalf("static must fall through to 1.0.0, got %q", doc.Version)
	}
}

// TestTauriCriticalVersionVisibleInGray 灰度 <100% 的关键版本对匿名强制可见
// （§5.4 / §4.4：IsCritical 忽略灰度）：动态端点经由 SelectTarget 的
// mandatory 路径返回更新；静态 latest.json 顶层与 platforms 均投影该版本
// （C18-5 灰度语义的另一半）。
func TestTauriCriticalVersionVisibleInGray(t *testing.T) {
	cat := newTauriCatalog(t, "demo",
		[]tauriVersionSpec{
			{semver: "1.0.0", versionInt: 10, rollout: 100, channel: "stable", changelogEn: "g 1.0.0"},
			{semver: "1.1.0", versionInt: 11, rollout: 50, critical: true, channel: "stable", changelogEn: "g 1.1.0"},
		},
		[]tauriLineSpec{macosReadyLine()})

	// 动态：关键版本对匿名客户端强制可见 → 200。
	resp, err := renderTauriDynamic(t, cat, "darwin", "x86_64", "1.0.0", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("critical version must be visible, status = %d", resp.Status)
	}
	doc := parseTauriUpdateDoc(t, resp.Body)
	if doc.Version != "1.1.0" {
		t.Fatalf("dynamic version = %q, want 1.1.0", doc.Version)
	}

	// 静态：顶层与 platforms 投影同一关键版本。
	stat, err := renderTauriStatic(t, cat, "macos", "x86_64", nil)
	if err != nil {
		t.Fatalf("static render: %v", err)
	}
	var sdoc tauriStaticDoc
	if err := json.Unmarshal(stat.Body, &sdoc); err != nil {
		t.Fatal(err)
	}
	if sdoc.Version != "1.1.0" {
		t.Fatalf("static version = %q, want 1.1.0", sdoc.Version)
	}
	if _, ok := sdoc.Platforms["darwin-x86_64"]; !ok {
		t.Fatalf("platforms must contain darwin-x86_64: %v", sdoc.Platforms)
	}
}

// TestTauriDefaultHWVariantOnly 非 hw 版本只能拿到默认变体（C18-7）：存在
// 非默认 hw 变体时动态仍指向默认变体；目标线只有非默认变体 → 无候选 →
// 204（绝不把非默认 hw 包当成该平台的包）。
func TestTauriDefaultHWVariantOnly(t *testing.T) {
	// 目标线含默认 + revb 变体：动态指向默认变体。
	cat := twoTauriVersions(t, "demo", tauriLineSpec{
		os: "macos", arch: "x86_64", status: model.VersionLineStatusReady,
		fileNameStem: "demo", withHwVariant: true,
	})
	resp, err := renderTauriDynamic(t, cat, "darwin", "x86_64", "1.0.0", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	doc := parseTauriUpdateDoc(t, resp.Body)
	if want := "/api/v1/projects/demo/packages/sha-demo-1.1.0-macos-x86_64.dmg"; doc.URL != want {
		t.Fatalf("url must be the default variant, got %q", doc.URL)
	}
	if strings.Contains(string(resp.Body), "revb") {
		t.Fatalf("non-default hw variant must not appear:\n%s", resp.Body)
	}

	// 目标线只有 revb 变体（无默认变体）→ 视为该平台无包 → 204。
	catNoDefault := twoTauriVersions(t, "demo", macosReadyLine())
	target := &catNoDefault.Versions[1].Lines[0]
	target.FullPkgs = target.FullPkgs[:0]
	hw := "revb"
	target.FullPkgs = append(target.FullPkgs, update.ArtifactInfo{
		FileName:    "demo-1.1.0-macos-x86_64-revb.dmg",
		Size:        999,
		SHA256:      "sha-revb",
		ContentType: "application/x-apple-diskimage",
		StorageKey:  "demo/sha-revb",
		HwRev:       &hw,
	})
	resp2, err := renderTauriDynamic(t, catNoDefault, "darwin", "x86_64", "1.0.0", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp2.Status != http.StatusNoContent {
		t.Fatalf("non-default-only target line must be 204, got %d", resp2.Status)
	}
}

// TestTauriPrivateSignedURL 私有项目：动态 url 带短时签名 query（§13.7）。
func TestTauriPrivateSignedURL(t *testing.T) {
	cat := twoTauriVersions(t, "demo", macosReadyLine())
	cat.Project.StorageVisibility = model.StorageVisibilityPrivate
	resp, err := renderTauriDynamic(t, cat, "darwin", "x86_64", "1.0.0",
		func(deps *Deps, _ *Request) { deps.Signer = stubSigner{} })
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	doc := parseTauriUpdateDoc(t, resp.Body)
	if want := "/api/v1/projects/demo/packages/sha-demo-1.1.0-macos-x86_64.dmg?exp=123&sig=abc"; doc.URL != want {
		t.Fatalf("private url = %q, want signed %q", doc.URL, want)
	}
}

// TestTauriNoSemverStaticVersion 静态文档 version 亦为整数字符串（C18-2）。
func TestTauriNoSemverStaticVersion(t *testing.T) {
	cat := newTauriCatalog(t, "demo",
		[]tauriVersionSpec{{versionInt: 11, rollout: 100, channel: "stable"}},
		[]tauriLineSpec{macosReadyLine()})
	stat, err := renderTauriStatic(t, cat, "macos", "x86_64", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc tauriStaticDoc
	if err := json.Unmarshal(stat.Body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != "11" {
		t.Fatalf("version = %q, want \"11\"", doc.Version)
	}
}
