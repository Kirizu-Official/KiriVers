package store

import (
	"container/list"
	"sync"
)

// DefaultSignatureCacheEntries 是 per-产物签名 LRU 的默认容量。一个条目仅是
// 一个签名字符串，单项目发布产物量级为数百，128 足以覆盖工作集；超出后
// 按 LRU 逐出（重新请求时再读文件签一次）。
const DefaultSignatureCacheEntries = 128

// signatureEntry 是 LRU 中的一个签名条目。
type signatureEntry struct {
	key string
	sig string
}

// signatureCall 是同一产物的在途签名计算（单飞）：并发首个请求执行
// compute，其余等待共享结果，避免同文件被并发重复读取与签名。
type signatureCall struct {
	done chan struct{}
	sig  string
	err  error
}

// SignatureCache 是互斥 + 单飞 + 容量上限的 per-产物签名缓存。
//
// 键为产物字节身份（SHA-256 hex）而非 ArtifactID：SHA-256 对字节恒定，
// 跨版本复用产物对象（§5.8）时天然共享同一签名，且无需目录暴露 UUID。
// 并发语义：读命中在锁内完成；未命中时同一键的后续调用等待首个计算
// （单飞），不同键并发计算不互斥。
type SignatureCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[string]*list.Element
	order    *list.List // front = 最近使用
	inflight map[string]*signatureCall
}

// NewSignatureCache 构造容量为 capacity 的签名缓存；capacity<=0 取默认值。
func NewSignatureCache(capacity int) *SignatureCache {
	if capacity <= 0 {
		capacity = DefaultSignatureCacheEntries
	}
	return &SignatureCache{
		capacity: capacity,
		entries:  make(map[string]*list.Element, capacity),
		order:    list.New(),
		inflight: make(map[string]*signatureCall),
	}
}

// GetOrCompute 返回键对应的签名；未命中时执行 compute 并写入缓存。
// compute 只在单飞胜者中执行一次，其余调用共享其结果（含错误）。
func (c *SignatureCache) GetOrCompute(key string, compute func() (string, error)) (string, error) {
	c.mu.Lock()
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)
		sig := el.Value.(*signatureEntry).sig
		c.mu.Unlock()
		return sig, nil
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.sig, call.err
	}
	call := &signatureCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	sig, err := compute()

	c.mu.Lock()
	delete(c.inflight, key)
	if err == nil {
		c.putLocked(key, sig)
	}
	c.mu.Unlock()

	call.sig, call.err = sig, err
	close(call.done)
	return sig, err
}

// putLocked 写入条目并在超容量时逐出 LRU 尾部；调用方已持有 mu。
func (c *SignatureCache) putLocked(key, sig string) {
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)
		el.Value.(*signatureEntry).sig = sig
		return
	}
	c.entries[key] = c.order.PushFront(&signatureEntry{key: key, sig: sig})
	for c.order.Len() > c.capacity {
		back := c.order.Back()
		if back == nil {
			return
		}
		c.order.Remove(back)
		delete(c.entries, back.Value.(*signatureEntry).key)
	}
}
