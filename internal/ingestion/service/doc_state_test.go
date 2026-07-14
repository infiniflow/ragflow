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

func (s *stubDocStateSvc) GetDocumentMetadataByID(docID string) (map[string]any, error) {
	if s.metaData == nil {
		return make(map[string]any), nil
	}
	return s.metaData, nil
}

func (s *stubDocStateSvc) SetDocumentMetadata(docID string, meta map[string]any) error {
	s.setCalled = true
	if s.metaData == nil {
		s.metaData = make(map[string]any)
	}
	for k, v := range meta {
		s.metaData[k] = v
	}
	return nil
}

func (s *stubDocStateSvc) IncrementChunkNum(docID, kbID string, chunkNum, tokenNum int, duration float64) error {
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
	u.apply(nil)
	if svc.setCalled || svc.incrementCalled {
		t.Fatal("nil result must not touch document state")
	}
}

func TestDocStateUpdater_EmptyMetadataSkipsMerge(t *testing.T) {
	svc := &stubDocStateSvc{}
	u := &docStateUpdater{docSvc: svc}
	u.apply(&taskpkg.PipelineResult{DocID: "doc-1", KbID: "kb-1", ChunkCount: 3, TokenConsumption: 100})
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
	u.apply(&taskpkg.PipelineResult{DocID: "doc-1", Metadata: map[string]any{"new_key": "value"}, ChunkCount: 1, TokenConsumption: 10})
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
	u.apply(&taskpkg.PipelineResult{DocID: "doc-1", Metadata: map[string]any{"author": "Bob"}, ChunkCount: 1, TokenConsumption: 10})
	if svc.metaData["author"] != "Alice" {
		t.Errorf("existing key must NOT be overwritten: got %q", svc.metaData["author"])
	}
}

func TestDocStateUpdater_IncrementArgs(t *testing.T) {
	svc := &stubDocStateSvc{}
	u := &docStateUpdater{docSvc: svc}
	u.apply(&taskpkg.PipelineResult{DocID: "doc-1", KbID: "kb-1", ChunkCount: 10, TokenConsumption: 100})
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
