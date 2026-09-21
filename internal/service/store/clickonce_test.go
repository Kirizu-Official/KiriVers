package store

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// ---------- ClickOnce .application 单元测试（§9 ClickOnce 行 / §9.2，C20-1..C20-6） ----------

// clickOnceAssembly 是 XML 反序列化验证根结构（验收项 1：必填 XML 元素完整）。
type clickOnceAssembly struct {
	XMLName          xml.Name          `xml:"urn:schemas-microsoft-com:asm.v1 assembly"`
	ManifestVersion  string            `xml:"manifestVersion,attr"`
	AssemblyIdentity clickOnceIdentity `xml:"assemblyIdentity"`
	Description      clickOnceDesc     `xml:"description"`
	Deployment       clickOnceDeploy   `xml:"deployment"`
	Dependency       clickOnceDep      `xml:"dependency"`
}

type clickOnceIdentity struct {
	Name                  string `xml:"name,attr"`
	Version               string `xml:"version,attr"`
	PublicKeyToken        string `xml:"publicKeyToken,attr"`
	ProcessorArchitecture string `xml:"processorArchitecture,attr"`
}

type clickOnceDesc struct {
	Publisher string `xml:"publisher,attr"`
	Product   string `xml:"product,attr"`
}

type clickOnceDeploy struct {
	Install            string                  `xml:"install,attr"`
	MapFileExtensions  string                  `xml:"mapFileExtensions,attr"`
	Subscription       clickOnceSubscription   `xml:"subscription"`
	DeploymentProvider clickOnceDeployProvider `xml:"deploymentProvider"`
}

type clickOnceSubscription struct {
	Update clickOnceUpdate `xml:"update"`
}

type clickOnceUpdate struct {
	BeforeApplicationStartup *struct{} `xml:"beforeApplicationStartup"`
}

type clickOnceDeployProvider struct {
	Codebase string `xml:"codebase,attr"`
}

type clickOnceDep struct {
	DependentAssembly clickOnceDepAssembly `xml:"dependentAssembly"`
}

type clickOnceDepAssembly struct {
	DependencyType   string            `xml:"dependencyType,attr"`
	Codebase         string            `xml:"codebase,attr"`
	Size             int64             `xml:"size,attr"`
	AssemblyIdentity clickOnceIdentity `xml:"assemblyIdentity"`
	Hash             clickOnceHash     `xml:"hash"`
}

type clickOnceHash struct {
	DigestMethod clickOnceDigestMethod `xml:"DigestMethod"`
	DigestValue  string                `xml:"DigestValue"`
}

type clickOnceDigestMethod struct {
	Algorithm string `xml:"Algorithm,attr"`
}

// parseClickOnceXML 解析并验证 ClickOnce XML 清单。
func parseClickOnceXML(t *testing.T, body []byte) *clickOnceAssembly {
	t.Helper()
	var doc clickOnceAssembly
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("xml unmarshal failed: %v\nbody:\n%s", err, body)
	}
	return &doc
}

// TestClickOnce_XMLFormatAndRequiredElements 验收项 1：必填 XML 元素完整、SHA-256 base64 正确、Content-Type 正确。
func TestClickOnce_XMLFormatAndRequiredElements(t *testing.T) {
	fileContent := []byte("clickonce-sample-package-payload")
	cat, storageData := newSquirrelMultiVerCatalog(t, "cdemo", "x86_64", []squirrelVer{
		{Semver: "1.2.3", VersionInt: 4, Rollout: 100, Ready: true, FileBytes: fileContent},
	})
	storage := newFakeStorage(storageData)

	adapter := NewClickOnceAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Listing: testListing("clickonce", map[string]string{
			"publisher": "Acme Corp",
			"product":   "Acme App",
		}),
		Channel: "stable",
		Path:    "MyApp.application",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if resp.ContentType != "application/x-ms-application; charset=utf-8" {
		t.Fatalf("content type = %q, want application/x-ms-application; charset=utf-8", resp.ContentType)
	}

	doc := parseClickOnceXML(t, resp.Body)

	// 1. 根属性
	if doc.ManifestVersion != "1.0" {
		t.Fatalf("manifestVersion = %q, want 1.0", doc.ManifestVersion)
	}

	// 2. assemblyIdentity
	if doc.AssemblyIdentity.Name != "MyApp.application" {
		t.Fatalf("identity name = %q, want MyApp.application", doc.AssemblyIdentity.Name)
	}
	// 4 段式版本：1.2.3.4
	if doc.AssemblyIdentity.Version != "1.2.3.4" {
		t.Fatalf("version = %q, want 1.2.3.4", doc.AssemblyIdentity.Version)
	}
	if doc.AssemblyIdentity.ProcessorArchitecture != "amd64" {
		t.Fatalf("processorArchitecture = %q, want amd64", doc.AssemblyIdentity.ProcessorArchitecture)
	}
	if doc.AssemblyIdentity.PublicKeyToken != "0000000000000000" {
		t.Fatalf("publicKeyToken = %q, want 0000000000000000", doc.AssemblyIdentity.PublicKeyToken)
	}

	// 3. description
	if doc.Description.Publisher != "Acme Corp" || doc.Description.Product != "Acme App" {
		t.Fatalf("description publisher=%q product=%q", doc.Description.Publisher, doc.Description.Product)
	}

	// 4. deployment & subscription
	if doc.Deployment.Install != "true" || doc.Deployment.MapFileExtensions != "true" {
		t.Fatalf("deployment install=%q mapFileExtensions=%q", doc.Deployment.Install, doc.Deployment.MapFileExtensions)
	}
	if doc.Deployment.Subscription.Update.BeforeApplicationStartup == nil {
		t.Fatal("subscription update beforeApplicationStartup missing")
	}

	// 5. dependency & dependentAssembly
	dep := doc.Dependency.DependentAssembly
	if dep.DependencyType != "install" {
		t.Fatalf("dep type = %q, want install", dep.DependencyType)
	}
	if dep.Size != int64(len(fileContent)) {
		t.Fatalf("dep size = %d, want %d", dep.Size, len(fileContent))
	}

	// 6. hash DigestValue (base64 of artifact sha256)
	artifact := cat.Versions[0].Lines[0].FullPkgs[0]
	rawSHA, _ := hex.DecodeString(artifact.SHA256)
	expectedB64 := base64.StdEncoding.EncodeToString(rawSHA)
	if dep.Hash.DigestValue != expectedB64 {
		t.Fatalf("digestValue = %q, want %q", dep.Hash.DigestValue, expectedB64)
	}
	if dep.Hash.DigestMethod.Algorithm != "http://www.w3.org/2000/09/xmldsig#sha256" {
		t.Fatalf("digest algorithm = %q", dep.Hash.DigestMethod.Algorithm)
	}
}

// TestClickOnce_VersionMapping 验证双号映射至 4 段式整数字符串规则（C20-2）。
func TestClickOnce_VersionMapping(t *testing.T) {
	ptrSemver := func(s string) *string { return &s }
	ptrInt := func(i int64) *int64 { return &i }

	tests := []struct {
		name       string
		semver     *string
		versionInt *int64
		want       string
	}{
		{
			name:       "semver + valid integer revision",
			semver:     ptrSemver("1.2.3"),
			versionInt: ptrInt(4),
			want:       "1.2.3.4",
		},
		{
			name:       "semver without integer revision",
			semver:     ptrSemver("2.0.1"),
			versionInt: nil,
			want:       "2.0.1.0",
		},
		{
			name:       "semver with integer exceeding 65535 falls back to 0",
			semver:     ptrSemver("3.1.0"),
			versionInt: ptrInt(100000),
			want:       "3.1.0.0",
		},
		{
			name:       "only integer revision maps to 1.0.0.x",
			semver:     nil,
			versionInt: ptrInt(42),
			want:       "1.0.0.42",
		},
		{
			name:       "only integer revision with modulo 65536",
			semver:     nil,
			versionInt: ptrInt(65536 + 7),
			want:       "1.0.0.7",
		},
		{
			name:       "neither present defaults to 1.0.0.0",
			semver:     nil,
			versionInt: nil,
			want:       "1.0.0.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := clickOnceVersion(tc.semver, tc.versionInt)
			if got != tc.want {
				t.Fatalf("clickOnceVersion(%v, %v) = %q, want %q", tc.semver, tc.versionInt, got, tc.want)
			}
		})
	}
}

// TestClickOnce_ArchMapping 验证架构映射至 ClickOnce processorArchitecture。
func TestClickOnce_ArchMapping(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"x86_64", "amd64"},
		{"amd64", "amd64"},
		{"x86", "x86"},
		{"i386", "x86"},
		{"386", "x86"},
		{"arm64", "arm64"},
		{"aarch64", "arm64"},
		{"unknown", "msil"},
	}

	for _, tc := range tests {
		got := clickOnceArch(tc.arch)
		if got != tc.want {
			t.Fatalf("clickOnceArch(%q) = %q, want %q", tc.arch, got, tc.want)
		}
	}
}

// TestClickOnce_GrayRolloutAndCritical 验证灰度与关键版本可见性（C20-3）。
func TestClickOnce_GrayRolloutAndCritical(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "cdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0")},
		{Semver: "1.1.0", Rollout: 50, IsCritical: false, Ready: true, FileBytes: []byte("v1.1.0")},
		{Semver: "1.2.0", Rollout: 20, IsCritical: true, Ready: true, FileBytes: []byte("v1.2.0")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewClickOnceAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "MyApp.application",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	doc := parseClickOnceXML(t, resp.Body)
	// 1.2.0 是关键版本，灰度 20% 仍匿名可见并成为最新目标；1.1.0 非关键 50% 不可见
	if doc.AssemblyIdentity.Version != "1.2.0.0" {
		t.Fatalf("version = %q, want 1.2.0.0", doc.AssemblyIdentity.Version)
	}
}

// TestClickOnce_DefaultHwVariantOnly 验证硬件变体隔离（C20-6：仅投影默认变体，MCU/非默认变体不冒充）。
func TestClickOnce_DefaultHwVariantOnly(t *testing.T) {
	hwNonDefault := "stm32"
	cat, storageData := newSquirrelMultiVerCatalog(t, "cdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0-win")},
		{Semver: "2.0.0", Rollout: 100, Ready: true, HwRev: &hwNonDefault, FileBytes: []byte("v2.0.0-mcu")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewClickOnceAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "cdemo.application",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	doc := parseClickOnceXML(t, resp.Body)
	// 2.0.0 仅有 stm32 硬件变体，不可见；投影视线为 1.0.0 默认变体
	if doc.AssemblyIdentity.Version != "1.0.0.0" {
		t.Fatalf("version = %q, want 1.0.0.0", doc.AssemblyIdentity.Version)
	}
}

// TestClickOnce_EmptyCatalogReturnsErrNoRelease 验证可见集为空时返回 ErrNoRelease（404）。
func TestClickOnce_EmptyCatalogReturnsErrNoRelease(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "cdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 50, IsCritical: false, Ready: true},
	})
	storage := newFakeStorage(storageData)

	adapter := NewClickOnceAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "app.application",
		Arch:    "x86_64",
	}

	_, err := adapter.Render(context.Background(), deps, req)
	if err != ErrNoRelease {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}

// TestClickOnce_UnknownPathReturnsErrUnknownPath 验证非 .application 路径返回 ErrUnknownPath。
func TestClickOnce_UnknownPathReturnsErrUnknownPath(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "cdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true},
	})
	storage := newFakeStorage(storageData)

	adapter := NewClickOnceAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}

	for _, badPath := range []string{"app.exe", "RELEASES", "latest.json", "manifest.xml"} {
		req := Request{
			Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
			Channel: "stable",
			Path:    badPath,
			Arch:    "x86_64",
		}
		_, err := adapter.Render(context.Background(), deps, req)
		if err == nil || !strings.Contains(err.Error(), ErrUnknownPath.Error()) {
			t.Fatalf("path %q err = %v, want ErrUnknownPath", badPath, err)
		}
	}
}

// TestClickOnce_UnknownChannelReturnsErrUnknownChannel 验证未知渠道返回 ErrUnknownChannel。
func TestClickOnce_UnknownChannelReturnsErrUnknownChannel(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "cdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true},
	})
	storage := newFakeStorage(storageData)

	adapter := NewClickOnceAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "nonexistent",
		Path:    "app.application",
		Arch:    "x86_64",
	}

	_, err := adapter.Render(context.Background(), deps, req)
	if err == nil || !strings.Contains(err.Error(), ErrUnknownChannel.Error()) {
		t.Fatalf("err = %v, want ErrUnknownChannel", err)
	}
}

// TestClickOnce_DefaultArchX86_64 验证省略 arch 时缺省采用 x86_64。
func TestClickOnce_DefaultArchX86_64(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "cdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewClickOnceAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "test.application",
		Arch:    "",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	doc := parseClickOnceXML(t, resp.Body)
	if doc.AssemblyIdentity.ProcessorArchitecture != "amd64" {
		t.Fatalf("processorArchitecture = %q, want amd64", doc.AssemblyIdentity.ProcessorArchitecture)
	}
}

// TestClickOnce_XMLEscaping 验证特殊字符安全转义。
func TestClickOnce_XMLEscaping(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "cdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewClickOnceAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Listing: testListing("clickonce", map[string]string{
			"publisher": "Foo & Bar <Special> \"Quotes\"",
			"product":   "Product 'Safe'",
		}),
		Channel: "stable",
		Path:    "test.application",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	doc := parseClickOnceXML(t, resp.Body)
	if doc.Description.Publisher != "Foo & Bar <Special> \"Quotes\"" {
		t.Fatalf("publisher unmarshaled = %q", doc.Description.Publisher)
	}
	if doc.Description.Product != "Product 'Safe'" {
		t.Fatalf("product unmarshaled = %q", doc.Description.Product)
	}
}

// TestClickOnce_EnabledFlag：HTTP 闸是 listing；Adapter.Enabled 仅 nil 为 false。
func TestClickOnce_EnabledFlag(t *testing.T) {
	adapter := NewClickOnceAdapter()
	if adapter.Enabled(nil) {
		t.Fatal("nil project must not be enabled")
	}
	if !adapter.Enabled(&model.Project{}) {
		t.Fatal("non-nil project must be enabled (listing is the HTTP gate)")
	}
}
