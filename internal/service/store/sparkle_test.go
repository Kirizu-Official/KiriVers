package store

import (
	"context"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

// ---------- appcast.xml 解析夹具（对照官方 Sparkle appcast 必填元素） ----------

const sparkleNS = "http://www.andymatuschak.org/xml-namespaces/sparkle"

// appcastDoc 是 Sparkle appcast 的解析目标；字段名对应官方 schema
// （https://sparkle-project.org/documentation/publishing/）。
type appcastDoc struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel struct {
		Title       string        `xml:"title"`
		Link        string        `xml:"link"`
		Description string        `xml:"description"`
		Items       []appcastItem `xml:"item"`
	} `xml:"channel"`
}

// appcastEnclosure 是 enclosure 元素的属性组（encoding/xml 不支持
// 「子元素属性」点路径，必须嵌套结构）。
type appcastEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type appcastItem struct {
	Title          string           `xml:"title"`
	PubDate        string           `xml:"pubDate"`
	SparkleVersion string           `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle version"`
	SparkleShort   string           `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
	SparkleOS      string           `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle os"`
	SparkleChannel string           `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle channel"`
	SparkleMinSys  string           `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle minimumSystemVersion"`
	Description    string           `xml:"description"`
	Enclosure      appcastEnclosure `xml:"enclosure"`
	EdSignature    string           `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle edSignature"`
}

// renderAppcast 装配默认依赖并渲染 appcast，返回解析后的文档与原始 body。
func renderAppcast(t *testing.T, cat *update.Catalog, mutate func(*Deps, *Request)) (*appcastDoc, []byte) {
	t.Helper()
	adapter := NewSparkleAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		OS:      "macos",
		Arch:    "x86_64",
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
	if resp.ContentType != "application/xml; charset=utf-8" {
		t.Fatalf("content type = %q", resp.ContentType)
	}
	var doc appcastDoc
	if err := xml.Unmarshal(resp.Body, &doc); err != nil {
		t.Fatalf("unmarshal: %v\nbody:\n%s", err, resp.Body)
	}
	return &doc, resp.Body
}

// assertRequiredElements 断言官方/常用 Sparkle appcast 必填元素齐全
// （验收项 1：rss@version、channel title/link/description、item
// title/pubDate/description/enclosure url+length+type）。
func assertRequiredElements(t *testing.T, doc *appcastDoc) {
	t.Helper()
	if doc.Version != "2.0" {
		t.Fatalf("rss version = %q", doc.Version)
	}
	if doc.Channel.Title == "" || doc.Channel.Link == "" || doc.Channel.Description == "" {
		t.Fatalf("channel required elements missing: %+v", doc.Channel)
	}
	if len(doc.Channel.Items) == 0 {
		t.Fatal("channel must contain at least one item")
	}
	for i, item := range doc.Channel.Items {
		if item.Title == "" {
			t.Fatalf("item %d missing title", i)
		}
		if item.PubDate == "" {
			t.Fatalf("item %d missing pubDate", i)
		}
		if _, err := time.Parse(time.RFC1123Z, item.PubDate); err != nil {
			t.Fatalf("item %d pubDate %q not RFC 822/1123Z: %v", i, item.PubDate, err)
		}
		if item.Description == "" {
			t.Fatalf("item %d missing description", i)
		}
		if item.Enclosure.URL == "" || item.Enclosure.Length <= 0 || item.Enclosure.Type == "" {
			t.Fatalf("item %d enclosure incomplete: %+v", i, item)
		}
	}
}

// TestSparkleAppcastFixtureRequiredElements 官方 appcast 必填元素 + 双号映射
// （验收项 1、2：整数进 sparkle:version、SemVer 进 sparkle:shortVersionString）。
func TestSparkleAppcastFixtureRequiredElements(t *testing.T) {
	// 目录含两个版本（1.1.0 / 1.0.0），1.1.0 附加非默认 hw 变体。
	cat := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, map[string]string{"revA": ""}, "10.13")
	v0 := newTestCatalog(t, "demo", "stable", "1.0.0", 10, 100, false, model.VersionLineStatusReady, nil, "")
	cat.Versions = append(cat.Versions, v0.Versions[0])
	cat.Versions[1].Version.PublishTime = &testPublishTime
	cat.Versions[0].Version.PublishTime = &testPublishTime

	doc, body := renderAppcast(t, cat, nil)

	assertRequiredElements(t, doc)
	if !strings.Contains(string(body), `xmlns:sparkle="`+sparkleNS+`"`) {
		t.Fatalf("missing sparkle namespace declaration:\n%s", body)
	}

	// 双号映射（C16-2）：sparkle:version = version_integer，
	// sparkle:shortVersionString = version_semver。
	if len(doc.Channel.Items) != 2 {
		t.Fatalf("items = %d, want 2\n%s", len(doc.Channel.Items), body)
	}
	newest, oldest := doc.Channel.Items[0], doc.Channel.Items[1]
	if newest.SparkleVersion != "11" || newest.SparkleShort != "1.1.0" {
		t.Fatalf("dual numbers wrong on newest: %q / %q", newest.SparkleVersion, newest.SparkleShort)
	}
	if oldest.SparkleVersion != "10" || oldest.SparkleShort != "1.0.0" {
		t.Fatalf("dual numbers wrong on oldest: %q / %q", oldest.SparkleVersion, oldest.SparkleShort)
	}
	// 排序：比较键新→旧。
	if newest.Title != "1.1.0" || oldest.Title != "1.0.0" {
		t.Fatalf("item order wrong: %q, %q", newest.Title, oldest.Title)
	}
	// sparkle:os 与 min_os。
	if newest.SparkleOS != "macos" {
		t.Fatalf("sparkle:os = %q", newest.SparkleOS)
	}
	if newest.SparkleMinSys != "10.13" {
		t.Fatalf("sparkle:minimumSystemVersion = %q", newest.SparkleMinSys)
	}
	// stable 渠道不标注 sparkle:channel。
	if newest.SparkleChannel != "" {
		t.Fatalf("stable channel must not emit sparkle:channel, got %q", newest.SparkleChannel)
	}
	// description 聚合该版本 changelog（target_only，C16-6）。
	if !strings.Contains(newest.Description, "changes for 1.1.0") {
		t.Fatalf("description missing changelog text: %q", newest.Description)
	}
	if strings.Contains(newest.Description, "changes for 1.0.0") {
		t.Fatalf("description must be target-only: %q", newest.Description)
	}
	// enclosure：默认变体稳定文件名 URL + 产物 Content-Type（C16-3），
	// 非默认变体不出现（C16-8）。
	if newest.Enclosure.URL != "/api/v1/projects/demo/packages/aa" {
		t.Fatalf("enclosure url = %q", newest.Enclosure.URL)
	}
	if newest.Enclosure.Type != "application/x-apple-diskimage" {
		t.Fatalf("enclosure type = %q", newest.Enclosure.Type)
	}
	if strings.Contains(string(body), "revA") {
		t.Fatalf("non-default hw variant must not appear in appcast:\n%s", body)
	}
}

// TestSparkleXMLEscaping changelog 含 XML 特殊字符（& < > 引号、注入片段）
// 时必须全部转义：body 中不得出现未转义的可执行片段，解析后 description
// 恢复原文（XML 注入不可能，§9.2）。
func TestSparkleXMLEscaping(t *testing.T) {
	raw := `a & b < c > d " e ' f </description><script>alert(1)</script>`
	cat := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, nil, "",
		func(v *model.Version, _ *update.LineState) {
			v.Changelog = model.ChangelogMap{"en": {Title: "v1.1.0", Markdown: raw}}
		})

	doc, body := renderAppcast(t, cat, nil)
	assertRequiredElements(t, doc)

	if strings.Contains(string(body), "<script>alert(1)</script>") {
		t.Fatalf("unescaped changelog content leaked into XML:\n%s", body)
	}
	if !strings.Contains(string(body), "&amp;") || !strings.Contains(string(body), "&lt;") {
		t.Fatalf("special characters must be escaped:\n%s", body)
	}
	// 聚合 layout 以标题开头（## v1.1.0），后接原文；全文逐字节还原。
	if got := doc.Channel.Items[0].Description; got != "## v1.1.0\n\n"+raw {
		t.Fatalf("description roundtrip mismatch:\n got %q\nwant %q", got, "## v1.1.0\n\n"+raw)
	}
}

// TestSparkleAppcastBetaChannel 非默认渠道输出 sparkle:channel。
func TestSparkleAppcastBetaChannel(t *testing.T) {
	cat := newTestCatalog(t, "demo", "beta", "1.1.0-beta.1", 11, 100, false, model.VersionLineStatusReady, nil, "")
	doc, _ := renderAppcast(t, cat, func(_ *Deps, req *Request) { req.Channel = "beta" })
	assertRequiredElements(t, doc)
	if doc.Channel.Items[0].SparkleChannel != "beta" {
		t.Fatalf("sparkle:channel = %q", doc.Channel.Items[0].SparkleChannel)
	}
}

// TestSparkleAppcastDefaultVariantWithoutHw 不传 hw 时（Request.HWRev 恒空）
// enclosure 指向默认变体全量包（验收项 5，C16-8）。
func TestSparkleAppcastDefaultVariantWithoutHw(t *testing.T) {
	cat := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, map[string]string{"revB": ""}, "")
	doc, _ := renderAppcast(t, cat, nil)
	assertRequiredElements(t, doc)
	item := doc.Channel.Items[0]
	if item.Enclosure.URL != "/api/v1/projects/demo/packages/aa" {
		t.Fatalf("enclosure must point to default variant, got %q", item.Enclosure.URL)
	}
	if strings.Contains(item.Enclosure.URL, "revB") {
		t.Fatalf("enclosure must not point at hw variant: %q", item.Enclosure.URL)
	}
}

// TestSparkleAppcastParameterErrors 缺 os/arch 与未知渠道。
func TestSparkleAppcastParameterErrors(t *testing.T) {
	cat := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, nil, "")
	adapter := NewSparkleAdapter()

	_, err := adapter.Render(context.Background(), &Deps{Updates: update.NewService(&fakeLoader{cat: cat})},
		Request{Project: &model.Project{ID: cat.Project.ID}, Channel: "stable"})
	if err == nil {
		t.Fatal("missing os/arch must fail")
	}

	_, err = adapter.Render(context.Background(), &Deps{Updates: update.NewService(&fakeLoader{cat: cat})},
		Request{Project: &model.Project{ID: cat.Project.ID}, Channel: "nightly", OS: "macos", Arch: "x86_64"})
	if err == nil {
		t.Fatal("unknown channel must fail")
	}
}

// TestSparklePrivateProjectSignedURL 私有项目 enclosure URL 经短时签名。
func TestSparklePrivateProjectSignedURL(t *testing.T) {
	cat := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, nil, "")
	cat.Project.StorageVisibility = model.StorageVisibilityPrivate
	doc, _ := renderAppcast(t, cat, func(deps *Deps, _ *Request) { deps.Signer = stubSigner{} })
	assertRequiredElements(t, doc)
	if !strings.HasPrefix(doc.Channel.Items[0].Enclosure.URL, "/api/v1/projects/demo/packages/aa?exp=123&sig=abc") {
		t.Fatalf("private enclosure url unsigned: %q", doc.Channel.Items[0].Enclosure.URL)
	}
}

// TestSparkleEdSignatureOverFileBytes Ed25519 项目：sparkle:edSignature 可用
// 项目公钥对 enclosure 文件字节验证（Sparkle EdDSA 语义：签文件而非元数据），
// 且签名按产物缓存——第二次渲染不再读文件。
func TestSparkleEdSignatureOverFileBytes(t *testing.T) {
	privPEM, pubPEM := genEd25519PEM(t)
	fileBytes := []byte("dmg-bytes-for-sparkle-signature-test")

	cat := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, nil, "")
	cat.Project.SigningPrivateKey = privPEM

	// 对齐 newTestCatalog 生成的 StorageKey。
	key := cat.Versions[0].Lines[0].FullPkgs[0].StorageKey
	storage := newFakeStorage(map[string][]byte{key: fileBytes})

	cache := NewSignatureCache(0)
	adapter := NewSparkleAdapterWithCache(cache)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}

	render := func() string {
		resp, err := adapter.Render(context.Background(), deps, Request{
			Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
			Channel: "stable", OS: "macos", Arch: "x86_64",
		})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		var doc appcastDoc
		if err := xml.Unmarshal(resp.Body, &doc); err != nil {
			t.Fatalf("unmarshal: %v\n%s", err, resp.Body)
		}
		return doc.Channel.Items[0].EdSignature
	}

	sig := render()
	if sig == "" {
		t.Fatal("ed25519 project must emit sparkle:edSignature")
	}
	// 用项目公钥对文件原始字节验证（验收：edSignature 可验证）。
	if err := signature.VerifyPayload(signature.AlgoEd25519, pubPEM, string(fileBytes), sig); err != nil {
		t.Fatalf("edSignature must verify over file bytes: %v", err)
	}

	// 第二次渲染：LRU 命中，文件不再读取。
	if sig != render() {
		t.Fatal("signature must be stable across renders")
	}
	if n := storage.readCount(key); n != 1 {
		t.Fatalf("artifact must be read once (LRU), got %d", n)
	}

	// 签名必须与原生 check 元数据签名无关：对 check 载荷验证必须失败。
	payload := signature.BuildCheckPayload("11", "1.1.0", "", "/u", "100", "aa")
	if err := signature.VerifyPayload(signature.AlgoEd25519, pubPEM, payload, sig); err == nil {
		t.Fatal("edSignature must NOT verify over native check payload (signs file bytes only)")
	}
}

// TestSparkleRSAProjectNoSignature RSA 项目不发 sparkle:dsaSignature
// （本服务不支持 DSA；Sparkle 2 主推 EdDSA，元素省略即未签名）。
func TestSparkleRSAProjectNoSignature(t *testing.T) {
	cat := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, nil, "")
	cat.Project.SigningAlgo = model.SigningAlgoRSASHA256
	storage := newFakeStorage(nil)

	doc, body := renderAppcast(t, cat, func(deps *Deps, _ *Request) { deps.Storage = storage })
	assertRequiredElements(t, doc)
	if doc.Channel.Items[0].EdSignature != "" || strings.Contains(string(body), "edSignature") || strings.Contains(string(body), "dsaSignature") {
		t.Fatalf("RSA project must not emit signature element:\n%s", body)
	}
}

// TestSparkleEdSignatureWithoutStorage 私钥已配置但无存储依赖：降级为未签名。
func TestSparkleEdSignatureWithoutStorage(t *testing.T) {
	cat := newTestCatalog(t, "demo", "stable", "1.1.0", 11, 100, false, model.VersionLineStatusReady, nil, "")
	cat.Project.SigningPrivateKey = "not-used"
	doc, _ := renderAppcast(t, cat, nil)
	assertRequiredElements(t, doc)
	if doc.Channel.Items[0].EdSignature != "" {
		t.Fatal("no storage dependency must skip signature element")
	}
}
