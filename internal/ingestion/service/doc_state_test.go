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

	taskpkg "ragflow/internal/ingestion/task"
)

type stubDocStateSvc struct {
	metaData        map[string]any
	gotDocID        string
	gotKbID         string
	gotChunkNum     int
	gotTokenNum     int
	gotDuration     float64
	setCalled       bool
	incrementCalled bool
}

func (s *stubDocStateSvc) GetDocumentMetadataByID(ctx context.Context, docID string) (map[string]any, error) {
	if s.metaData == nil {
		return make(map[string]any), nil
	}
	return s.metaData, nil
}

func (s *stubDocStateSvc) SetDocumentMetadata(ctx context.Context, docID string, meta map[string]any) error {
	s.setCalled = true
	if s.metaData == nil {
		s.metaData = make(map[string]any)
	}
	for k, v := range meta {
		s.metaData[k] = v
	}
	return nil
}

func (s *stubDocStateSvc) IncrementChunkNum(ctx context.Context, docID, kbID string, chunkNum, tokenNum int, duration float64) error {
	s.incrementCalled = true
	s.gotDocID = docID
	s.gotKbID = kbID
	s.gotChunkNum = chunkNum
	s.gotTokenNum = tokenNum
	s.gotDuration = duration
	return nil
}

func TestDocStateUpdater_NilResultIsNoop(t *testing.T) {
	svc := &stubDocStateSvc{}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, nil)
	if svc.setCalled || svc.incrementCalled {
		t.Fatal("nil result must not touch document state")
	}
}

func TestDocStateUpdater_EmptyMetadataSkipsMerge(t *testing.T) {
	svc := &stubDocStateSvc{}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{DocID: "doc-1", KbID: "kb-1", ChunkCount: 3, TokenConsumption: 100})
	if svc.setCalled {
		t.Fatal("empty metadata must not call SetDocumentMetadata")
	}
	if !svc.incrementCalled {
		t.Fatal("counter must still be bumped")
	}
	if svc.gotChunkNum != 3 || svc.gotTokenNum != 100 {
		t.Fatalf("chunk=%d token=%d, want 3/100", svc.gotChunkNum, svc.gotTokenNum)
	}
}

func TestDocStateUpdater_MergesNewKeys(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{"existing": "old"}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{DocID: "doc-1", Metadata: map[string]any{"new_key": "value"}, ChunkCount: 1, TokenConsumption: 10})
	if svc.metaData["existing"] != "old" {
		t.Errorf("existing key should be preserved: got %q", svc.metaData["existing"])
	}
	if svc.metaData["new_key"] != "value" {
		t.Errorf("new_key = %q, want \"value\"", svc.metaData["new_key"])
	}
}

func TestDocStateUpdater_PreservesExistingKey(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{"author": "Alice"}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{DocID: "doc-1", Metadata: map[string]any{"author": "Bob"}, ChunkCount: 1, TokenConsumption: 10})
	if svc.metaData["author"] != "Alice" {
		t.Errorf("existing key must NOT be overwritten: got %q", svc.metaData["author"])
	}
}

func TestDocStateUpdater_UnionsListValues(t *testing.T) {
	// Stored doc metadata already has a list value; a new run extracts more
	// (possibly overlapping) values. The merge must union + de-dupe, matching
	// Python update_metadata_to(metadata, existing_meta).
	svc := &stubDocStateSvc{metaData: map[string]any{"people": []string{"关羽", "张辽"}}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID:      "doc-1",
		Metadata:   map[string]any{"people": []string{"张辽", "刘备"}},
		ChunkCount: 1, TokenConsumption: 10,
	})
	got, ok := svc.metaData["people"].([]string)
	if !ok {
		t.Fatalf("people should be []string, got %T", svc.metaData["people"])
	}
	want := []string{"张辽", "刘备", "关羽"}
	if len(got) != len(want) {
		t.Fatalf("people=%v, want union %v", got, want)
	}
	seen := map[string]bool{}
	for _, p := range got {
		if seen[p] {
			t.Fatalf("duplicate %q in %v", p, got)
		}
		seen[p] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Fatalf("missing %q in %v", w, got)
		}
	}
}

func TestDocStateUpdater_PreservesExistingScalar(t *testing.T) {
	// Python update_metadata_to keeps the stored scalar when both sides carry
	// a scalar for the same key (stored wins).
	svc := &stubDocStateSvc{metaData: map[string]any{"author": "Alice"}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID:      "doc-1",
		Metadata:   map[string]any{"author": "Bob"},
		ChunkCount: 1, TokenConsumption: 10,
	})
	if svc.metaData["author"] != "Alice" {
		t.Fatalf("stored scalar must win: got %q", svc.metaData["author"])
	}
}

func TestDocStateUpdater_IncrementArgs(t *testing.T) {
	svc := &stubDocStateSvc{}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{DocID: "doc-1", KbID: "kb-1", ChunkCount: 10, TokenConsumption: 100})
	if svc.gotDocID != "doc-1" || svc.gotKbID != "kb-1" {
		t.Fatalf("docID=%q kbID=%q, want doc-1/kb-1", svc.gotDocID, svc.gotKbID)
	}
	if svc.gotChunkNum != 10 || svc.gotTokenNum != 100 {
		t.Fatalf("chunk=%d token=%d, want 10/100", svc.gotChunkNum, svc.gotTokenNum)
	}
	if svc.gotDuration != 0 {
		t.Fatalf("duration=%f, want 0", svc.gotDuration)
	}
}
