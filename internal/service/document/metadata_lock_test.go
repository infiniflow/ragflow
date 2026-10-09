package document

import (
	"context"
	"sync"
	"time"
)

type memoryMetadataLocks struct {
	mu     sync.Mutex
	owners map[string]string
}

func (s *memoryMetadataLocks) SetNX(_ context.Context, key, owner string, _ time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owners == nil {
		s.owners = make(map[string]string)
	}
	if _, held := s.owners[key]; held {
		return false
	}
	s.owners[key] = owner
	return true
}

func (s *memoryMetadataLocks) DeleteIfEqual(_ context.Context, key, owner string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owners[key] != owner {
		return false
	}
	delete(s.owners, key)
	return true
}
