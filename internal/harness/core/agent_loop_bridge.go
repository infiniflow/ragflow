package core

import (
	"context"
	"sync"
)

const bridgeCheckpointID = "__adk_turnloop_bridge_cp__"

// bridgeStore is a minimal CheckPointStore used to bridge AgentLoop with Runner
// checkpoints without using the actual Store.
type bridgeStore struct {
	cpID string
	data []byte
	mu   sync.RWMutex
}

func newBridgeStore() *bridgeStore {
	return &bridgeStore{cpID: bridgeCheckpointID}
}

func newResumeBridgeStore(cpID string, data []byte) *bridgeStore {
	return &bridgeStore{cpID: cpID, data: append([]byte{}, data...)}
}

func (s *bridgeStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if key != s.cpID {
		return nil, false, nil
	}
	if len(s.data) == 0 {
		return nil, false, nil
	}
	return append([]byte{}, s.data...), true, nil
}

func (s *bridgeStore) Set(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == s.cpID {
		s.data = append([]byte{}, data...)
	}
	return nil
}

func (s *bridgeStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == s.cpID {
		s.data = nil
	}
	return nil
}

var _ CheckPointStore = (*bridgeStore)(nil)
var _ CheckPointDeleter = (*bridgeStore)(nil)
