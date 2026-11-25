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

package service

import (
	"context"
	"testing"

	"ragflow/internal/engine"
)

// chunkStoreCountingEngine records index metadata calls made while ensuring a
// skill index exists.
type chunkStoreCountingEngine struct {
	engine.DocEngine
	engineType  string
	exists      bool
	existsCalls int
	dropCalls   int
	createCalls int
	createdDims []int
}

func (e *chunkStoreCountingEngine) GetType() string {
	if e.engineType == "" {
		return "infinity"
	}
	return e.engineType
}

func (e *chunkStoreCountingEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	e.existsCalls++
	return e.exists, nil
}

func (e *chunkStoreCountingEngine) DropChunkStore(context.Context, string, string) error {
	e.dropCalls++
	return nil
}

func (e *chunkStoreCountingEngine) CreateChunkStore(_ context.Context, _, _ string, dimension int, _, _ string) error {
	e.createCalls++
	e.createdDims = append(e.createdDims, dimension)
	return nil
}

// TestCreateIndexWithDimensionSkipsRedundantExistenceCheck is the core RED
// assertion: callers already probed ChunkStoreExists before creating, so the
// create path must not issue a second metadata round trip.
func TestCreateIndexWithDimensionSkipsRedundantExistenceCheck(t *testing.T) {
	e := &chunkStoreCountingEngine{exists: false}
	svc := &SkillIndexerService{}
	if err := svc.createIndexWithDimension(t.Context(), "tenant-1", "space-1", e, "embd-1", 128); err != nil {
		t.Fatalf("createIndexWithDimension: %v", err)
	}
	if e.existsCalls != 0 {
		t.Fatalf("ChunkStoreExists called %d times, want 0", e.existsCalls)
	}
	if e.dropCalls != 0 || e.createCalls != 1 {
		t.Fatalf("drop/create calls = %d/%d, want 0/1", e.dropCalls, e.createCalls)
	}
	if len(e.createdDims) != 1 || e.createdDims[0] != 128 {
		t.Fatalf("created dimensions = %v, want [128]", e.createdDims)
	}
}

// TestCreateIndexWithDimensionNonInfinityNeverProbes pins that the existence
// question is irrelevant on engines without the dimension-recreate path.
func TestCreateIndexWithDimensionNonInfinityNeverProbes(t *testing.T) {
	e := &chunkStoreCountingEngine{engineType: "elasticsearch", exists: true}
	svc := &SkillIndexerService{}
	if err := svc.createIndexWithDimension(t.Context(), "tenant-1", "space-1", e, "embd-1", 128); err != nil {
		t.Fatalf("createIndexWithDimension: %v", err)
	}
	if e.existsCalls != 0 || e.dropCalls != 0 || e.createCalls != 1 {
		t.Fatalf("exists/drop/create = %d/%d/%d, want 0/0/1", e.existsCalls, e.dropCalls, e.createCalls)
	}
}
