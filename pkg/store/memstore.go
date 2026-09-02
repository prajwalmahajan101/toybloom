package store

import (
	"context"
	"fmt"
	"maps"
	"sync"
)

// MemStore is an in-memory BitStore for testing. It is safe for concurrent use.
type MemStore struct {
	mu       sync.Mutex
	bitmaps  map[string]map[uint64]bool
	hashes   map[string]map[string]string
	counters map[string]int64
	sets     map[string]map[string]bool
}

// NewMemStore returns a ready-to-use MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		bitmaps:  make(map[string]map[uint64]bool),
		hashes:   make(map[string]map[string]string),
		counters: make(map[string]int64),
		sets:     make(map[string]map[string]bool),
	}
}

func (m *MemStore) Ping(_ context.Context) error { return nil }

func (m *MemStore) SetBits(_ context.Context, key string, offsets []uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	bm := m.bitmaps[key]
	if bm == nil {
		bm = make(map[uint64]bool)
		m.bitmaps[key] = bm
	}
	for _, o := range offsets {
		bm[o] = true
	}
	return nil
}

func (m *MemStore) GetBits(_ context.Context, key string, offsets []uint64) ([]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	bm := m.bitmaps[key]
	result := make([]bool, len(offsets))
	for i, o := range offsets {
		result[i] = bm[o]
	}
	return result, nil
}

func (m *MemStore) HGetAll(_ context.Context, key string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.hashes[key]
	if h == nil {
		return map[string]string{}, nil
	}
	cp := make(map[string]string, len(h))
	maps.Copy(cp, h)
	return cp, nil
}

func (m *MemStore) HSet(_ context.Context, key string, fields map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.hashes[key]
	if h == nil {
		h = make(map[string]string)
		m.hashes[key] = h
	}
	maps.Copy(h, fields)
	return nil
}

func (m *MemStore) Incr(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[key]++
	return m.counters[key], nil
}

func (m *MemStore) SAdd(_ context.Context, key string, members ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sets[key]
	if s == nil {
		s = make(map[string]bool)
		m.sets[key] = s
	}
	for _, mem := range members {
		s[mem] = true
	}
	return nil
}

func (m *MemStore) SRem(_ context.Context, key string, members ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sets[key]
	if s == nil {
		return nil
	}
	for _, mem := range members {
		delete(s, mem)
	}
	return nil
}

func (m *MemStore) SMembers(_ context.Context, key string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sets[key]
	out := make([]string, 0, len(s))
	for mem := range s {
		out = append(out, mem)
	}
	return out, nil
}

func (m *MemStore) Del(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.bitmaps, k)
		delete(m.hashes, k)
		delete(m.counters, k)
		delete(m.sets, k)
	}
	return nil
}

func (m *MemStore) AppendStage(_ context.Context, metaKey string, expected int64, newStageFields map[string]string, newStageMetaKey string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.hashes[metaKey]
	if h == nil {
		return 0, fmt.Errorf("memstore: meta key %q not found", metaKey)
	}
	var current int64
	if v, ok := h["stage_count"]; ok {
		if _, err := fmt.Sscanf(v, "%d", &current); err != nil {
			return 0, fmt.Errorf("memstore: parse stage_count %q: %w", v, err)
		}
	}
	if current != expected {
		return current, nil
	}
	sh := make(map[string]string, len(newStageFields))
	maps.Copy(sh, newStageFields)
	m.hashes[newStageMetaKey] = sh
	current++
	h["stage_count"] = fmt.Sprintf("%d", current)
	return current, nil
}
