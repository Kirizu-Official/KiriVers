package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/oschwald/geoip2-golang"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

var (
	// ErrGeoipNotFound 库不存在 → HTTP 404。
	ErrGeoipNotFound = errors.New("geoip database not found")
	// ErrGeoipLimit 超过 8 份或单文件过大 → HTTP 400。
	ErrGeoipLimit = errors.New("geoip database limit exceeded")
	// ErrInvalidGeoip 非 MMDB / 坏文件名 → HTTP 400。
	ErrInvalidGeoip = errors.New("invalid geoip database")
	// ErrGeoipStorage 对象存储不可用。
	ErrGeoipStorage = errors.New("geoip storage unavailable")
)

const geoipReplicaPoll = 30 * time.Second

// GeoipResult 是融合查询结果（ISO 代码 + 多语言地名图）。
type GeoipResult struct {
	CountryCode string
	RegionCode  string
	GeoI18n     model.GeoI18n
}

// GeoipLookupFunc 供名册 upsert 注入；单测可 stub，不打开真实 MMDB。
type GeoipLookupFunc func(ctx context.Context, ip string) GeoipResult

type geoReader interface {
	City(ip net.IP) (*geoip2.City, error)
	Country(ip net.IP) (*geoip2.Country, error)
	Close() error
}

type geoSnapshot struct {
	readers []liveReader
}

type liveReader struct {
	meta   model.GeoipDatabase
	size   int64
	reader geoReader
}

type geoRecord struct {
	CountryCode  string
	RegionCode   string
	CountryNames map[string]string
	RegionNames  map[string]string
}

// GeoipService 管理平台 MMDB 与融合查询。
type GeoipService struct {
	store     repository.GeoipStore
	storage   storage.Backend
	localRoot string
	openBytes func([]byte) (geoReader, error)
	mu        sync.RWMutex
	snap      *geoSnapshot
}

// NewGeoipService 构造服务。storage 为私有 Backend（nil 时上传失败）。localRoot 为本机 temp。
func NewGeoipService(store repository.GeoipStore, backend storage.Backend, localRoot string) *GeoipService {
	if strings.TrimSpace(localRoot) == "" {
		localRoot = filepath.Join(os.TempDir(), "kirivers-geoip")
	}
	return &GeoipService{
		store:     store,
		storage:   backend,
		localRoot: localRoot,
		openBytes: func(data []byte) (geoReader, error) {
			return geoip2.FromBytes(data)
		},
	}
}

// SetOpener 注入 MMDB 打开函数（单测 stub；忽略 path，按字节打开）。
func (s *GeoipService) SetOpener(fn func(path string) (geoReader, error)) {
	if s == nil || fn == nil {
		return
	}
	s.openBytes = func([]byte) (geoReader, error) {
		return fn("")
	}
}

// SetBytesOpener 注入按字节打开 MMDB（单测 stub）。
func (s *GeoipService) SetBytesOpener(fn func([]byte) (geoReader, error)) {
	if s == nil || fn == nil {
		return
	}
	s.openBytes = fn
}

// SetNopOpener 让上传校验/Reload 不打开真实 MMDB（HTTP 单测）。
func (s *GeoipService) SetNopOpener() {
	if s == nil {
		return
	}
	s.openBytes = func([]byte) (geoReader, error) {
		return nopGeoReader{}, nil
	}
}

type nopGeoReader struct{}

func (nopGeoReader) City(net.IP) (*geoip2.City, error)       { return nil, errNopGeo }
func (nopGeoReader) Country(net.IP) (*geoip2.Country, error) { return nil, errNopGeo }
func (nopGeoReader) Close() error                            { return nil }

var errNopGeo = errors.New("nop geo reader")


// LookupFn 适配 ProjectService.SetGeoipLookup。
func (s *GeoipService) LookupFn() GeoipLookupFunc {
	if s == nil {
		return nil
	}
	return s.Lookup
}

// StartPoll 每 30s 从 DB 刷新 reader 快照（多副本）。ctx 取消后退出。
func (s *GeoipService) StartPoll(ctx context.Context) {
	if s == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(geoipReplicaPoll)
		defer ticker.Stop()
		_ = s.Reload(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.Reload(ctx)
			}
		}
	}()
}

// List 返回全部库（含停用），按 rank 升序。
func (s *GeoipService) List(ctx context.Context) ([]model.GeoipDatabase, error) {
	if s.store == nil {
		return nil, ErrGeoipStorage
	}
	return s.store.List(ctx)
}

// GeoipWrite 是 PATCH 输入。
type GeoipWrite struct {
	Name    *string
	Enabled *bool
	Rank    *int
}

// Upload 写入对象存储并登记元数据。file 须为 .mmdb，≤256MiB，总数 ≤8。
func (s *GeoipService) Upload(ctx context.Context, name, fileName string, body io.Reader, size int64) (*model.GeoipDatabase, error) {
	if s.store == nil || s.storage == nil {
		return nil, ErrGeoipStorage
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 128 {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidGeoip)
	}
	safe, err := safeMMDBFileName(fileName)
	if err != nil {
		return nil, err
	}
	if size < 0 {
		size = 0
	}
	if size > model.GeoipMaxFileBytes {
		return nil, fmt.Errorf("%w: file exceeds 256MiB", ErrGeoipLimit)
	}
	n, err := s.store.Count(ctx)
	if err != nil {
		return nil, err
	}
	if n >= model.GeoipMaxDatabases {
		return nil, fmt.Errorf("%w: at most 8 databases", ErrGeoipLimit)
	}
	rank, err := s.store.MaxRank(ctx)
	if err != nil {
		return nil, err
	}
	row := &model.GeoipDatabase{
		Name:     name,
		FileName: safe,
		Enabled:  true,
		Rank:     rank + 1,
		Size:     size,
	}
	if err := row.BeforeCreate(nil); err != nil {
		return nil, err
	}
	row.StorageKey = storage.GeoipObjectKey(row.ID, safe)
	tempRel := storage.GeoipTempRel(row.ID)
	tempAbs := filepath.Join(s.localRoot, filepath.FromSlash(tempRel))
	if err := os.MkdirAll(filepath.Dir(tempAbs), 0o700); err != nil {
		return nil, err
	}
	limited := io.LimitReader(body, model.GeoipMaxFileBytes+1)
	tf, err := os.OpenFile(tempAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	nwrite, copyErr := io.Copy(tf, limited)
	_ = tf.Close()
	if copyErr != nil {
		_ = os.Remove(tempAbs)
		return nil, copyErr
	}
	if nwrite > model.GeoipMaxFileBytes {
		_ = os.Remove(tempAbs)
		return nil, fmt.Errorf("%w: file exceeds 256MiB", ErrGeoipLimit)
	}
	raw, err := os.ReadFile(tempAbs)
	if err != nil {
		_ = os.Remove(tempAbs)
		return nil, err
	}
	rdr, err := s.openBytes(raw)
	if err != nil {
		_ = os.Remove(tempAbs)
		return nil, fmt.Errorf("%w: not a valid mmdb", ErrInvalidGeoip)
	}
	_ = rdr.Close()
	row.Size = int64(len(raw))
	if err := s.storage.Put(ctx, row.StorageKey, bytes.NewReader(raw), row.Size, "application/octet-stream"); err != nil {
		_ = os.Remove(tempAbs)
		return nil, err
	}
	_ = os.Remove(tempAbs)
	now := time.Now().UTC()
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := s.store.Create(ctx, row); err != nil {
		_ = s.storage.Delete(ctx, row.StorageKey)
		return nil, err
	}
	_ = s.Reload(ctx)
	return row, nil
}

// Patch 改名/启停/调序。
func (s *GeoipService) Patch(ctx context.Context, id uuid.UUID, in GeoipWrite) (*model.GeoipDatabase, error) {
	if s.store == nil {
		return nil, ErrGeoipStorage
	}
	row, err := s.store.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGeoipNotFound
	}
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || utf8.RuneCountInString(name) > 128 {
			return nil, fmt.Errorf("%w: name is required", ErrInvalidGeoip)
		}
		row.Name = name
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if in.Rank != nil {
		row.Rank = *in.Rank
	}
	row.UpdatedAt = time.Now().UTC()
	if err := s.store.Save(ctx, row); err != nil {
		return nil, err
	}
	_ = s.Reload(ctx)
	return row, nil
}

// Delete 删除对象与行并重载。
func (s *GeoipService) Delete(ctx context.Context, id uuid.UUID) error {
	if s.store == nil {
		return ErrGeoipStorage
	}
	row, err := s.store.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrGeoipNotFound
	}
	if err != nil {
		return err
	}
	if s.storage != nil && row.StorageKey != "" {
		_ = s.storage.Delete(ctx, row.StorageKey)
	}
	if err := s.store.Delete(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGeoipNotFound
		}
		return err
	}
	_ = s.Reload(ctx)
	return nil
}

// Lookup 对公网 IP 融合查询。私网/无效/无启用库返回空结果，永不 500。
func (s *GeoipService) Lookup(_ context.Context, ip string) GeoipResult {
	empty := GeoipResult{GeoI18n: model.GeoI18n{Country: map[string]string{}, Region: map[string]string{}}}
	parsed := parsePublicIP(ip)
	if parsed == nil || s == nil {
		return empty
	}
	s.mu.RLock()
	snap := s.snap
	s.mu.RUnlock()
	if snap == nil || len(snap.readers) == 0 {
		return empty
	}
	parts := make([]geoRecord, 0, len(snap.readers))
	for _, live := range snap.readers {
		rec, ok := readGeo(live.reader, parsed)
		if !ok {
			continue
		}
		parts = append(parts, rec)
	}
	return fuseGeoRecords(parts)
}

// Reload 从私有存储把启用库拉入内存。Head 尺寸未变则跳过 Get。
func (s *GeoipService) Reload(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}
	list, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	s.mu.RLock()
	oldByKey := map[string]liveReader{}
	if s.snap != nil {
		for _, r := range s.snap.readers {
			oldByKey[r.meta.StorageKey] = r
		}
	}
	s.mu.RUnlock()

	next := &geoSnapshot{}
	kept := map[string]struct{}{}
	for i := range list {
		row := list[i]
		if !row.Enabled {
			continue
		}
		if s.storage == nil {
			continue
		}
		sz, _, herr := s.storage.Head(ctx, row.StorageKey)
		if herr == nil {
			if prev, ok := oldByKey[row.StorageKey]; ok && prev.size == sz {
				next.readers = append(next.readers, prev)
				kept[row.StorageKey] = struct{}{}
				continue
			}
		}
		data, err := s.fetchBytes(ctx, &row)
		if err != nil {
			continue
		}
		rdr, err := s.openBytes(data)
		if err != nil {
			continue
		}
		next.readers = append(next.readers, liveReader{meta: row, size: int64(len(data)), reader: rdr})
	}
	s.mu.Lock()
	old := s.snap
	s.snap = next
	s.mu.Unlock()
	if old != nil {
		for _, r := range old.readers {
			if _, ok := kept[r.meta.StorageKey]; ok {
				continue
			}
			_ = r.reader.Close()
		}
	}
	return nil
}

func (s *GeoipService) fetchBytes(ctx context.Context, row *model.GeoipDatabase) ([]byte, error) {
	if s.storage == nil {
		return nil, ErrGeoipStorage
	}
	rc, err := s.storage.Get(ctx, row.StorageKey)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, model.GeoipMaxFileBytes+1))
}

func safeMMDBFileName(raw string) (string, error) {
	base := filepath.Base(strings.TrimSpace(raw))
	base = strings.ReplaceAll(base, "..", "")
	if base == "" || base == "." || strings.ContainsAny(base, `/\`) {
		return "", fmt.Errorf("%w: invalid file name", ErrInvalidGeoip)
	}
	if !strings.HasSuffix(strings.ToLower(base), ".mmdb") {
		return "", fmt.Errorf("%w: file must be .mmdb", ErrInvalidGeoip)
	}
	return base, nil
}

func parsePublicIP(raw string) net.IP {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return nil
	}
	return ip
}

func readGeo(rdr geoReader, ip net.IP) (geoRecord, bool) {
	if rdr == nil || ip == nil {
		return geoRecord{}, false
	}
	city, err := rdr.City(ip)
	if err == nil && city != nil {
		rec := geoRecord{
			CountryCode:  strings.ToUpper(strings.TrimSpace(city.Country.IsoCode)),
			CountryNames: copyNameMap(city.Country.Names),
		}
		if len(city.Subdivisions) > 0 {
			rec.RegionCode = strings.ToUpper(strings.TrimSpace(city.Subdivisions[0].IsoCode))
			rec.RegionNames = copyNameMap(city.Subdivisions[0].Names)
		}
		return rec, rec.CountryCode != "" || rec.RegionCode != "" || len(rec.CountryNames) > 0
	}
	ctry, err := rdr.Country(ip)
	if err != nil || ctry == nil {
		return geoRecord{}, false
	}
	rec := geoRecord{
		CountryCode:  strings.ToUpper(strings.TrimSpace(ctry.Country.IsoCode)),
		CountryNames: copyNameMap(ctry.Country.Names),
	}
	return rec, rec.CountryCode != "" || len(rec.CountryNames) > 0
}

func copyNameMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if strings.TrimSpace(v) == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func fuseGeoRecords(parts []geoRecord) GeoipResult {
	out := GeoipResult{
		GeoI18n: model.GeoI18n{
			Country: map[string]string{},
			Region:  map[string]string{},
		},
	}
	for _, p := range parts {
		if out.CountryCode == "" && strings.TrimSpace(p.CountryCode) != "" {
			out.CountryCode = strings.ToUpper(strings.TrimSpace(p.CountryCode))
		}
		if out.RegionCode == "" && strings.TrimSpace(p.RegionCode) != "" {
			out.RegionCode = strings.ToUpper(strings.TrimSpace(p.RegionCode))
		}
		mergeNamesFirst(out.GeoI18n.Country, p.CountryNames)
		mergeNamesFirst(out.GeoI18n.Region, p.RegionNames)
	}
	return out
}

func mergeNamesFirst(dst, src map[string]string) {
	for k, v := range src {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if cur, ok := dst[k]; ok && strings.TrimSpace(cur) != "" {
			continue
		}
		dst[k] = v
	}
}
