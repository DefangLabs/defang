package tokenstore

import (
	"errors"
	"strings"
	"sync"
)

// MemTokenStore is an in-memory TokenStore intended for use in tests.
type MemTokenStore struct {
	mu      sync.RWMutex
	tokens  map[string]string
	ListErr error // if set, List returns this error
}

func NewMemTokenStore() *MemTokenStore {
	return &MemTokenStore{tokens: make(map[string]string)}
}

func (s *MemTokenStore) Save(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[key] = value
	return nil
}

func (s *MemTokenStore) Load(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.tokens[key]
	if !ok {
		return "", errors.New("key not found")
	}
	return v, nil
}

func (s *MemTokenStore) List(prefix string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	var keys []string
	for k := range s.tokens {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *MemTokenStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, key)
	return nil
}
