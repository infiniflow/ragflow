// Package checkpoint provides checkpoint implementations for LangGraph Go.
package checkpoint

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"ragflow/internal/harness/graphengine/constants"
)

// MemorySaver is an in-memory checkpoint saver.
type MemorySaver struct {
	mu          sync.RWMutex
	checkpoints map[string]map[string]interface{}
	// Store multiple checkpoints per thread for versioning
	versions    map[string][]checkpointEntry
}

type checkpointEntry struct {
	ID          string
	ThreadID    string
	Checkpoint  map[string]interface{}
	Metadata    map[string]interface{}
	CreatedAt   time.Time
	ParentID    string
}

// NewMemorySaver creates a new in-memory checkpoint saver.
func NewMemorySaver() *MemorySaver {
	return &MemorySaver{
		checkpoints: make(map[string]map[string]interface{}),
		versions:    make(map[string][]checkpointEntry),
	}
}

// Get retrieves the latest checkpoint for a thread.
func (s *MemorySaver) Get(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	threadID, ok := config[constants.ConfigKeyThreadID].(string)
	if !ok {
		return nil, fmt.Errorf("thread_id is required")
	}
	
	// Check for specific checkpoint_id
	if checkpointID, ok := config[constants.ConfigKeyCheckpointID].(string); ok {
		versions := s.versions[threadID]
		for _, entry := range versions {
			if entry.ID == checkpointID {
				cp := deepCopy(entry.Checkpoint)
				if m, ok := cp.(map[string]interface{}); ok {
					return m, nil
				}
				return nil, fmt.Errorf("checkpoint has invalid type")
			}
		}
		return nil, fmt.Errorf("checkpoint not found: %s", checkpointID)
	}
	
	// Return latest checkpoint
	versions := s.versions[threadID]
	if len(versions) == 0 {
		return nil, nil
	}
	
	cp := deepCopy(versions[len(versions)-1].Checkpoint)
	if m, ok := cp.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, fmt.Errorf("checkpoint has invalid type")
}

// Put saves a new checkpoint.
func (s *MemorySaver) Put(ctx context.Context, config map[string]interface{}, checkpoint map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	threadID, ok := config[constants.ConfigKeyThreadID].(string)
	if !ok {
		return fmt.Errorf("thread_id is required")
	}
	
	checkpointID := uuid.New().String()
	if id, ok := config[constants.ConfigKeyCheckpointID].(string); ok {
		checkpointID = id
	}
	
	entry := checkpointEntry{
		ID:         checkpointID,
		ThreadID:   threadID,
		Checkpoint: deepCopyMap(checkpoint),
		Metadata:   deepCopyMap(config),
		CreatedAt:  time.Now(),
	}
	
	if parentID, ok := config["parent_checkpoint_id"].(string); ok {
		entry.ParentID = parentID
	}
	
	s.versions[threadID] = append(s.versions[threadID], entry)
	cp := deepCopy(checkpoint)
	if m, ok := cp.(map[string]interface{}); ok {
		s.checkpoints[threadID] = m
	}
	
	return nil
}

// List lists checkpoints for a thread.
func (s *MemorySaver) List(ctx context.Context, config map[string]interface{}, limit int) ([]map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	threadID, ok := config[constants.ConfigKeyThreadID].(string)
	if !ok {
		return nil, fmt.Errorf("thread_id is required")
	}
	
	versions := s.versions[threadID]
	if limit <= 0 || limit > len(versions) {
		limit = len(versions)
	}
	
	result := make([]map[string]interface{}, 0, limit)
	for i := len(versions) - 1; i >= len(versions)-limit && i >= 0; i-- {
		entry := versions[i]
		result = append(result, map[string]interface{}{
			constants.ConfigKeyCheckpointID: entry.ID,
			constants.ConfigKeyThreadID:     entry.ThreadID,
			"metadata":      entry.Metadata,
			"created_at":    entry.CreatedAt,
			"parent_id":     entry.ParentID,
		})
	}
	
	return result, nil
}

// GetState retrieves a specific checkpoint by ID.
func (s *MemorySaver) GetState(ctx context.Context, config map[string]interface{}) (*CheckpointState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	threadID, ok := config[constants.ConfigKeyThreadID].(string)
	if !ok {
		return nil, fmt.Errorf("thread_id is required")
	}
	
	checkpointID, ok := config[constants.ConfigKeyCheckpointID].(string)
	if !ok {
		// Get latest
		versions := s.versions[threadID]
		if len(versions) == 0 {
			return nil, nil
		}
		entry := versions[len(versions)-1]
		return &CheckpointState{
			Checkpoint: entry.Checkpoint,
			Metadata:   entry.Metadata,
		}, nil
	}
	
	versions := s.versions[threadID]
	for _, entry := range versions {
		if entry.ID == checkpointID {
			return &CheckpointState{
				Checkpoint: entry.Checkpoint,
				Metadata:   entry.Metadata,
			}, nil
		}
	}
	
	return nil, fmt.Errorf("checkpoint not found: %s", checkpointID)
}

// CheckpointState represents a checkpoint with its metadata.
type CheckpointState struct {
	Checkpoint map[string]interface{}
	Metadata   map[string]interface{}
}


