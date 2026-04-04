package statestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type entry[V any] struct {
	Value V     `json:"v"`
	TS    int64 `json:"ts,omitempty"`
}

// Store is a generic in-memory cache backed by an atomic JSON file.
type Store[V any] struct {
	path string
	ttl  time.Duration
	mu   sync.RWMutex
	data map[string]entry[V]
}

// Open loads a store from disk, pruning expired entries.
func Open[V any](path string, ttl time.Duration) (*Store[V], error) {
	s := &Store[V]{
		path: path,
		ttl:  ttl,
		data: make(map[string]entry[V]),
	}

	if data, err := os.ReadFile(path); err == nil {
		var loaded map[string]entry[V]
		if err := json.Unmarshal(data, &loaded); err == nil {
			now := time.Now().Unix()
			for k, e := range loaded {
				if e.TS == 0 {
					s.data[k] = e
					continue
				}
				if ttl == 0 || now-e.TS < int64(ttl.Seconds()) {
					s.data[k] = e
				}
			}
		}
	}

	return s, nil
}

// Get retrieves a value by key.
func (s *Store[V]) Get(key string) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	if !ok {
		var zero V
		return zero, false
	}
	if e.TS != 0 && s.ttl > 0 && time.Now().Unix()-e.TS >= int64(s.ttl.Seconds()) {
		var zero V
		return zero, false
	}
	return e.Value, true
}

// Set stores a value with the current timestamp.
func (s *Store[V]) Set(key string, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = entry[V]{
		Value: value,
		TS:    time.Now().Unix(),
	}
}

// SetPermanent stores a value without a timestamp (never pruned).
func (s *Store[V]) SetPermanent(key string, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = entry[V]{Value: value}
}

// Delete removes a key.
func (s *Store[V]) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// Save persists the store to disk atomically.
func (s *Store[V]) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("mkdir store dir: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write store tmp: %w", err)
	}

	return os.Rename(tmp, s.path)
}

// Len returns the number of entries.
func (s *Store[V]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}
