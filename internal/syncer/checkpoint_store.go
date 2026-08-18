//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package syncer

import (
	"context"
	syncerconnector "ragflow/internal/syncer/connector"
	"sync"
)

// SyncCheckpointStore persists running sync task checkpoints.
type SyncCheckpointStore interface {
	LoadSyncCheckpoint(ctx context.Context, taskID string) (*syncerconnector.SyncCheckpointState, error)
	SaveSyncCheckpoint(ctx context.Context, taskID string, state syncerconnector.SyncCheckpointState) error
	DeleteSyncCheckpoint(ctx context.Context, taskID string) error
}

type memorySyncCheckpointStore struct {
	mu     sync.Mutex
	states map[string]syncerconnector.SyncCheckpointState
}

func newMemorySyncCheckpointStore() *memorySyncCheckpointStore {
	return &memorySyncCheckpointStore{states: map[string]syncerconnector.SyncCheckpointState{}}
}

func cloneSyncCheckpointState(state syncerconnector.SyncCheckpointState) syncerconnector.SyncCheckpointState {
	clone := state
	if state.WindowStart != nil {
		windowStart := *state.WindowStart
		clone.WindowStart = &windowStart
	}

	if state.Checkpoint != nil {
		checkpoint := *state.Checkpoint
		if checkpoint.UpdatedAt != nil {
			updatedAt := *checkpoint.UpdatedAt
			checkpoint.UpdatedAt = &updatedAt
		}
		clone.Checkpoint = &checkpoint
	}
	return clone
}

func (s *memorySyncCheckpointStore) LoadSyncCheckpoint(ctx context.Context, taskID string) (*syncerconnector.SyncCheckpointState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[taskID]
	if !ok {
		return nil, nil
	}
	clone := cloneSyncCheckpointState(state)
	return &clone, nil
}

func (s *memorySyncCheckpointStore) SaveSyncCheckpoint(ctx context.Context, taskID string, state syncerconnector.SyncCheckpointState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[taskID] = cloneSyncCheckpointState(state)
	return nil
}

func (s *memorySyncCheckpointStore) DeleteSyncCheckpoint(ctx context.Context, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, taskID)
	return nil
}
