//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

//go:build !ee

package service

import (
	"context"
	"sort"
	"sync"
)

// memoryTaskClaimStore is the in-memory task claim store: a process-local set
// of task IDs held by a running worker. It guards against duplicate deliveries
// within one ingestor process.
type memoryTaskClaimStore struct {
	mu     sync.RWMutex
	claims map[string]struct{}
}

func newTaskClaimStore(string) taskClaimStore {
	return &memoryTaskClaimStore{claims: make(map[string]struct{})}
}

func (s *memoryTaskClaimStore) Claim(_ context.Context, taskID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.claims[taskID]; ok {
		return false, nil
	}
	s.claims[taskID] = struct{}{}
	return true, nil
}

func (s *memoryTaskClaimStore) Release(_ context.Context, taskID string) {
	s.mu.Lock()
	delete(s.claims, taskID)
	s.mu.Unlock()
}

func (s *memoryTaskClaimStore) InFlight() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.claims))
	for id := range s.claims {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
