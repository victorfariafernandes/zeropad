package store

import (
	"sync"
	"time"

	"zeropad-backend/encryption"
)

type Pad struct {
	Content          string             `json:"content"`
	Encrypted        bool               `json:"encrypted"`
	VerifyBlob       string             `json:"verify_blob"`
	DeriverId        encryption.Deriver `json:"deriver_id"`
	HashedWriteToken string             `json:"hashed_write_token"`
	UpdatedAt        time.Time          `json:"updated_at"`
}

type PadStore interface {
	Get(slug string) (Pad, bool)
	Set(slug string, pad Pad)
	Delete(slug string)
}

type MemoryPadStore struct {
	mu   sync.RWMutex
	pads map[string]Pad
}

func NewMemoryPadStore() *MemoryPadStore {
	return &MemoryPadStore{pads: make(map[string]Pad)}
}

func (s *MemoryPadStore) Get(slug string) (Pad, bool) {
	s.mu.RLock()
	pad, ok := s.pads[slug]
	s.mu.RUnlock()
	return pad, ok
}

func (s *MemoryPadStore) Set(slug string, pad Pad) {
	s.mu.Lock()
	s.pads[slug] = pad
	s.mu.Unlock()
}

func (s *MemoryPadStore) Delete(slug string) {
	s.mu.Lock()
	delete(s.pads, slug)
	s.mu.Unlock()
}
