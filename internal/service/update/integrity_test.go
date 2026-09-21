package update

import (
	"context"
	"crypto/ed25519"
	corand "crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

// ---------- integrity 夹具 ----------

// testSHA256 / testMD5 是合法长度的占位哈希（Manifest 行不参与真实校验）。
var (
	testSHA256 = strings.Repeat("a", 64)
	testMD5    = strings.Repeat("b", 32)
)

// fakeLineDetails 是内存版 LineDetailSource：按 lineID 返回预置明细。
type fakeLineDetails struct {
	byLine map[uuid.UUID]*LineDetail
}

func (f *fakeLineDetails) LineDetails(_ context.Context, id uuid.UUID) (*LineDetail, error) {
	if d, ok := f.byLine[id]; ok {
		return d, nil
	}
	return &LineDetail{}, nil
}

// mkManifestEntry 构造 Manifest 条目快照。
func mkManifestEntry(path string, size int64, policy string) model.ManifestEntry {
	e := model.ManifestEntry{
		ID:            uuid.New(),
		Path:          path,
		Size:          size,
		SHA256:        testSHA256,
		MD5:           testMD5,
		InstallPolicy: policy,
	}
	if policy == model.InstallPolicyKeepIfExists {
		e.IntegrityCheck = false
	} else {
		e.IntegrityCheck = true
	}
	return e
}

// mkIDLine 构造带 ID 的平台切片（integrity/diff 按需读取明细需要 line.ID）。
func mkIDLine(id uuid.UUID, os, arch, status, rootHash string, pkgs ...ArtifactInfo) LineState {
	ls := LineState{
		ID:       id,
		OS:       os,
		Arch:     arch,
		Status:   status,
		RootHash: rootHash,
		FullPkgs: pkgs,
	}
	if status == model.VersionLineStatusReady {
		t := testGrayCompleteAt
		ls.PacksReadyAt = &t
	}
	return ls
}

// integrityFixture 构造多文件线：1.0.0 published + windows/x86_64 ready +
// 5 条 Manifest（其中一条 KEEP）+ 全量包 1000 字节。
func integrityFixture() (*Catalog, *fakeLineDetails, uuid.UUID) {
	vs := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	lineID := uuid.New()
	vs.Lines = append(vs.Lines, mkIDLine(lineID, "windows", "x86_64",
		model.VersionLineStatusReady, "roothash-1.0.0",
		mkPkg("demo-1.0.0-windows-x86_64.zip", 1000, nil)))

	paths := []string{"a/one", "b/two", "c/three", "d/four", "e/five"}
	entries := make([]model.ManifestEntry, 0, len(paths))
	for i, p := range paths {
		policy := model.InstallPolicyOverwrite
		if i == 4 {
			policy = model.InstallPolicyKeepIfExists
		}
		entries = append(entries, mkManifestEntry(p, int64(10+i), policy))
	}
	details := &fakeLineDetails{byLine: map[uuid.UUID]*LineDetail{
		lineID: {Manifest: entries},
	}}

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, vs)
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeMultiFile}
	return cat, details, lineID
}

// integrityInput 构造默认 integrity 请求。
func integrityInput() IntegrityInput {
	return IntegrityInput{Version: "1.0.0", OS: "windows", Arch: "x86_64"}
}

// ed25519KeyPair 生成测试用 PEM 密钥对。
func ed25519KeyPair(t *testing.T) (privPEM, pubPEM string) {
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
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
}

// ---------- integrity 测试 ----------

// 200 快照：全字段（root_hash、full_package_url、双号、package_type、signature、
// KEEP 行 integrity_check=false）、分页默认 limit、条目按 Path 升序。
func TestIntegrity200Shape(t *testing.T) {
	cat, details, _ := integrityFixture()
	cat.Project.SigningAlgo = signature.AlgoEd25519
	cat.Project.SigningPrivateKey, _ = ed25519KeyPair(t)

	res, err := Integrity(context.Background(), cat, details, integrityInput())
	if err != nil {
		t.Fatal(err)
	}
	b := res.Body
	if b.RootHash != "roothash-1.0.0" {
		t.Fatalf("header fields wrong: %+v", b)
	}
	if b.VersionInteger == nil || *b.VersionInteger != 10 || b.VersionSemver == nil || *b.VersionSemver != "1.0.0" {
		t.Fatalf("dual numbers wrong: %+v", b)
	}
	if b.Channel != "stable" || b.PackageType != model.PackageTypeMultiFile {
		t.Fatalf("channel/package_type wrong: %+v", b)
	}
	if b.FullPackageURL != "/api/v1/projects//packages/sha-demo-1.0.0-windows-x86_64.zip" ||
		b.Size != 1000 || b.SHA256 != "sha-demo-1.0.0-windows-x86_64.zip" ||
		b.FileName != "demo-1.0.0-windows-x86_64.zip" {
		t.Fatalf("full package triple wrong: %+v", b)
	}
	if len(b.Files) != 5 {
		t.Fatalf("expected 5 files, got %d", len(b.Files))
	}
	// Path 升序确定性；KEEP 行 integrity_check=false（C09-1）。
	if b.Files[0].Path != "a/one" || b.Files[4].Path != "e/five" {
		t.Fatalf("files not sorted: %+v", b.Files)
	}
	if b.Files[4].InstallPolicy != model.InstallPolicyKeepIfExists || b.Files[4].IntegrityCheck {
		t.Fatalf("KEEP row must carry integrity_check=false: %+v", b.Files[4])
	}
	// 默认 hash_algo=sha256：有 sha256、无 md5、默认无逐文件 url。
	for _, f := range b.Files {
		if f.SHA256 != testSHA256 || f.MD5 != "" || f.URL != "" {
			t.Fatalf("default hash/url wrong: %+v", f)
		}
	}
	if b.Signature == "" {
		t.Fatalf("signature must be present with signing key configured")
	}
	// 签名载荷与 check 同一套：双号\nroot_hash\nfull_package_url\nsize\nsha256。
	privPEM, pubPEM := ed25519KeyPair(t)
	cat.Project.SigningPrivateKey = privPEM
	res, err = Integrity(context.Background(), cat, details, integrityInput())
	if err != nil {
		t.Fatal(err)
	}
	payload := signature.BuildCheckPayload("10", "1.0.0", "roothash-1.0.0", b.FullPackageURL, "1000", b.SHA256)
	if err := signature.VerifyPayload(signature.AlgoEd25519, pubPEM, payload, res.Body.Signature); err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}
}

// ETag = "<line.RootHash>"（强 ETag）；If-None-Match 命中语义与缓存头。
func TestIntegrityETag(t *testing.T) {
	cat, details, _ := integrityFixture()
	res, err := Integrity(context.Background(), cat, details, integrityInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.ETag != `"roothash-1.0.0"` {
		t.Fatalf("etag = %s", res.ETag)
	}
	if !MatchesETag(`"roothash-1.0.0"`, res.ETag) || !MatchesETag(`W/`+res.ETag, res.ETag) {
		t.Fatalf("if-none-match matching failed")
	}
	if MatchesETag(`"other"`, res.ETag) {
		t.Fatalf("unrelated etag must not match")
	}
	// 缓存头复用 check public 分支 + Vary 同规则。
	if res.CacheControl != "public, s-maxage=60, stale-while-revalidate=30" {
		t.Fatalf("cache-control = %s", res.CacheControl)
	}
	if !equalStrings(res.Vary, []string{"Accept-Encoding"}) {
		t.Fatalf("vary = %v", res.Vary)
	}
	// RootHash 变化 → ETag 变化（Publish/产物变更即失效）。
	cat.Versions[0].Lines[0].RootHash = "roothash-2"
	res2, _ := Integrity(context.Background(), cat, details, integrityInput())
	if res2.ETag == res.ETag {
		t.Fatalf("root hash change must change etag")
	}
}

// 状态闸：Draft 404 VERSION_NOT_VISIBLE；Revoked 409；未知 404；
// 无线/未就绪线 404 VERSION_LINE_NOT_FOUND（C09-2）。
func TestIntegrityStatusGates(t *testing.T) {
	cat, details, lineID := integrityFixture()

	cat.Versions[0].Version.Status = model.VersionStatusDraft
	if _, err := Integrity(context.Background(), cat, details, integrityInput()); !errors.Is(err, ErrVersionNotVisible) {
		t.Fatalf("draft must be VERSION_NOT_VISIBLE, got %v", err)
	}

	cat.Versions[0].Version.Status = model.VersionStatusRevoked
	if _, err := Integrity(context.Background(), cat, details, integrityInput()); !errors.Is(err, ErrVersionRevoked) {
		t.Fatalf("revoked must be VERSION_REVOKED, got %v", err)
	}

	// Deprecated 开放（模型约定 RootHash 仍在）。
	cat.Versions[0].Version.Status = model.VersionStatusDeprecated
	if _, err := Integrity(context.Background(), cat, details, integrityInput()); err != nil {
		t.Fatalf("deprecated must be served: %v", err)
	}

	// 未知版本。
	cat.Versions[0].Version.Status = model.VersionStatusPublished
	in := integrityInput()
	in.Version = "999"
	if _, err := Integrity(context.Background(), cat, details, in); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("unknown must be VERSION_NOT_FOUND, got %v", err)
	}

	// 线未就绪。
	cat.Versions[0].Lines[0].Status = model.VersionLineStatusYanked
	if _, err := Integrity(context.Background(), cat, details, integrityInput()); !errors.Is(err, ErrVersionLineNotFound) {
		t.Fatalf("yanked line must be VERSION_LINE_NOT_FOUND, got %v", err)
	}

	// 无此平台线。
	in = integrityInput()
	in.OS, in.Arch = "linux", "x86_64"
	if _, err := Integrity(context.Background(), cat, details, in); !errors.Is(err, ErrVersionLineNotFound) {
		t.Fatalf("missing line must be VERSION_LINE_NOT_FOUND, got %v", err)
	}

	_ = lineID
}

// channel 校验（C09-10）：传入不等于 Version 渠道 → CHANNEL_CONFLICT；一致/缺省放行。
func TestIntegrityChannelConflict(t *testing.T) {
	cat, details, _ := integrityFixture()

	in := integrityInput()
	in.Channel = "beta"
	if _, err := Integrity(context.Background(), cat, details, in); !errors.Is(err, ErrChannelConflict) {
		t.Fatalf("expected CHANNEL_CONFLICT, got %v", err)
	}
	in.Channel = "stable"
	if _, err := Integrity(context.Background(), cat, details, in); err != nil {
		t.Fatalf("matching channel must pass: %v", err)
	}
}

// 无分页：一次返回全部 Manifest 条目。
func TestIntegrityReturnsAllFiles(t *testing.T) {
	cat, details, _ := integrityFixture()
	res, err := Integrity(context.Background(), cat, details, integrityInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Body.Files) != 5 {
		t.Fatalf("must return all 5 files, got %d", len(res.Body.Files))
	}
	if res.Body.Files[0].Path != "a/one" || res.Body.Files[4].Path != "e/five" {
		t.Fatalf("files not sorted: %+v", res.Body.Files)
	}
}

// hash_algo / compact：md5-only、both、compact 省 md5；非法值 400。
func TestIntegrityHashAlgoAndCompact(t *testing.T) {
	cat, details, _ := integrityFixture()
	ctx := context.Background()

	in := integrityInput()
	in.HashAlgo = "md5"
	res, err := Integrity(ctx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.Files[0].MD5 != testMD5 || res.Body.Files[0].SHA256 != "" {
		t.Fatalf("md5-only wrong: %+v", res.Body.Files[0])
	}

	in.HashAlgo = "both"
	res, _ = Integrity(ctx, cat, details, in)
	if res.Body.Files[0].MD5 != testMD5 || res.Body.Files[0].SHA256 != testSHA256 {
		t.Fatalf("both wrong: %+v", res.Body.Files[0])
	}

	// compact=true 省 md5（§8）。
	in.Compact = true
	res, _ = Integrity(ctx, cat, details, in)
	if res.Body.Files[0].MD5 != "" || res.Body.Files[0].SHA256 != testSHA256 {
		t.Fatalf("compact must omit md5: %+v", res.Body.Files[0])
	}

	in.HashAlgo = "crc32"
	if _, err := Integrity(ctx, cat, details, in); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid hash_algo must be INVALID_QUERY_PARAM, got %v", err)
	}
}

// include_file_urls：本页 ≤16 且存在 kind=file 产物才附 url；超阈值不附。
func TestIntegrityIncludeFileURLs(t *testing.T) {
	cat, details, lineID := integrityFixture()
	ctx := context.Background()

	details.byLine[lineID].Files = []FileArtifactInfo{
		{Path: "a/one", FileName: "a-one-file", Size: 10, SHA256: testSHA256},
		{Path: "b/two", FileName: "b-two-file", Size: 11, SHA256: testSHA256},
	}

	in := integrityInput()
	in.IncludeFileURLs = true
	res, err := Integrity(ctx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Body.Files) != 5 {
		t.Fatalf("must return all files, got %d", len(res.Body.Files))
	}
	if res.Body.Files[0].URL != "/api/v1/projects//packages/"+testSHA256 {
		t.Fatalf("file url missing: %+v", res.Body.Files[0])
	}
	if res.Body.Files[1].URL != "/api/v1/projects//packages/"+testSHA256 {
		t.Fatalf("file url missing: %+v", res.Body.Files[1])
	}
	if res.Body.Files[2].URL != "" {
		t.Fatalf("unmatched entry must omit url: %+v", res.Body.Files[2])
	}

	entries := make([]model.ManifestEntry, 0, 20)
	var fileArts []FileArtifactInfo
	for i := 0; i < 20; i++ {
		p := fmt.Sprintf("m/%02d", i)
		entries = append(entries, mkManifestEntry(p, 1, model.InstallPolicyOverwrite))
		fileArts = append(fileArts, FileArtifactInfo{Path: p, FileName: "f-" + p, SHA256: testSHA256})
	}
	details.byLine[lineID].Manifest = entries
	details.byLine[lineID].Files = fileArts
	res, _ = Integrity(ctx, cat, details, in)
	for _, f := range res.Body.Files {
		if f.URL != "" {
			t.Fatalf("total >16 must not attach urls: %+v", f)
		}
	}

	details.byLine[lineID].Manifest = entries[:16]
	details.byLine[lineID].Files = fileArts[:16]
	res, _ = Integrity(ctx, cat, details, in)
	if res.Body.Files[0].URL == "" {
		t.Fatalf("total <=16 must attach urls")
	}
}

// 单文件线（§10.2/C09-9）：files 恒一项，即全包哈希行。
func TestIntegritySingleFile(t *testing.T) {
	cat, details, _ := integrityFixture()
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile}
	// 单文件线 RootHash 留空（模型约定）→ ETag 退化为全包 SHA-256。
	cat.Versions[0].Lines[0].RootHash = ""

	res, err := Integrity(context.Background(), cat, details, integrityInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Body.Files) != 1 {
		t.Fatalf("single-file line must have exactly one file entry: %d", len(res.Body.Files))
	}
	f := res.Body.Files[0]
	if f.Path != "demo-1.0.0-windows-x86_64.zip" || f.Size != 1000 ||
		f.SHA256 != "sha-demo-1.0.0-windows-x86_64.zip" || !f.IntegrityCheck {
		t.Fatalf("single-file entry wrong: %+v", f)
	}
	if res.ETag != `"sha-demo-1.0.0-windows-x86_64.zip"` {
		t.Fatalf("single-file etag fallback = %s", res.ETag)
	}
	// 响应不得出现 volumes（无预切分卷）。
	if len(res.Body.Volumes) != 0 {
		t.Fatalf("volumes must be omitted without pre-split volumes")
	}
}

// 未注入 LineDetailSource → 装配错误（check 路径不受影响）。
func TestIntegrityRequiresLineDetails(t *testing.T) {
	cat, _, _ := integrityFixture()
	if _, err := Integrity(context.Background(), cat, nil, integrityInput()); !errors.Is(err, ErrLineDetailsUnavailable) {
		t.Fatalf("expected ErrLineDetailsUnavailable, got %v", err)
	}
}
