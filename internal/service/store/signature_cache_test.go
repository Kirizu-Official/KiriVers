package store

import (
	"bytes"
	"context"
	"crypto/ed25519"
	corand "crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// ---------- 测试夹具：纯内存目录加载器与存储 ----------

// fakeLoader 是 update.CatalogLoader 的测试实现：直接返回预制快照。
type fakeLoader struct{ cat *update.Catalog }

func (f *fakeLoader) LoadCatalog(_ context.Context, _ uuid.UUID, _, _ string) (*update.Catalog, error) {
	if f.cat == nil {
		return nil, errors.New("catalog unavailable")
	}
	return f.cat, nil
}

// fakeStorage 是 ArtifactReader 的测试实现：记录每键读取次数（LRU 命中断言）。
type fakeStorage struct {
	mu    sync.Mutex
	data  map[string][]byte
	reads map[string]int
}

func newFakeStorage(data map[string][]byte) *fakeStorage {
	return &fakeStorage{data: data, reads: make(map[string]int)}
}

func (f *fakeStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.data[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	f.reads[key]++
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeStorage) readCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[key]
}

// stubSigner 是 update.URLSigner 的测试实现：追加可识别的签名 query。
type stubSigner struct{}

func (stubSigner) SignDownload(path string, _ int) string {
	return path + "?exp=123&sig=abc"
}

// genEd25519PEM 生成测试用 Ed25519 密钥对（PKCS#8 私钥 / PKIX 公钥 PEM）。
func genEd25519PEM(t *testing.T) (privPEM, pubPEM string) {
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

// TestSignatureCacheGetOrCompute 并发单飞与 LRU 逐出语义。
func TestSignatureCacheGetOrCompute(t *testing.T) {
	cache := NewSignatureCache(2)
	computes := 0
	var mu sync.Mutex
	compute := func() (string, error) {
		mu.Lock()
		computes++
		mu.Unlock()
		return "sig", nil
	}

	// 并发 32 个同键调用：compute 只执行一次（单飞）。
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sig, err := cache.GetOrCompute("k1", compute)
			if err != nil || sig != "sig" {
				t.Errorf("GetOrCompute = %q, %v", sig, err)
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	if computes != 1 {
		t.Fatalf("computes = %d, want 1", computes)
	}
	mu.Unlock()

	// 命中不再计算。
	if _, err := cache.GetOrCompute("k1", compute); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if computes != 1 {
		mu.Unlock()
		t.Fatalf("cache hit recomputed: computes = %d", computes)
	}
	mu.Unlock()

	// LRU：容量 2，k2/k3 为新键各计算一次；填满后 k1 被逐出，再取会重新计算
	//（1 次 k1 + 1 次命中 + 2 次 k2/k3 + 1 次逐出后 k1 = 4）。
	if _, err := cache.GetOrCompute("k2", compute); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.GetOrCompute("k3", compute); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.GetOrCompute("k1", compute); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if computes != 4 {
		mu.Unlock()
		t.Fatalf("evicted key should recompute: computes = %d", computes)
	}
	mu.Unlock()
}

// TestSignatureCacheComputeError 错误结果不写缓存，共享给等待方。
func TestSignatureCacheComputeError(t *testing.T) {
	cache := NewSignatureCache(0)
	boom := errors.New("boom")
	if _, err := cache.GetOrCompute("k", func() (string, error) { return "", boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	computes := 0
	if _, err := cache.GetOrCompute("k", func() (string, error) {
		computes++
		return "", boom
	}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if computes != 1 {
		t.Fatalf("error result must not be cached: computes = %d", computes)
	}
}
