package store

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/md4"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
)

// AppImageUpdate .zsync 旁路清单投影（docs/app-init.md §9 AppImageUpdate 行 / §9.2，C21-1..C21-6）。
//
// 产出 AppImageUpdate 可直接消费的标准 .zsync 控制文件：
//   - 端点：GET /store/appimage/default/{path...}?channel=&arch=；
//   - 路径限制：path 必须以 .zsync 结尾（如 MyApp-x86_64.AppImage.zsync 或 latest.zsync），
//     其它路径报 ErrUnknownPath（HTTP 纯文本 404，绝不冒充或回退 zip）；
//   - os 恒为 linux（AppImage 仅运行于 Linux 平台）；arch 缺省为 x86_64；
//   - hw_rev：AppImage 协议无法表达硬件代号，恒只投影默认硬件变体（C21-6）；
//   - 可见性：按 §4.4 匿名口径（AnonymousVisible），灰度未 100% 且非关键版本永不出现；
//   - 生成与防空防坏（C21-1, C21-4）：首请求从存储流式读取 AppImage 字节并计算
//     Blocksize / Hash-Lengths / 全量 SHA-1 与各块 Rolling Checksum (Rsync) + MD4 强校验和；
//     采用 ZSyncDataCache（SingleFlight + LRU 缓存，按产物 SHA-256 缓存不变块校验和数据）；
//     存储失败时返回 error（HTTP 500），绝不返回损坏或 0 字节的 200 空文件；
//   - 稳定直链与私有签名：公开项目输出稳定全量包下载 URL，私有项目通过 deps.Signer 短时签名 URL，
//     HTTP 层对公开项目支持强 ETag 304，私有项目输出 private, no-store。
const (
	// ProtocolAppImage 是 AppImage 协议名（Project.StoreProtocols 的 jsonb 键）。
	ProtocolAppImage = "appimage"

	// appimageContentType 是 .zsync 文件的标准 MIME。
	appimageContentType = "application/x-zsync"

	// appimageDefaultOS 是 AppImage 固定的操作系统。
	appimageDefaultOS = "linux"

	// appimageDefaultArch 是未提供 arch query 时的默认架构。
	appimageDefaultArch = "x86_64"

	// zsyncVersion 是 .zsync 控制文件规范版本。
	zsyncVersion = "0.6.2"
)

// zsyncDataEntry 缓存单个产物计算所得的不可变 zsync 块校验和数据（键为产物 SHA-256）。
type zsyncDataEntry struct {
	fileLen     int64
	blockSize   int
	seqMatches  int
	rsumLen     int
	checksumLen int
	sha1Hex     string
	blockSums   []byte
}

type zsyncCall struct {
	done chan struct{}
	data *zsyncDataEntry
	err  error
}

// ZSyncDataCache 是并发安全、支持单飞（SingleFlight）与 LRU 逐出的 zsync 块数据缓存。
type ZSyncDataCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[string]*list.Element
	order    *list.List
	inflight map[string]*zsyncCall
}

// NewZSyncDataCache 构造 ZSyncDataCache 实例；capacity <= 0 时使用默认容量（128）。
func NewZSyncDataCache(capacity int) *ZSyncDataCache {
	if capacity <= 0 {
		capacity = DefaultSignatureCacheEntries
	}
	return &ZSyncDataCache{
		capacity: capacity,
		entries:  make(map[string]*list.Element, capacity),
		order:    list.New(),
		inflight: make(map[string]*zsyncCall),
	}
}

// GetOrCompute 查询缓存或单飞计算块校验和数据。
func (c *ZSyncDataCache) GetOrCompute(key string, compute func() (*zsyncDataEntry, error)) (*zsyncDataEntry, error) {
	c.mu.Lock()
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)
		data := el.Value.(*zsyncCacheItem).data
		c.mu.Unlock()
		return data, nil
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.data, call.err
	}
	call := &zsyncCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	data, err := compute()

	c.mu.Lock()
	delete(c.inflight, key)
	if err == nil && data != nil {
		c.putLocked(key, data)
	}
	c.mu.Unlock()

	call.data, call.err = data, err
	close(call.done)
	return data, err
}

type zsyncCacheItem struct {
	key  string
	data *zsyncDataEntry
}

func (c *ZSyncDataCache) putLocked(key string, data *zsyncDataEntry) {
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)
		el.Value.(*zsyncCacheItem).data = data
		return
	}
	c.entries[key] = c.order.PushFront(&zsyncCacheItem{key: key, data: data})
	for c.order.Len() > c.capacity {
		back := c.order.Back()
		if back == nil {
			return
		}
		c.order.Remove(back)
		delete(c.entries, back.Value.(*zsyncCacheItem).key)
	}
}

// AppImageAdapter 是 AppImageUpdate .zsync 协议的 Adapter 实现。
type AppImageAdapter struct {
	cache *ZSyncDataCache
}

// compile-time 校验：满足 Adapter 合同。
var _ Adapter = (*AppImageAdapter)(nil)

// NewAppImageAdapter 构造 AppImageAdapter。
func NewAppImageAdapter() *AppImageAdapter {
	return &AppImageAdapter{cache: NewZSyncDataCache(0)}
}

// NewAppImageAdapterWithCache 使用指定缓存构造 AppImageAdapter（供测试注入）。
func NewAppImageAdapterWithCache(cache *ZSyncDataCache) *AppImageAdapter {
	return &AppImageAdapter{cache: cache}
}

// Protocol 实现 Adapter：返回 "appimage"。
func (a *AppImageAdapter) Protocol() string { return ProtocolAppImage }

// Enabled 实现 Adapter：StoreProtocols["appimage"].Enabled（C21-3）。
func (a *AppImageAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolAppImage)
}

// Render 实现 Adapter：校验路径 → 加载 linux 目录 → 匿名可见集（选最新版本）→ 获取/计算 zsync 控制文件。
func (a *AppImageAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, fmt.Errorf("feed: update service unavailable")
	}
	if req.Project == nil {
		return nil, fmt.Errorf("%w: project is required", ErrMissingParam)
	}

	cleanPath := strings.Trim(req.Path, "/")
	if !strings.HasSuffix(strings.ToLower(cleanPath), ".zsync") {
		return nil, fmt.Errorf("%w: %q (path must end with .zsync)", ErrUnknownPath, req.Path)
	}

	arch := req.Arch
	if arch == "" {
		arch = appimageDefaultArch
	}
	arch = platform.CanonicalArch(arch)

	cat, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, appimageDefaultOS, arch)
	if err != nil {
		return nil, err
	}

	chName := req.Channel
	if chName == "" {
		chName = "stable"
	}
	if cat.Channels != nil {
		if _, found := cat.Channel(chName); !found {
			return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, chName)
		}
	}

	items := AnonymousVisible(cat, cat.Project.CompareEngine, chName, appimageDefaultOS, arch)
	if len(items) == 0 {
		return nil, ErrNoRelease
	}

	chosen := &items[0]
	v := &chosen.Version.Version
	pkg, ok := resolveStorePackage(ctx, deps, req, chosen.Line, chosen.Package)
	if !ok || pkg == nil {
		return nil, ErrNoRelease
	}

	signing := urlSigningFromDeps(cat, deps)
	downloadURL := enclosureURL(signing, cat.Project.Slug, pkg.SHA256, pkg.StorageKey)

	dataEntry, err := a.cache.GetOrCompute(pkg.SHA256, func() (*zsyncDataEntry, error) {
		if deps.Storage == nil {
			return nil, fmt.Errorf("feed: storage backend unavailable for zsync generation")
		}
		rc, err := deps.Storage.Get(ctx, pkg.StorageKey)
		if err != nil {
			return nil, fmt.Errorf("feed: read artifact storage: %w", err)
		}
		defer rc.Close()

		rawBytes, err := io.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("feed: read artifact body: %w", err)
		}
		if len(rawBytes) == 0 {
			return nil, errors.New("feed: artifact in storage is empty")
		}

		return buildZSyncData(rawBytes)
	})
	if err != nil {
		return nil, err
	}

	pubTime := v.CreatedAt
	if v.PublishTime != nil {
		pubTime = *v.PublishTime
	}
	body := renderZSyncManifest(pkg.FileName, pubTime, downloadURL, dataEntry)
	return &Response{
		Body:        body,
		ContentType: appimageContentType,
		Status:      http.StatusOK,
	}, nil
}

// buildZSyncData 根据全量产物字节计算不可变的 zsync 摘要与每块校验和数据。
func buildZSyncData(fileBytes []byte) (*zsyncDataEntry, error) {
	fileLen := int64(len(fileBytes))
	blockSize, seqMatches, rsumLen, checksumLen := calcZSyncParams(fileLen)

	sha1Sum := sha1.Sum(fileBytes)
	sha1Hex := hex.EncodeToString(sha1Sum[:])

	numBlocks := int((fileLen + int64(blockSize) - 1) / int64(blockSize))
	blockSums := make([]byte, 0, numBlocks*(rsumLen+checksumLen))
	blockBuf := make([]byte, blockSize)

	for i := int64(0); i < fileLen; i += int64(blockSize) {
		end := i + int64(blockSize)
		if end > fileLen {
			end = fileLen
		}
		clear(blockBuf)
		copy(blockBuf, fileBytes[i:end])

		// 滚动校验和（Rsync rolling checksum）
		a, b := calcRSumBlock(blockBuf)
		rBuf := [4]byte{
			byte(a >> 8),
			byte(a),
			byte(b >> 8),
			byte(b),
		}
		// 取末尾 rsumLen 字节（大端对齐）
		blockSums = append(blockSums, rBuf[4-rsumLen:4]...)

		// 强校验和（MD4）
		m := md4.New()
		m.Write(blockBuf)
		c := m.Sum(nil)
		blockSums = append(blockSums, c[:checksumLen]...)
	}

	return &zsyncDataEntry{
		fileLen:     fileLen,
		blockSize:   blockSize,
		seqMatches:  seqMatches,
		rsumLen:     rsumLen,
		checksumLen: checksumLen,
		sha1Hex:     sha1Hex,
		blockSums:   blockSums,
	}, nil
}

// calcZSyncParams 遵照 Colin Phipps zsync make.c 与 AppImage zsync2 标准计算块大小与校验和截断长度。
func calcZSyncParams(fileLen int64) (blockSize, seqMatches, rsumLen, checksumLen int) {
	if fileLen < 100_000_000 {
		blockSize = 2048
	} else {
		blockSize = 4096
	}

	if fileLen <= 0 {
		return blockSize, 1, 2, 3
	}

	if fileLen > int64(blockSize) {
		seqMatches = 2
	} else {
		seqMatches = 1
	}

	flen := float64(fileLen)
	fbs := float64(blockSize)

	// rsum_len = ceil(((log(len) + log(blocksize)) / log(2) - 8.6) / seq_matches / 8)
	rsumVal := math.Ceil(((math.Log2(flen) + math.Log2(fbs)) - 8.6) / float64(seqMatches) / 8.0)
	rsumLen = int(rsumVal)
	if rsumLen > 4 {
		rsumLen = 4
	}
	if rsumLen < 2 {
		rsumLen = 2
	}

	// checksum_len = ceil((20 + (log(len) + log(1 + len / blocksize)) / log(2)) / seq_matches / 8)
	c1Val := math.Ceil((20.0 + (math.Log2(flen) + math.Log2(1.0+flen/fbs))) / float64(seqMatches) / 8.0)
	c1 := int(c1Val)

	// checksum_len2 = (7.9 + (20 + log(1 + len / blocksize) / log(2))) / 8
	c2 := int((7.9 + (20.0 + math.Log2(1.0+flen/fbs))) / 8.0)

	checksumLen = c1
	if checksumLen < c2 {
		checksumLen = c2
	}
	if checksumLen < 3 {
		checksumLen = 3
	}
	if checksumLen > 16 {
		checksumLen = 16
	}

	return blockSize, seqMatches, rsumLen, checksumLen
}

// calcRSumBlock 计算给定块（已填充对齐）的 Rsync 滚动校验和（a 和 b）。
func calcRSumBlock(data []byte) (uint16, uint16) {
	var a, b uint16
	n := len(data)
	for i, c := range data {
		uc := uint16(c)
		a += uc
		b += uint16(n-i) * uc
	}
	return a, b
}

// renderZSyncManifest 拼接 .zsync ASCII 文本头与二进制校验和数据。
func renderZSyncManifest(filename string, mtime time.Time, downloadURL string, entry *zsyncDataEntry) []byte {
	var buf bytes.Buffer

	buf.WriteString("zsync: " + zsyncVersion + "\n")
	if filename != "" {
		fmt.Fprintf(&buf, "Filename: %s\n", filename)
	}
	if !mtime.IsZero() {
		fmt.Fprintf(&buf, "MTime: %s\n", mtime.UTC().Format(time.RFC1123Z))
	}
	fmt.Fprintf(&buf, "Blocksize: %d\n", entry.blockSize)
	fmt.Fprintf(&buf, "Length: %d\n", entry.fileLen)
	fmt.Fprintf(&buf, "Hash-Lengths: %d,%d,%d\n", entry.seqMatches, entry.rsumLen, entry.checksumLen)
	fmt.Fprintf(&buf, "URL: %s\n", downloadURL)
	fmt.Fprintf(&buf, "SHA-1: %s\n", entry.sha1Hex)
	buf.WriteString("\n")

	buf.Write(entry.blockSums)
	return buf.Bytes()
}
