package store

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/md4"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// ---------- AppImageUpdate .zsync 单元测试（§9 AppImageUpdate 行 / §9.2，C21-1..C21-6） ----------

// appimageVer 是构建测试 AppImage 目录版本的选项。
type appimageVer struct {
	Semver     string
	VersionInt int64
	Rollout    int
	IsCritical bool
	FileBytes  []byte
	FileName   string
	HwRev      *string
	Ready      bool
}

// newAppImageCatalog 构造含 linux 平台版本的目录与 fake 存储映射。
func newAppImageCatalog(t *testing.T, slug, arch string, vers []appimageVer) (*update.Catalog, map[string][]byte) {
	t.Helper()
	cat := &update.Catalog{
		Project: newTestProject(slug),
		Channels: []update.ChannelInfo{
			{Slug: "stable", StabilityRank: 30, Enabled: true},
			{Slug: "beta", StabilityRank: 20, Enabled: true},
		},
		Matrix:    &update.MatrixInfo{OS: "linux", Arch: arch, PackageType: model.PackageTypeSingleFile},
		EnabledOS: []string{"linux"},
		Versions:  make([]update.VersionState, 0, len(vers)),
	}

	storageData := make(map[string][]byte)

	for _, vInfo := range vers {
		vID := uuid.New()
		lineID := uuid.New()
		v := model.Version{
			ID:             vID,
			ProjectID:      testProjectID,
			ChannelSlug:    "stable",
			Status:         model.VersionStatusPublished,
			GrayStartPercent: 100,
			IsCritical:     vInfo.IsCritical,
			PublishTime:    &testPublishTime,
		}
		applyFeedGray(&v, vInfo.Rollout, vInfo.IsCritical)
		if vInfo.Semver != "" {
			v.VersionSemver = &vInfo.Semver
			v.VersionSemverCanonical = &vInfo.Semver
		}
		if vInfo.VersionInt > 0 {
			v.VersionInteger = &vInfo.VersionInt
		}

		lineStatus := model.VersionLineStatusReady
		if !vInfo.Ready {
			lineStatus = model.VersionLineStatusPending
		}

		line := update.LineState{
			ID:       lineID,
			OS:       "linux",
			Arch:     arch,
			Status:   lineStatus,
			FullPkgs: []update.ArtifactInfo{},
		}
		if lineStatus == model.VersionLineStatusReady {
			line.PacksReadyAt = packsReady()
		}

		if vInfo.Ready {
			fileName := vInfo.FileName
			if fileName == "" {
				fileName = slug + "-" + displayRef(vInfo.Semver, vInfo.VersionInt) + "-" + arch + ".AppImage"
			}
			fileBytes := vInfo.FileBytes
			if fileBytes == nil {
				fileBytes = []byte("default-appimage-bytes-for-" + fileName)
			}
			h256 := sha256.Sum256(fileBytes)
			sha256Hex := hex.EncodeToString(h256[:])
			storageKey := slug + "/" + sha256Hex
			storageData[storageKey] = fileBytes
			line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{
				FileName:    fileName,
				Size:        int64(len(fileBytes)),
				SHA256:      sha256Hex,
				ContentType: "application/x-executable",
				StorageKey:  storageKey,
				HwRev:       vInfo.HwRev,
			})
		}

		cat.Versions = append(cat.Versions, update.VersionState{
			Version: v,
			Lines:   []update.LineState{line},
		})
	}
	return cat, storageData
}

// parsedZSync 是按照 Colin Phipps zsync / zsync2 规范反序列化后的控制文件。
type parsedZSync struct {
	Headers     map[string]string
	BlockSize   int
	Length      int64
	SeqMatches  int
	RSumLen     int
	ChecksumLen int
	URL         string
	SHA1Hex     string
	BlockSums   []byte
}

// parseAndValidateZSync 模拟 zsync.c / zsync_begin 与 zsync_read_blocksums 逻辑对 .zsync 结构进行严格校验。
func parseAndValidateZSync(t *testing.T, zsyncBytes, originalFileBytes []byte) *parsedZSync {
	t.Helper()

	// 1. 头和二进制以 "\n\n" 分隔
	sepIdx := bytes.Index(zsyncBytes, []byte("\n\n"))
	if sepIdx == -1 {
		t.Fatalf(".zsync missing header separator \\n\\n")
	}

	headerText := string(zsyncBytes[:sepIdx])
	blockSums := zsyncBytes[sepIdx+2:]

	headers := make(map[string]string)
	lines := strings.Split(headerText, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			t.Fatalf("invalid header line %q", l)
		}
		headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	// 2. 必填头字段校验
	if ver := headers["zsync"]; ver != "0.6.2" {
		t.Fatalf("header zsync = %q, want 0.6.2", ver)
	}
	if headers["Filename"] == "" {
		t.Fatalf("header Filename is empty")
	}
	bsStr := headers["Blocksize"]
	bs, err := strconv.Atoi(bsStr)
	if err != nil || bs <= 0 || (bs&(bs-1)) != 0 {
		t.Fatalf("header Blocksize = %q invalid power of 2: %v", bsStr, err)
	}

	lenStr := headers["Length"]
	fileLen, err := strconv.ParseInt(lenStr, 10, 64)
	if err != nil || fileLen != int64(len(originalFileBytes)) {
		t.Fatalf("header Length = %q, want %d: %v", lenStr, len(originalFileBytes), err)
	}

	hlStr := headers["Hash-Lengths"]
	hlParts := strings.Split(hlStr, ",")
	if len(hlParts) != 3 {
		t.Fatalf("header Hash-Lengths = %q, want 3 comma-separated tokens", hlStr)
	}
	seq, err := strconv.Atoi(hlParts[0])
	if err != nil || seq < 1 || seq > 2 {
		t.Fatalf("Hash-Lengths seq_matches %q invalid", hlParts[0])
	}
	rsumL, err := strconv.Atoi(hlParts[1])
	if err != nil || rsumL < 2 || rsumL > 4 {
		t.Fatalf("Hash-Lengths rsum_len %q invalid (want 2..4)", hlParts[1])
	}
	checkL, err := strconv.Atoi(hlParts[2])
	if err != nil || checkL < 3 || checkL > 16 {
		t.Fatalf("Hash-Lengths checksum_len %q invalid (want 3..16)", hlParts[2])
	}

	targetURL := headers["URL"]
	if targetURL == "" {
		t.Fatalf("header URL is empty")
	}

	sha1H := headers["SHA-1"]
	expectedSHA1 := sha1.Sum(originalFileBytes)
	expectedSHA1Hex := hex.EncodeToString(expectedSHA1[:])
	if strings.ToLower(sha1H) != strings.ToLower(expectedSHA1Hex) {
		t.Fatalf("header SHA-1 = %q, want %q", sha1H, expectedSHA1Hex)
	}

	// 3. 块校验和数据大小与每块哈希验证
	numBlocks := int((fileLen + int64(bs) - 1) / int64(bs))
	perBlockBytes := rsumL + checkL
	expectedBlockSumsLen := numBlocks * perBlockBytes
	if len(blockSums) != expectedBlockSumsLen {
		t.Fatalf("blockSums len = %d, want %d (blocks=%d, perBlock=%d)", len(blockSums), expectedBlockSumsLen, numBlocks, perBlockBytes)
	}

	blockBuf := make([]byte, bs)
	for b := 0; b < numBlocks; b++ {
		start := int64(b * bs)
		end := start + int64(bs)
		if end > fileLen {
			end = fileLen
		}
		clear(blockBuf)
		copy(blockBuf, originalFileBytes[start:end])

		// 重新计算滚动校验和
		a, bSum := calcRSumBlock(blockBuf)
		rBuf := [4]byte{byte(a >> 8), byte(a), byte(bSum >> 8), byte(bSum)}
		expectedRSum := rBuf[4-rsumL : 4]

		offset := b * perBlockBytes
		actualRSum := blockSums[offset : offset+rsumL]
		if !bytes.Equal(actualRSum, expectedRSum) {
			t.Fatalf("block %d rsum mismatch: got %x, want %x", b, actualRSum, expectedRSum)
		}

		// 重新计算 MD4 校验和
		m := md4.New()
		m.Write(blockBuf)
		expectedMD4 := m.Sum(nil)[:checkL]
		actualMD4 := blockSums[offset+rsumL : offset+perBlockBytes]
		if !bytes.Equal(actualMD4, expectedMD4) {
			t.Fatalf("block %d md4 mismatch: got %x, want %x", b, actualMD4, expectedMD4)
		}
	}

	return &parsedZSync{
		Headers:     headers,
		BlockSize:   bs,
		Length:      fileLen,
		SeqMatches:  seq,
		RSumLen:     rsumL,
		ChecksumLen: checkL,
		URL:         targetURL,
		SHA1Hex:     sha1H,
		BlockSums:   blockSums,
	}
}

// TestAppImage_ZSyncFormatAndParserVerification 验收项 1：小 fixture AppImage 的 zsync 可被标准 zsync parser 识别并验证。
func TestAppImage_ZSyncFormatAndParserVerification(t *testing.T) {
	// 构建跨多块的小 fixture 数据（5000 字节，跨 3 个 2048 块，含最后一个短块）
	fileContent := make([]byte, 5000)
	for i := range fileContent {
		fileContent[i] = byte((i*31 + 17) % 256)
	}

	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: fileContent},
	})
	storage := newFakeStorage(storageData)

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "aidemo-1.0.0-x86_64.AppImage.zsync",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if resp.ContentType != appimageContentType {
		t.Fatalf("content-type = %q, want %q", resp.ContentType, appimageContentType)
	}

	parsed := parseAndValidateZSync(t, resp.Body, fileContent)
	if parsed.BlockSize != 2048 {
		t.Fatalf("blocksize = %d, want 2048", parsed.BlockSize)
	}
	if parsed.Length != 5000 {
		t.Fatalf("length = %d, want 5000", parsed.Length)
	}
	if parsed.Headers["Filename"] != "aidemo-1.0.0-x86_64.AppImage" {
		t.Fatalf("filename = %q", parsed.Headers["Filename"])
	}
}

// TestAppImage_ZSyncCalculationFormulas 验证 Blocksize / Hash-Lengths 计算在不同文件尺寸下的参数与边界值。
func TestAppImage_ZSyncCalculationFormulas(t *testing.T) {
	tests := []struct {
		name       string
		fileLen    int64
		wantBS     int
		wantSeq    int
		wantRSumL  int
		wantCheckL int
	}{
		{"empty file", 0, 2048, 1, 2, 3},
		{"small 500B", 500, 2048, 1, 2, 4},
		{"5000B", 5000, 2048, 2, 2, 3},
		{"1MB", 1024 * 1024, 2048, 2, 2, 4},
		{"50MB", 50 * 1024 * 1024, 2048, 2, 2, 5},
		{"150MB", 150 * 1024 * 1024, 4096, 2, 2, 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bs, seq, rsum, check := calcZSyncParams(tc.fileLen)
			if bs != tc.wantBS {
				t.Errorf("bs = %d, want %d", bs, tc.wantBS)
			}
			if seq != tc.wantSeq {
				t.Errorf("seq = %d, want %d", seq, tc.wantSeq)
			}
			if rsum != tc.wantRSumL {
				t.Errorf("rsum = %d, want %d", rsum, tc.wantRSumL)
			}
			if check != tc.wantCheckL {
				t.Errorf("check = %d, want %d", check, tc.wantCheckL)
			}
		})
	}
}

// TestAppImage_StorageUnavailableReturnsError 验收项 2：存储不可用或失败时绝不冒充 200 空文件，必须报错。
func TestAppImage_StorageUnavailableReturnsError(t *testing.T) {
	fileContent := []byte("appimage-bytes")
	cat, _ := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: fileContent},
	})
	// 存储不含该 key
	emptyStorage := newFakeStorage(map[string][]byte{})

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: emptyStorage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "aidemo-1.0.0-x86_64.AppImage.zsync",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err == nil {
		t.Fatalf("expected error when storage fails, got resp: %+v", resp)
	}
	if resp != nil {
		t.Fatalf("expected nil resp on error, got: %+v", resp)
	}

	// 当 Storage 完全为 nil 时同样必须报错
	depsNilStorage := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: nil}
	resp2, err2 := adapter.Render(context.Background(), depsNilStorage, req)
	if err2 == nil || resp2 != nil {
		t.Fatalf("expected error when storage is nil, got: %v, resp: %+v", err2, resp2)
	}
}

// TestAppImage_CacheSingleFlightPreventsDuplicateStorageReads 验证单飞 + LRU 缓存：并发与重复请求不重复读取存储。
func TestAppImage_CacheSingleFlightPreventsDuplicateStorageReads(t *testing.T) {
	fileContent := []byte("appimage-payload-for-caching-test")
	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: fileContent},
	})
	storage := newFakeStorage(storageData)

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "latest.zsync",
		Arch:    "x86_64",
	}

	resp1, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render 1: %v", err)
	}
	key := cat.Versions[0].Lines[0].FullPkgs[0].StorageKey
	if storage.readCount(key) != 1 {
		t.Fatalf("storage read count = %d, want 1", storage.readCount(key))
	}

	resp2, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render 2: %v", err)
	}
	// 命中缓存，存储读取计数仍为 1
	if storage.readCount(key) != 1 {
		t.Fatalf("storage read count after second render = %d, want 1", storage.readCount(key))
	}

	if !bytes.Equal(resp1.Body, resp2.Body) {
		t.Fatalf("body mismatch across renders")
	}
}

// TestAppImage_GrayRolloutAndCritical 验证灰度与关键版本过滤。
func TestAppImage_GrayRolloutAndCritical(t *testing.T) {
	// 2.0.0 灰度 50% 且非关键版本不可见；1.1.0 灰度 20% 但关键版本可见；1.0.0 100% 可见
	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0")},
		{Semver: "1.1.0", Rollout: 20, IsCritical: true, Ready: true, FileBytes: []byte("v1.1.0")},
		{Semver: "2.0.0", Rollout: 50, IsCritical: false, Ready: true, FileBytes: []byte("v2.0.0")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "latest.zsync",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// 最新可见应为关键版本 1.1.0
	parsed := parseAndValidateZSync(t, resp.Body, []byte("v1.1.0"))
	if parsed.Headers["Filename"] != "aidemo-1.1.0-x86_64.AppImage" {
		t.Fatalf("projected filename = %q, want aidemo-1.1.0-x86_64.AppImage", parsed.Headers["Filename"])
	}
}

// TestAppImage_DefaultHwVariantOnly 验证仅投影默认硬件变体（C21-6）。
func TestAppImage_DefaultHwVariantOnly(t *testing.T) {
	hwRevA := "rev-a"
	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0-default")},
		{Semver: "2.0.0", Rollout: 100, Ready: true, HwRev: &hwRevA, FileBytes: []byte("v2.0.0-hw")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "latest.zsync",
		Arch:    "x86_64",
		HWRev:   "rev-a", // 传入 hw_rev 也不得选非默认变体
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// 2.0.0 无默认变体，可见最新为 1.0.0
	parsed := parseAndValidateZSync(t, resp.Body, []byte("v1.0.0-default"))
	if parsed.Headers["Filename"] != "aidemo-1.0.0-x86_64.AppImage" {
		t.Fatalf("projected filename = %q, want aidemo-1.0.0-x86_64.AppImage", parsed.Headers["Filename"])
	}
}

// TestAppImage_EmptyCatalogReturnsErrNoRelease 验证可见集为空时报 ErrNoRelease（404）。
func TestAppImage_EmptyCatalogReturnsErrNoRelease(t *testing.T) {
	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 50, IsCritical: false, Ready: true}, // 灰度不可见
	})
	storage := newFakeStorage(storageData)

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "latest.zsync",
		Arch:    "x86_64",
	}

	_, err := adapter.Render(context.Background(), deps, req)
	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}

// TestAppImage_UnknownPathReturnsErrUnknownPath 验证非 .zsync 结尾的路径报 ErrUnknownPath（404）。
func TestAppImage_UnknownPathReturnsErrUnknownPath(t *testing.T) {
	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true},
	})
	storage := newFakeStorage(storageData)

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}

	badPaths := []string{
		"appimage",
		"MyApp.exe",
		"RELEASES",
		"latest.json",
		"manifest.xml",
		"foo/bar",
	}

	for _, p := range badPaths {
		req := Request{
			Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
			Channel: "stable",
			Path:    p,
			Arch:    "x86_64",
		}
		_, err := adapter.Render(context.Background(), deps, req)
		if !errors.Is(err, ErrUnknownPath) {
			t.Fatalf("path %q err = %v, want ErrUnknownPath", p, err)
		}
	}
}

// TestAppImage_UnknownChannelReturnsErrUnknownChannel 验证未知渠道报 ErrUnknownChannel（400）。
func TestAppImage_UnknownChannelReturnsErrUnknownChannel(t *testing.T) {
	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true},
	})
	storage := newFakeStorage(storageData)

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "unknown-channel",
		Path:    "latest.zsync",
		Arch:    "x86_64",
	}

	_, err := adapter.Render(context.Background(), deps, req)
	if !errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("err = %v, want ErrUnknownChannel", err)
	}
}

// TestAppImage_DefaultArchX86_64 验证请求未传 arch query 时默认采用 x86_64。
func TestAppImage_DefaultArchX86_64(t *testing.T) {
	fileContent := []byte("appimage-x86_64-content")
	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: fileContent},
	})
	storage := newFakeStorage(storageData)

	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "latest.zsync",
		Arch:    "", // 省略 arch
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render with empty arch: %v", err)
	}
	parsed := parseAndValidateZSync(t, resp.Body, fileContent)
	if parsed.Headers["Filename"] != "aidemo-1.0.0-x86_64.AppImage" {
		t.Fatalf("filename = %q", parsed.Headers["Filename"])
	}
}

// TestAppImage_PrivateProjectSignedURL 验证私有存储项目 URL 附加签名参数。
func TestAppImage_PrivateProjectSignedURL(t *testing.T) {
	fileContent := []byte("private-appimage-bytes")
	cat, storageData := newAppImageCatalog(t, "aidemo", "x86_64", []appimageVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: fileContent},
	})
	cat.Project.StorageVisibility = model.StorageVisibilityPrivate
	storage := newFakeStorage(storageData)

	signer := stubSigner{}
	adapter := NewAppImageAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Signer: signer, Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:                cat.Project.ID,
			Slug:              cat.Project.Slug,
			StorageVisibility: model.StorageVisibilityPrivate,
		},
		Channel: "stable",
		Path:    "latest.zsync",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	parsed := parseAndValidateZSync(t, resp.Body, fileContent)
	if !strings.Contains(parsed.URL, "?exp=123&sig=abc") {
		t.Fatalf("URL = %q, want signed query ?exp=123&sig=abc", parsed.URL)
	}
}

// TestAppImage_EnabledFlag 验证协议开关控制（C21-3）。
func TestAppImage_EnabledFlag(t *testing.T) {
	adapter := NewAppImageAdapter()
	if adapter.Enabled(nil) {
		t.Fatal("nil project must not be enabled")
	}
	if !adapter.Enabled(&model.Project{}) {
		t.Fatal("non-nil project must be enabled (listing is the HTTP gate)")
	}
}
