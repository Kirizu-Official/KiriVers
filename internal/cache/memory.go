package cache

import (
	"context"
	"strings"
	"sync"
	"time"
)

type memoryEntry struct {
	value []byte
	exp   time.Time // zero = immortal
}

// memoryStore 是进程内 mutex map 后端；driver=memory 直接返回，driver=redis 作为待机。
type memoryStore struct {
	mu   sync.Mutex
	data map[string]memoryEntry
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: map[string]memoryEntry{}}
}

func (m *memoryStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ent, ok := m.data[key]
	if !ok {
		return nil, false, nil
	}
	if !ent.exp.IsZero() && !time.Now().Before(ent.exp) {
		delete(m.data, key)
		return nil, false, nil
	}
	return cloneBytes(ent.value), true, nil
}

func (m *memoryStore) Set(_ context.Context, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = memoryEntry{value: cloneBytes(value)}
	return nil
}

func (m *memoryStore) SetWithTTL(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = memoryEntry{value: cloneBytes(value), exp: time.Now().Add(ttl)}
	return nil
}

func (m *memoryStore) SetNX(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ent, ok := m.data[key]; ok {
		if ent.exp.IsZero() || time.Now().Before(ent.exp) {
			return false, nil
		}
		delete(m.data, key)
	}
	ent := memoryEntry{value: cloneBytes(value)}
	if ttl > 0 {
		ent.exp = time.Now().Add(ttl)
	}
	m.data[key] = ent
	return true, nil
}

func (m *memoryStore) Delete(_ context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.data, key)
	}
	return nil
}

func (m *memoryStore) DeletePrefix(_ context.Context, prefix string) error {
	if prefix == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.data {
		if strings.HasPrefix(key, prefix) {
			delete(m.data, key)
		}
	}
	return nil
}

func (m *memoryStore) Close() error { return nil }

func (m *memoryStore) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = map[string]memoryEntry{}
}
