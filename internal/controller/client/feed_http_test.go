package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	corand "crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
	"github.com/Kirizu-Official/KiriVers/pkg/urlsign"
)

// genFeedEd25519PEM 生成测试用 Ed25519 密钥对（PKCS#8 私钥 / PKIX 公钥 PEM）。
func genFeedEd25519PEM(t *testing.T) (privPEM, pubPEM string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(corand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return privPEM, pubPEM
}

// ---------- 商店协议 feed HTTP 集成测试（§9 / §9.2，C16-1..C16-8） ----------
//
// 走真实路由装配（client.Register → StoreAuth → internal/controller/client/store），
// 复用 check_test.go 的 setupCheckTest / publishSingleFile / perform / ptr。

const feedSparkleNS = "http://www.andymatuschak.org/xml-namespaces/sparkle"

// feedAppcastDoc / feedAppcastItem 是 Sparkle appcast 的解析目标（官方 schema）。
type feedAppcastDoc struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel struct {
		Title       string            `xml:"title"`
		Link        string            `xml:"link"`
		Description string            `xml:"description"`
		Items       []feedAppcastItem `xml:"item"`
	} `xml:"channel"`
}

type feedAppcastItem struct {
	Title          string `xml:"title"`
	PubDate        string `xml:"pubDate"`
	SparkleVersion string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle version"`
	SparkleShort   string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
	SparkleOS      string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle os"`
	SparkleChannel string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle channel"`
	SparkleMinSys  string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle minimumSystemVersion"`
	Description    string `xml:"description"`
	Enclosure      struct {
		URL    string `xml:"url,attr"`
		Length int64  `xml:"length,attr"`
		Type   string `xml:"type,attr"`
	} `xml:"enclosure"`
	EdSignature string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle edSignature"`
}

// parseFeedAppcast 解析 appcast body 并断言官方必填元素齐全
// （rss@version=2.0、channel title/link/description、item
// title/pubDate(RFC 822)/description/enclosure url+length+type）。
func parseFeedAppcast(t *testing.T, body []byte) *feedAppcastDoc {
	t.Helper()
	var doc feedAppcastDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal appcast: %v\n%s", err, body)
	}
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
	return &doc
}

// enableSparklePatch 返回启用 sparkle 协议的 Patch 输入。
// TestFeedAppcastEndToEnd 官方必填元素、双号映射（验收 1、2）、默认变体
// enclosure（验收 5）、下载可达、别名规范化（§4.1）、ETag/304 与缓存头。
func TestFeedAppcastEndToEnd(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "demo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "sparkle", "default", nil)

	payload1 := bytes.Repeat([]byte("x"), 100)
	payload2 := bytes.Repeat([]byte("y"), 200)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 100, "demo-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) {
			in.Changelog = ptr("changes for 1.0.0")
			in.VersionInteger = ptr(int64(10))
		})
	// 1.1.0 手工装配：默认变体 + hw 变体都在发布前上传（ARTIFACT_IMMUTABLE）。
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("changes for 1.1.0"),
		VersionInteger: ptr(int64(11)),
	}); err != nil {
		t.Fatal(err)
	}
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload2))
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "macos", "x86_64", service.UploadArtifactInput{
		Filename:       "demo-1.1.0-macos-x86_64.dmg",
		ExpectedSHA256: sha,
		Size:           int64(len(payload2)),
		ContentType:    "application/x-apple-diskimage",
	}, bytes.NewReader(payload2)); err != nil {
		t.Fatalf("upload default variant: %v", err)
	}
	// hw 变体（C16-8：不传 hw 不得收到非默认变体）。
	if _, err := projSvc.CreateHwRev(ctx, p.ID, service.HwRevWrite{Slug: ptr("reva"), Rank: ptr(10)}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "macos", "x86_64", service.UploadArtifactInput{
		Filename:       "demo-1.1.0-macos-x86_64-reva.dmg",
		ExpectedSHA256: sha,
		Size:           int64(len(payload2)),
		HwRev:          ptr("reva"),
		ContentType:    "application/x-apple-diskimage",
	}, bytes.NewReader(payload2)); err != nil {
		t.Fatalf("upload hw variant: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "macos", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	_ = payload1

	// 别名入口（darwin/amd64 → macos/x86_64，§4.1）。
	w := perform(r, http.MethodGet, "/api/v1/projects/demo/store/sparkle/default/appcast.xml?os=darwin&arch=amd64")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.Bytes()

	// Content-Type / 缓存头（§9.2，框架层统一写）。
	if ct := w.Header().Get("Content-Type"); ct != "application/xml; charset=utf-8" {
		t.Fatalf("content-type=%q", ct)
	}
	etag := w.Header().Get("ETag")
	if etag == "" || !strings.HasPrefix(etag, `"`) {
		t.Fatalf("missing strong ETag: %q", etag)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, s-maxage=60, stale-while-revalidate=30" {
		t.Fatalf("cache-control=%q", cc)
	}
	if v := w.Header().Get("Vary"); !strings.Contains(v, "Accept-Encoding") {
		t.Fatalf("vary=%q", v)
	}

	// 结构与双号解析。
	doc := parseFeedAppcast(t, body)
	if !strings.Contains(string(body), `xmlns:sparkle="`+feedSparkleNS+`"`) {
		t.Fatalf("missing sparkle namespace declaration:\n%s", body)
	}
	if len(doc.Channel.Items) != 2 {
		t.Fatalf("items=%d want 2\n%s", len(doc.Channel.Items), body)
	}
	// 双号映射（C16-2）：sparkle:version = version_integer，
	// sparkle:shortVersionString = version_semver。
	if doc.Channel.Items[0].SparkleVersion != "11" || doc.Channel.Items[0].SparkleShort != "1.1.0" {
		t.Fatalf("dual numbers wrong on newest: %q/%q", doc.Channel.Items[0].SparkleVersion, doc.Channel.Items[0].SparkleShort)
	}
	if doc.Channel.Items[1].SparkleVersion != "10" || doc.Channel.Items[1].SparkleShort != "1.0.0" {
		t.Fatalf("dual numbers wrong on oldest: %q/%q", doc.Channel.Items[1].SparkleVersion, doc.Channel.Items[1].SparkleShort)
	}
	// 不传 hw：enclosure 指向默认变体全量包哈希 URL，且真实可下载。
	enc := doc.Channel.Items[0].Enclosure.URL
	if !strings.Contains(enc, "/packages/") || strings.Contains(enc, "reva") {
		t.Fatalf("enclosure must point to default variant hash URL: %q", enc)
	}
	if !strings.Contains(enc, "/packages/") {
		t.Fatalf("enclosure must be hash URL: %q", enc)
	}
	if doc.Channel.Items[0].Enclosure.Type != "application/x-apple-diskimage" {
		t.Fatalf("enclosure type=%q", doc.Channel.Items[0].Enclosure.Type)
	}
	dl := perform(r, http.MethodGet, doc.Channel.Items[0].Enclosure.URL)
	if dl.Code != http.StatusOK {
		t.Fatalf("enclosure download status=%d", dl.Code)
	}
	if ct := dl.Header().Get("Content-Type"); ct != "application/x-apple-diskimage" {
		t.Fatalf("download content-type=%q", ct)
	}

	// 别名等价：规范名请求与别名请求 ETag 恒等。
	w2 := perform(r, http.MethodGet, "/api/v1/projects/demo/store/sparkle/default/appcast.xml?os=macos&arch=x86_64")
	if w2.Code != http.StatusOK || w2.Header().Get("ETag") != etag {
		t.Fatalf("canonical alias mismatch: status=%d etag=%q want=%q", w2.Code, w2.Header().Get("ETag"), etag)
	}

	// If-None-Match → 304。
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/demo/store/sparkle/default/appcast.xml?os=macos&arch=x86_64", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match status=%d want 304", rec.Code)
	}

	// 缺 os / arch → 400。未钉 channel 的 listing 忽略未知渠道 query（AC13：文档内表达全部公开渠道）。
	for _, q := range []string{"", "?os=macos", "?arch=x86_64"} {
		wq := perform(r, http.MethodGet, "/api/v1/projects/demo/store/sparkle/default/appcast.xml"+q)
		if wq.Code != http.StatusBadRequest {
			t.Fatalf("query %q status=%d want 400", q, wq.Code)
		}
	}
	wq := perform(r, http.MethodGet, "/api/v1/projects/demo/store/sparkle/default/appcast.xml?os=macos&arch=x86_64&channel=nightly")
	if wq.Code != http.StatusOK {
		t.Fatalf("unpinned listing ignores unknown channel query, status=%d want 200", wq.Code)
	}
}

// TestFeedProtocolDisabled404NonZip 协议未开启 / 未适配 → 404 且 body 不是
// 通用 zip（C16-4，验收 3）。
func TestFeedProtocolDisabled404NonZip(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	// 项目存在但 sparkle 关闭。
	slug := "closed"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	disabled := service.PatchProjectInput{}
	if _, _, err := projSvc.Patch(ctx, slug, disabled); err != nil {
		t.Fatal(err)
	}

	w := perform(r, http.MethodGet, "/api/v1/projects/closed/store/sparkle/default/appcast.xml?os=macos&arch=x86_64")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled protocol status=%d want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "zip") {
		t.Fatalf("404 must not be zip: %q", ct)
	}
	if strings.HasPrefix(w.Body.String(), "PK") {
		t.Fatalf("404 body must not be a zip archive")
	}

	// 未适配协议名 → 404。
	w2 := perform(r, http.MethodGet, "/api/v1/projects/closed/store/apt/dists/stable/Release?os=macos&arch=x86_64")
	if w2.Code != http.StatusNotFound {
		t.Fatalf("unknown protocol status=%d want 404", w2.Code)
	}

	// 项目不存在 → 404。
	w3 := perform(r, http.MethodGet, "/api/v1/projects/missing/store/sparkle/default/appcast.xml")
	if w3.Code != http.StatusNotFound {
		t.Fatalf("missing project status=%d want 404", w3.Code)
	}
}

// TestStoreAuth401WithStoreToken store_token 开启时缺 Token → 401 UNAUTHORIZED，
// 携带合法 X-Project-Token（feed token）→ 200（C16-7，验收 4）。
func TestStoreAuth401WithStoreToken(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "feed-secret-plaintext"
	slug := "tok"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "sparkle", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 10, "tok-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("tok release") })

	url := "/api/v1/projects/tok/store/sparkle/default/appcast.xml?os=macos&arch=x86_64"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token status=%d want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UNAUTHORIZED") {
		t.Fatalf("401 body=%s", w.Body.String())
	}

	// 合法 feed token（X-Project-Token 头）→ 200。
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Project-Token", secret)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid feed token status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestFeedGrayProjection 灰度 <100% 的非强制版本不出现在 appcast，
// 关键版本出现（C16-5，§4.4 匿名口径）。
func TestFeedGrayProjection(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "gray"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "sparkle", "default", nil)
	seedClients(t, projSvc, p, 2)

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 10, "g-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("g 1.0.0 notes") })
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "macos", "x86_64", 10, "g-1.1.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) {
			in.GrayStartPercent = ptr(50)
			in.Changelog = ptr("g 1.1.0 notes")
		})
	publishSingleFile(t, projSvc, ctx, p.ID, "1.2.0", "stable", "macos", "x86_64", 10, "g-1.2.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.IsCritical = ptr(true); in.Changelog = ptr("g 1.2.0 notes") })

	w := perform(r, http.MethodGet, "/api/v1/projects/gray/store/sparkle/default/appcast.xml?os=macos&arch=x86_64")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	doc := parseFeedAppcast(t, w.Body.Bytes())
	shorts := make([]string, 0, len(doc.Channel.Items))
	for _, it := range doc.Channel.Items {
		shorts = append(shorts, it.SparkleShort)
	}
	joined := strings.Join(shorts, ",")
	if strings.Contains(joined, "1.1.0") {
		t.Fatalf("gray 50%% non-mandatory version must be absent, got [%s]", joined)
	}
	for _, want := range []string{"1.0.0", "1.2.0"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("version %s must be visible, got [%s]", want, joined)
		}
	}
}

// TestFeedPrivateProjectSignedEnclosure 私有项目：enclosure URL 带短时签名、
// 缓存头 private no-store、签名 URL 可真实下载（§13.7 / C15-1）。
func TestFeedPrivateProjectSignedEnclosure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	signer := urlsign.NewSigner("test-secret", 0)
	r := gin.New()
	Register(r.Group("/api/v1"), projSvc,
		update.NewService(repository.NewMemoryUpdateCatalog(store)), nil, nil, signer, nil)
	ctx := context.Background()

	slug := "priv"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	markProjectPrivate(t, store, ctx, p)
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "sparkle", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 64, "priv-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("priv release") })

	w := perform(r, http.MethodGet, "/api/v1/projects/priv/store/sparkle/default/appcast.xml?os=macos&arch=x86_64")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("private project cache-control=%q", cc)
	}
	doc := parseFeedAppcast(t, w.Body.Bytes())
	encURL := doc.Channel.Items[0].Enclosure.URL
	if !strings.Contains(encURL, "?exp=") || !strings.Contains(encURL, "&sig=") {
		t.Fatalf("private enclosure must be signed: %q", encURL)
	}
	// 签名 URL 真实可下载。
	dl := perform(r, http.MethodGet, encURL)
	if dl.Code != http.StatusOK {
		t.Fatalf("signed enclosure download status=%d", dl.Code)
	}

	// 私有存储跳过 304 短路（§13.7）：即使 If-None-Match 命中也恒回新鲜
	// 200——304 会把客户端钉死在即将过期的签名 URL 上。
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/priv/store/sparkle/default/appcast.xml?os=macos&arch=x86_64", nil)
	req.Header.Set("If-None-Match", w.Header().Get("ETag"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("private project must skip 304 short-circuit, got %d", rec.Code)
	}
}

// TestFeedEdSignatureEndToEnd ed25519 项目：appcast 的 sparkle:edSignature
// 可用项目公钥对产物文件字节验证（Sparkle EdDSA 语义，与原生 check
// signature 字节格式无关）。
func TestFeedEdSignatureEndToEnd(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	privPEM, pubPEM := genFeedEd25519PEM(t)
	slug := "signed"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		SigningAlgo:       ptr(model.SigningAlgoEd25519),
		SigningPrivateKey: &privPEM,
		SigningPublicKey:  &pubPEM,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "sparkle", "default", nil)
	// publishSingleFile 的产物字节是确定性的（"x" × size），可直接重建验证。
	payload := bytes.Repeat([]byte("x"), 37)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 37, "signed-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("signed release") })

	w := perform(r, http.MethodGet, "/api/v1/projects/signed/store/sparkle/default/appcast.xml?os=macos&arch=x86_64")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	doc := parseFeedAppcast(t, w.Body.Bytes())
	sig := doc.Channel.Items[0].EdSignature
	if sig == "" {
		t.Fatal("ed25519 project must emit sparkle:edSignature")
	}
	// EdDSA 语义：对产物文件字节验证（不是对任何元数据载荷）。
	if err := signature.VerifyPayload(signature.AlgoEd25519, pubPEM, string(payload), sig); err != nil {
		t.Fatalf("edSignature must verify over file bytes: %v", err)
	}
}

// TestFeedSparkleTwoListingsDoNotMix AC12：同一协议两条 listing 钉死不同 os/channel
// 与 identifiers 时，各自 feed 不得混入对方产物。
func TestFeedSparkleTwoListingsDoNotMix(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "dual-sparkle"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}

	macOS, win, beta := "macos", "windows", "beta"
	src := model.PackageSourceLineFull
	enabled := true
	macSlug, winSlug := "macos-app", "win-beta"
	macProto, winProto := "sparkle", "sparkle"
	macIDs := model.IdentifierMap{"homepage": "https://mac.example/app"}
	winIDs := model.IdentifierMap{"homepage": "https://win.example/app"}
	if _, err := projSvc.CreateStoreListing(ctx, p.ID, service.StoreListingWrite{
		Protocol: &macProto, Slug: &macSlug, Enabled: &enabled, OS: &macOS,
		PackageSource: &src, Identifiers: &macIDs,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateStoreListing(ctx, p.ID, service.StoreListingWrite{
		Protocol: &winProto, Slug: &winSlug, Enabled: &enabled, OS: &win, Channel: &beta,
		PackageSource: &src, Identifiers: &winIDs,
	}); err != nil {
		t.Fatal(err)
	}

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 111, "dual-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) {
			in.VersionInteger = ptr(int64(10))
			in.Changelog = ptr("mac release")
		})
	publishSingleFile(t, projSvc, ctx, p.ID, "2.0.0", "beta", "windows", "x86_64", 222, "dual-2.0.0-windows-x86_64.exe",
		func(in *service.VersionWriteInput) {
			in.VersionInteger = ptr(int64(20))
			in.Changelog = ptr("win release")
		})

	mac := perform(r, http.MethodGet, "/api/v1/projects/dual-sparkle/store/sparkle/macos-app/appcast.xml?os=windows&arch=x86_64&channel=beta")
	if mac.Code != http.StatusOK {
		t.Fatalf("macos listing status=%d body=%s", mac.Code, mac.Body.String())
	}
	macDoc := parseFeedAppcast(t, mac.Body.Bytes())
	if macDoc.Channel.Link != "https://mac.example/app" {
		t.Fatalf("macos listing link=%q", macDoc.Channel.Link)
	}
	if len(macDoc.Channel.Items) != 1 || macDoc.Channel.Items[0].SparkleVersion != "10" {
		t.Fatalf("macos listing items=%+v", macDoc.Channel.Items)
	}
	if !strings.Contains(macDoc.Channel.Items[0].Enclosure.URL, "/packages/") {
		t.Fatalf("enclosure=%s", macDoc.Channel.Items[0].Enclosure.URL)
	}
	if strings.Contains(mac.Body.String(), "win-beta") || strings.Contains(mac.Body.String(), "2.0.0") {
		t.Fatalf("macos listing mixed windows package: %s", mac.Body.String())
	}

	winFeed := perform(r, http.MethodGet, "/api/v1/projects/dual-sparkle/store/sparkle/win-beta/appcast.xml?os=macos&arch=x86_64&channel=stable")
	if winFeed.Code != http.StatusOK {
		t.Fatalf("windows listing status=%d body=%s", winFeed.Code, winFeed.Body.String())
	}
	winDoc := parseFeedAppcast(t, winFeed.Body.Bytes())
	if winDoc.Channel.Link != "https://win.example/app" {
		t.Fatalf("windows listing link=%q", winDoc.Channel.Link)
	}
	if len(winDoc.Channel.Items) != 1 || winDoc.Channel.Items[0].SparkleVersion != "20" {
		t.Fatalf("windows listing items=%+v", winDoc.Channel.Items)
	}
	if strings.Contains(winFeed.Body.String(), "macos-app") || strings.Contains(winFeed.Body.String(), "1.0.0") {
		t.Fatalf("windows listing mixed macos package: %s", winFeed.Body.String())
	}
}

// TestFeedOldPathWithoutListingSlug404 AC15：旧路径 /store/{protocol}/{doc} 必须 404。
func TestFeedOldPathWithoutListingSlug404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "oldpath"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "sparkle", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 40, "old-1.0.0-macos-x86_64.dmg")

	w := perform(r, http.MethodGet, "/api/v1/projects/oldpath/store/sparkle/appcast.xml")
	if w.Code != http.StatusNotFound {
		t.Fatalf("old feed path must 404, got %d body=%s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		t.Fatalf("old feed path should be plain 404, content-type=%q body=%s", ct, w.Body.String())
	}
}

func TestStoreOldFeedPrefix404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "oldfeed"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "sparkle", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 40, "old-1.0.0-macos-x86_64.dmg")

	ok := perform(r, http.MethodGet, "/api/v1/projects/oldfeed/store/sparkle/default/appcast.xml?os=macos&arch=x86_64")
	if ok.Code != http.StatusOK {
		t.Fatalf("store path must 200, got %d %s", ok.Code, ok.Body.String())
	}
	w := perform(r, http.MethodGet, "/api/v1/projects/oldfeed/feed/sparkle/default/appcast.xml?os=macos&arch=x86_64")
	if w.Code != http.StatusNotFound {
		t.Fatalf("/feed/ prefix must 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestFeedSparklePinnedBetaOmitsStableSameOS AC13：未钉 channel 的 Sparkle listing
// 在同一 OS 文档内同时表达 stable 与 beta；钉死 beta 的 listing 即使 query 写
// channel=stable 也不得混入 stable 条目。
func TestFeedSparklePinnedBetaOmitsStableSameOS(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "sparkle-channels"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}

	enableListing(t, projSvc, ctx, p.ID, "sparkle", "all-channels", nil)
	beta := "beta"
	proto, listingSlug, src, enabled := "sparkle", "beta-only", model.PackageSourceLineFull, true
	if _, err := projSvc.CreateStoreListing(ctx, p.ID, service.StoreListingWrite{
		Protocol: &proto, Slug: &listingSlug, Enabled: &enabled, Channel: &beta, PackageSource: &src,
	}); err != nil {
		t.Fatal(err)
	}

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 80, "ch-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) {
			in.VersionInteger = ptr(int64(10))
			in.Changelog = ptr("stable macos")
		})
	publishSingleFile(t, projSvc, ctx, p.ID, "2.0.0", "beta", "macos", "x86_64", 90, "ch-2.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) {
			in.VersionInteger = ptr(int64(20))
			in.Changelog = ptr("beta macos")
		})

	unpinned := perform(r, http.MethodGet, "/api/v1/projects/sparkle-channels/store/sparkle/all-channels/appcast.xml?os=macos&arch=x86_64&channel=stable")
	if unpinned.Code != http.StatusOK {
		t.Fatalf("unpinned status=%d body=%s", unpinned.Code, unpinned.Body.String())
	}
	unpinnedDoc := parseFeedAppcast(t, unpinned.Body.Bytes())
	if len(unpinnedDoc.Channel.Items) != 2 {
		t.Fatalf("unpinned listing must express both public channels, items=%d body=%s", len(unpinnedDoc.Channel.Items), unpinned.Body.String())
	}
	sawStable, sawBeta := false, false
	for _, item := range unpinnedDoc.Channel.Items {
		switch item.SparkleVersion {
		case "10":
			sawStable = true
			if item.SparkleChannel != "" {
				t.Fatalf("stable item must omit sparkle:channel, got %q", item.SparkleChannel)
			}
		case "20":
			sawBeta = true
			if item.SparkleChannel != "beta" {
				t.Fatalf("beta item sparkle:channel=%q", item.SparkleChannel)
			}
		}
	}
	if !sawStable || !sawBeta {
		t.Fatalf("unpinned listing missing channel items: %+v", unpinnedDoc.Channel.Items)
	}

	pinned := perform(r, http.MethodGet, "/api/v1/projects/sparkle-channels/store/sparkle/beta-only/appcast.xml?os=macos&arch=x86_64&channel=stable")
	if pinned.Code != http.StatusOK {
		t.Fatalf("pinned beta status=%d body=%s", pinned.Code, pinned.Body.String())
	}
	pinnedDoc := parseFeedAppcast(t, pinned.Body.Bytes())
	if len(pinnedDoc.Channel.Items) != 1 {
		t.Fatalf("pinned beta must omit stable, items=%d body=%s", len(pinnedDoc.Channel.Items), pinned.Body.String())
	}
	if pinnedDoc.Channel.Items[0].SparkleVersion != "20" || pinnedDoc.Channel.Items[0].SparkleChannel != "beta" {
		t.Fatalf("pinned beta item=%+v", pinnedDoc.Channel.Items[0])
	}
	if strings.Contains(pinned.Body.String(), "1.0.0") || pinnedDoc.Channel.Items[0].SparkleVersion == "10" {
		t.Fatalf("pinned beta listing mixed stable: %s", pinned.Body.String())
	}
}

// TestFeedManifestPathSetupExeSHA256 AC14：多文件 listing 的 manifest_path=Setup.exe
// 必须投影该文件 sha256（不是 kind=full zip）；缺该文件的版本被跳过。
func TestFeedManifestPathSetupExeSHA256(t *testing.T) {
	r, projSvc, store, ctx := setupCheckTest(t)
	slug := "win-setup"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	pkgType := model.PackageTypeMultiFile
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: &pkgType,
	}); err != nil {
		t.Fatal(err)
	}

	src := model.PackageSourceManifestPath
	pathSel := "Setup.exe"
	proto, listingSlug, enabled := "sparkle", "installer", true
	if _, err := projSvc.CreateStoreListing(ctx, p.ID, service.StoreListingWrite{
		Protocol: &proto, Slug: &listingSlug, Enabled: &enabled, PackageSource: &src, ManifestPath: &pathSel,
	}); err != nil {
		t.Fatal(err)
	}

	setupBytes := []byte("MZ-setup-installer-bytes")
	setupSHA, _ := hashutil.SHA256Hex(bytes.NewReader(setupBytes))
	zipPayload := mkZipPayload(t, 1024)
	zipSHA, _ := hashutil.SHA256Hex(bytes.NewReader(zipPayload))
	if zipSHA == setupSHA {
		t.Fatal("fixture zip and Setup.exe hashes must differ")
	}

	publishMultiFileWithSetup(t, projSvc, store, ctx, p, "1.0.0", "stable", 10, zipPayload, setupBytes, setupSHA, true)
	publishMultiFileWithSetup(t, projSvc, store, ctx, p, "2.0.0", "stable", 20, zipPayload, nil, "", false)

	w := perform(r, http.MethodGet, "/api/v1/projects/win-setup/store/sparkle/installer/appcast.xml?os=windows&arch=x86_64")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	doc := parseFeedAppcast(t, w.Body.Bytes())
	if len(doc.Channel.Items) != 1 {
		t.Fatalf("version without Setup.exe must be skipped, items=%d body=%s", len(doc.Channel.Items), w.Body.String())
	}
	enc := doc.Channel.Items[0].Enclosure.URL
	if !strings.Contains(enc, "/packages/"+setupSHA) {
		t.Fatalf("enclosure must use Setup.exe sha256, url=%q zip=%s", enc, zipSHA)
	}
	if strings.Contains(enc, zipSHA) {
		t.Fatalf("enclosure must not be the zip hash: %q", enc)
	}
	dl := perform(r, http.MethodGet, enc)
	if dl.Code != http.StatusOK {
		t.Fatalf("Setup.exe download status=%d", dl.Code)
	}
	if !bytes.Equal(dl.Body.Bytes(), setupBytes) {
		t.Fatal("downloaded bytes must match Setup.exe, not the zip")
	}
}

func publishMultiFileWithSetup(t *testing.T, projSvc *service.ProjectService, store repository.ProjectStore, ctx context.Context, p *model.Project, versionRef, channel string, versionInteger int64, zipPayload, setupBytes []byte, setupSHA string, includeSetup bool) {
	t.Helper()
	if _, _, err := projSvc.PutVersion(ctx, p.ID, versionRef, service.VersionWriteInput{
		Channel:        channel,
		Changelog:      ptr("win " + versionRef),
		VersionInteger: ptr(versionInteger),
	}); err != nil {
		t.Fatalf("put %s: %v", versionRef, err)
	}
	zipSHA, _ := hashutil.SHA256Hex(bytes.NewReader(zipPayload))
	fname := fmt.Sprintf("demo-%s-windows-x86_64.zip", versionRef)
	full, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), versionRef, "windows", "x86_64",
		service.UploadArtifactInput{Filename: fname, ExpectedSHA256: zipSHA, Size: int64(len(zipPayload))},
		bytes.NewReader(zipPayload))
	if err != nil {
		t.Fatalf("upload zip %s: %v", versionRef, err)
	}

	entries := []service.ManifestEntryInput{{
		Path: "README.txt", Size: 4, SHA256: integritySHA, MD5: integrityMD5, InstallPolicy: "OVERWRITE",
	}}
	if includeSetup {
		entries = append(entries, service.ManifestEntryInput{
			Path: "Setup.exe", Size: int64(len(setupBytes)), SHA256: setupSHA, MD5: integrityMD5, InstallPolicy: "OVERWRITE",
		})
	}
	if _, err := projSvc.SetManifest(ctx, fmt.Sprint(p.ID), versionRef, "windows", "x86_64", entries); err != nil {
		t.Fatalf("manifest %s: %v", versionRef, err)
	}

	if includeSetup {
		v, err := projSvc.ResolveVersion(ctx, p.ID, versionRef)
		if err != nil {
			t.Fatal(err)
		}
		fileID := uuid.New()
		key := storage.ArtifactObjectKey(p.Slug, setupSHA)
		if err := projSvc.Storage().Put(ctx, key, bytes.NewReader(setupBytes), int64(len(setupBytes)), "application/vnd.microsoft.portable-executable"); err != nil {
			t.Fatalf("put Setup.exe: %v", err)
		}
		if err := store.CreateArtifact(ctx, &model.Artifact{
			ID: fileID, ProjectID: p.ID, VersionID: v.ID, VersionLineID: full.VersionLineID,
			Kind: model.ArtifactKindFile, FileName: "Setup.exe", StorageKey: key,
			Size: int64(len(setupBytes)), SHA256: setupSHA, MD5: integrityMD5,
			ContentType: "application/vnd.microsoft.portable-executable",
		}); err != nil {
			t.Fatalf("create Setup.exe artifact: %v", err)
		}
	}

	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, versionRef, "windows", "x86_64"); err != nil {
		t.Fatalf("ready %s: %v", versionRef, err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, versionRef); err != nil {
		t.Fatalf("publish %s: %v", versionRef, err)
	}
}

func setupFeedLineFullProject(t *testing.T, projSvc *service.ProjectService, store repository.ProjectStore, ctx context.Context, slug string) (*model.Project, string, string) {
	t.Helper()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	pkgType := model.PackageTypeMultiFile
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: &pkgType,
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "sparkle", "default", nil)
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("feed full zip"),
		VersionInteger: ptr(int64(10)),
	}); err != nil {
		t.Fatal(err)
	}
	payload := zipNamedFiles(t, map[string]string{"dir/a.txt": "aaa-bytes", "dir/b.txt": "bbb-bytes"})
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.0.0", "windows", "x86_64",
		service.UploadArtifactInput{Filename: "app.zip", ExpectedSHA256: sha, Size: int64(len(payload))},
		bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload zip: %v", err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	v, err := projSvc.ResolveVersion(ctx, p.ID, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	arts, err := store.ListArtifactsByVersionID(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	var fullSHA, feedSHA string
	for i := range arts {
		switch arts[i].Kind {
		case model.ArtifactKindFull:
			fullSHA = arts[i].SHA256
		case model.ArtifactKindStoreFull:
			feedSHA = arts[i].SHA256
		}
	}
	if fullSHA == "" || feedSHA == "" {
		t.Fatalf("expected both full and store_full, arts=%d full=%q feed=%q", len(arts), fullSHA, feedSHA)
	}
	if fullSHA == feedSHA {
		t.Fatal("native full SHA must differ from store_full SHA")
	}
	return p, fullSHA, feedSHA
}

func TestStoreLineFullUsesStoreFullSHANotNativeFull(t *testing.T) {
	r, projSvc, store, ctx := setupCheckTest(t)
	p, fullSHA, feedSHA := setupFeedLineFullProject(t, projSvc, store, ctx, "feed-full-sep")
	w := perform(r, http.MethodGet, "/api/v1/projects/"+p.Slug+"/store/sparkle/default/appcast.xml?os=windows&arch=x86_64")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	doc := parseFeedAppcast(t, w.Body.Bytes())
	enc := doc.Channel.Items[0].Enclosure.URL
	if !strings.Contains(enc, "/packages/"+feedSHA) {
		t.Fatalf("enclosure must use store_full sha256, url=%q feed=%s full=%s", enc, feedSHA, fullSHA)
	}
	if strings.Contains(enc, fullSHA) {
		t.Fatalf("enclosure must not be native hash-root full: %q", enc)
	}
}

func TestStoreLineFullMissingStoreFullSkipsVersion(t *testing.T) {
	r, projSvc, store, ctx := setupCheckTest(t)
	p, _, _ := setupFeedLineFullProject(t, projSvc, store, ctx, "feed-full-skip")
	v, err := projSvc.ResolveVersion(ctx, p.ID, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	arts, err := store.ListArtifactsByVersionID(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range arts {
		if arts[i].Kind == model.ArtifactKindStoreFull {
			if err := store.DeleteArtifact(ctx, arts[i].ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	w := perform(r, http.MethodGet, "/api/v1/projects/"+p.Slug+"/store/sparkle/default/appcast.xml?os=windows&arch=x86_64")
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing store_full must skip line_full (404), got %d %s", w.Code, w.Body.String())
	}
}
