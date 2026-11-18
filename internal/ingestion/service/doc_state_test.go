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
	"reflect"
	"testing"
	"time"

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
	s.metaData = make(map[string]any, len(meta))
	for k, v := range meta {
		s.metaData[k] = v
	}
	return nil
}

func TestDocStateUpdater_PreservesTypedMetadataOnReplacement(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{
		"year": float64(2024), "verified": false, "null": nil,
		"tags":    []any{"contract", float64(123)},
		"outline": []any{map[string]any{"title": "Intro", "depth": float64(1)}},
	}}
	u := &docStateUpdater{docSvc: svc}
	u.apply(t.Context(), &taskpkg.PipelineResult{
		DocID: "doc-1", Metadata: map[string]any{
			"author": "Alice", "year": map[string]any{"value": float64(2025)},
		},
	})
	want := map[string]any{
		"author": "Alice", "year": map[string]any{"value": float64(2025)}, "verified": false, "null": nil,
		"tags":    []any{"contract", float64(123)},
		"outline": []any{map[string]any{"title": "Intro", "depth": float64(1)}},
	}
	if !svc.setCalled || !reflect.DeepEqual(svc.metaData, want) {
		t.Fatalf("stored metadata = %#v, want %#v", svc.metaData, want)
	}
}

func (s *stubDocStateSvc) ApplyDocCounts(ctx context.Context, docID, kbID string, chunkNum, tokenNum int, duration float64) error {
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

func TestDocStateUpdater_ReplacesExistingKey(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{"author": "Alice"}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{DocID: "doc-1", Metadata: map[string]any{"author": "Bob"}, ChunkCount: 1, TokenConsumption: 10})
	if svc.metaData["author"] != "Bob" {
		t.Errorf("new value must replace existing key: got %q", svc.metaData["author"])
	}
}

func TestDocStateUpdater_ReplacesListValues(t *testing.T) {
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
	want := []string{"张辽", "刘备"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("people=%v, want replacement %v", got, want)
	}
}

func TestDocStateUpdater_NewValuesReplaceStoredValues(t *testing.T) {
	tests := []struct {
		name              string
		stored, extracted any
	}{
		{"number", float64(2024), float64(2025)},
		{"false", true, false},
		{"zero", float64(42), float64(0)},
		{"null", "old", nil},
		{"object to scalar", map[string]any{"x": 1}, "new"},
		{"scalar to object", "old", map[string]any{"x": 2}},
		{"structured list to scalar", []any{1, 2}, float64(42)},
		{"scalar to structured list", "old", []any{1, 2}},
		{"structured list", []any{1, 2}, []any{3, false}},
		{"object", map[string]any{"x": 1}, map[string]any{"x": 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubDocStateSvc{metaData: map[string]any{"key": tt.stored, "keep": false}}
			u := &docStateUpdater{docSvc: svc}
			u.apply(t.Context(), &taskpkg.PipelineResult{
				DocID: "doc-1", Metadata: map[string]any{"key": tt.extracted},
			})
			want := map[string]any{"key": tt.extracted, "keep": false}
			if !svc.setCalled || !reflect.DeepEqual(svc.metaData, want) {
				t.Fatalf("stored metadata = %#v, want %#v", svc.metaData, want)
			}
		})
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

func TestDocStateUpdater_AppliesBuiltInMetadata(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{"category": "finance"}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID:               "doc-1",
		DocName:             "annual-report.pdf",
		AutoMetadataEnabled: true,
		BuiltInMetadataConfig: []any{
			map[string]any{"key": "update_time", "type": "time"},
			map[string]any{"key": "file_name", "type": "string"},
		},
		ChunkCount: 1, TokenConsumption: 10,
	})
	if svc.metaData["file_name"] != "annual-report.pdf" {
		t.Errorf("file_name = %v, want annual-report.pdf", svc.metaData["file_name"])
	}
	updateTime, ok := svc.metaData["update_time"].(string)
	if !ok {
		t.Fatalf("update_time = %T, want string", svc.metaData["update_time"])
	}
	if _, err := time.Parse("2006-01-02 15:04:05", updateTime); err != nil {
		t.Errorf("update_time = %q, want 2006-01-02 15:04:05 format", updateTime)
	}
	if svc.metaData["category"] != "finance" {
		t.Errorf("existing metadata must be preserved: category = %v", svc.metaData["category"])
	}
}

func TestDocStateUpdater_BuiltInMetadataGatedOff(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID:                 "doc-1",
		DocName:               "annual-report.pdf",
		AutoMetadataEnabled:   false,
		BuiltInMetadataConfig: []any{map[string]any{"key": "file_name", "type": "string"}},
		ChunkCount:            1, TokenConsumption: 10,
	})
	if _, ok := svc.metaData["file_name"]; ok {
		t.Errorf("built-in metadata must not be applied when auto-metadata is disabled: %v", svc.metaData)
	}
}

func TestDocStateUpdater_BuiltInMetadataOverwritesExisting(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{
		"update_time": []any{float64(2020)},
		"file_name":   map[string]any{"old": true},
		"keep":        false,
	}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID:               "doc-1",
		DocName:             "annual-report.pdf",
		AutoMetadataEnabled: true,
		BuiltInMetadataConfig: []any{
			map[string]any{"key": "update_time", "type": "time"},
			map[string]any{"key": "file_name", "type": "string"},
		},
		ChunkCount: 1, TokenConsumption: 10,
	})
	if svc.metaData["update_time"] == "2020-01-01 00:00:00" {
		t.Errorf("built-in update_time must overwrite the stored value: %v", svc.metaData["update_time"])
	}
	if _, ok := svc.metaData["update_time"].(string); !ok {
		t.Fatalf("update_time = %T, want string", svc.metaData["update_time"])
	}
	if svc.metaData["file_name"] != "annual-report.pdf" || svc.metaData["keep"] != false {
		t.Fatalf("built-in fields must overwrite structures and retain other keys: %#v", svc.metaData)
	}
}

func TestDocStateUpdater_BuiltInMetadataEmptyConfig(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID: "doc-1", DocName: "annual-report.pdf",
		AutoMetadataEnabled: true,
		ChunkCount:          1, TokenConsumption: 10,
	})
	if len(svc.metaData) != 0 {
		t.Errorf("empty built-in config must be a no-op, got %v", svc.metaData)
	}
}

func TestDocStateUpdater_BuiltInNotWrittenWhenEnabledFalse(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID:               "doc-1",
		DocName:             "report.pdf",
		AutoMetadataEnabled: false,
		BuiltInMetadataConfig: []any{
			map[string]any{"key": "file_name", "type": "string"},
			map[string]any{"key": "update_time", "type": "time"},
		},
		Metadata:         map[string]any{"author": "Alice"},
		ChunkCount:       1,
		TokenConsumption: 1,
	})
	if _, ok := svc.metaData["file_name"]; ok {
		t.Fatalf("built_in must not be written when enabled=false, got %v", svc.metaData)
	}
	if _, ok := svc.metaData["update_time"]; ok {
		t.Fatalf("built_in must not be written when enabled=false, got %v", svc.metaData)
	}
	if svc.metaData["author"] != "Alice" {
		t.Fatalf("custom metadata should still be written even when built_in is off, got %v", svc.metaData)
	}
}
