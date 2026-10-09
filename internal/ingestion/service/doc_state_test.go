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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/entity"
	ingestiontable "ragflow/internal/ingestion/table"
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
	deletedKeys     []string
	incrementCalled bool
	setErr          error
	readErr         error
}

func (s *stubDocStateSvc) WithDocumentMetadataLock(ctx context.Context, _ string, update func(context.Context) error) error {
	return update(ctx)
}

func (s *stubDocStateSvc) RevokeTableProfile(ctx context.Context, docID string) error {
	profile, _, err := ingestiontable.DecodeProfile(s.metaData[ingestiontable.ProfileMetadataField])
	if err != nil {
		return err
	}
	for _, key := range profile.OwnedKeys() {
		delete(s.metaData, key)
	}
	delete(s.metaData, ingestiontable.ProfileMetadataField)
	return nil
}

func (s *stubDocStateSvc) GetDocumentMetadataRaw(ctx context.Context, docID string) (map[string]any, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	if s.metaData == nil {
		return make(map[string]any), nil
	}
	return s.metaData, nil
}

func (s *stubDocStateSvc) SetDocumentMetadataRaw(ctx context.Context, docID string, meta map[string]any) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.setCalled = true
	// The engines write the record whole, so the stub replaces it rather than
	// folding the new map into the old one.
	s.metaData = meta
	return nil
}

func (s *stubDocStateSvc) DeleteDocumentMetadataRaw(ctx context.Context, docID string, keys []string) error {
	s.deletedKeys = append(s.deletedKeys, keys...)
	for _, key := range keys {
		delete(s.metaData, key)
	}
	return nil
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
	svc := &stubDocStateSvc{metaData: map[string]any{"update_time": "2020-01-01 00:00:00"}}
	u := &docStateUpdater{docSvc: svc}
	ctx := t.Context()
	u.apply(ctx, &taskpkg.PipelineResult{
		DocID:                 "doc-1",
		DocName:               "annual-report.pdf",
		AutoMetadataEnabled:   true,
		BuiltInMetadataConfig: []any{map[string]any{"key": "update_time", "type": "time"}},
		ChunkCount:            1, TokenConsumption: 10,
	})
	if svc.metaData["update_time"] == "2020-01-01 00:00:00" {
		t.Errorf("built-in update_time must overwrite the stored value: %v", svc.metaData["update_time"])
	}
	if _, ok := svc.metaData["update_time"].(string); !ok {
		t.Fatalf("update_time = %T, want string", svc.metaData["update_time"])
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

func tableProfileForTest(owned []string) *ingestiontable.Profile {
	return &ingestiontable.Profile{
		Engine:        "infinity",
		Columns:       ingestiontable.DeriveColumns([]string{"金额"}),
		OwnedMetadata: owned,
	}
}

func TestPublishTableProfileWritesRecordAndValues(t *testing.T) {
	svc := &stubDocStateSvc{metaData: map[string]any{"作者": "张三"}}
	u := &docStateUpdater{docSvc: svc}

	err := u.apply(context.Background(), &taskpkg.PipelineResult{
		DocID:        "doc-1",
		Metadata:     map[string]any{"金额": []string{"100", "200"}},
		TableProfile: tableProfileForTest([]string{"金额"}),
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	raw, ok := svc.metaData[ingestiontable.ProfileMetadataField].(string)
	if !ok {
		t.Fatalf("profile not published: %v", svc.metaData)
	}
	stored, decoded, err := ingestiontable.DecodeProfile(raw)
	if err != nil || !decoded {
		t.Fatalf("stored profile unreadable: ok=%v err=%v", decoded, err)
	}
	if stored.Engine != "infinity" || len(stored.Columns) != 1 || stored.Columns[0].Key != "金额" {
		t.Errorf("stored profile = %#v", stored)
	}
	if strings.Join(stored.OwnedMetadata, ",") != "金额" {
		t.Errorf("owned metadata = %v", stored.OwnedMetadata)
	}
	if svc.metaData["作者"] != "张三" {
		t.Errorf("user metadata lost: %v", svc.metaData)
	}
	if got := fmt.Sprintf("%v", svc.metaData["金额"]); got != "[100 200]" {
		t.Errorf("column values = %v", svc.metaData["金额"])
	}
}

// TestPublishNarrowsPreviousColumnValues is the reason the profile carries an
// ownership list: re-parsing a document whose rows changed must not leave the
// values of rows that no longer exist.
func TestPublishNarrowsPreviousColumnValues(t *testing.T) {
	previous := tableProfileForTest([]string{"金额"})
	encoded, err := previous.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	svc := &stubDocStateSvc{metaData: map[string]any{
		ingestiontable.ProfileMetadataField: encoded,
		"金额":                                []string{"100", "200", "300"},
		"作者":                                "张三",
	}}
	u := &docStateUpdater{docSvc: svc}

	narrowed := tableProfileForTest([]string{"金额"})
	err = u.apply(context.Background(), &taskpkg.PipelineResult{
		DocID:        "doc-1",
		Metadata:     map[string]any{"金额": []string{"100"}},
		TableProfile: narrowed,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, ok := svc.metaData["金额"].([]string)
	if !ok {
		t.Fatalf("金额 = %T %v, want the narrowed list", svc.metaData["金额"], svc.metaData["金额"])
	}
	if len(got) != 1 || got[0] != "100" {
		t.Errorf("金额 = %v, want only the values this run indexed", got)
	}
	if svc.metaData["作者"] != "张三" {
		t.Errorf("unrelated metadata lost: %v", svc.metaData)
	}
}

// TestPublishRetiredColumnIsDeleted covers the delete, not just the omission: an
// engine that folds an update into the stored map would keep the old value when
// a write merely leaves it out, and the column would go on looking queryable.
func TestPublishRetiredColumnIsDeleted(t *testing.T) {
	previous := tableProfileForTest([]string{"金额"})
	encoded, err := previous.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	svc := &stubDocStateSvc{metaData: map[string]any{
		ingestiontable.ProfileMetadataField: encoded,
		"金额":                                []string{"100"},
		"作者":                                "张三",
	}}
	u := &docStateUpdater{docSvc: svc}

	// This run extracts a non-table key and indexes no spreadsheet rows.
	if err := u.apply(context.Background(), &taskpkg.PipelineResult{
		DocID:    "doc-1",
		Metadata: map[string]any{"摘要": "季度销售"},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, still := svc.metaData["金额"]; still {
		t.Errorf("a retired column must be deleted: %v", svc.metaData)
	}
	if _, still := svc.metaData[ingestiontable.ProfileMetadataField]; still {
		t.Errorf("a document with no table rows must not keep a profile: %v", svc.metaData)
	}
	if !containsKey(svc.deletedKeys, ingestiontable.ProfileMetadataField) || !containsKey(svc.deletedKeys, "金额") {
		t.Errorf("expected explicit deletes, got %v", svc.deletedKeys)
	}
	if svc.metaData["作者"] != "张三" || svc.metaData["摘要"] != "季度销售" {
		t.Errorf("unrelated metadata changed: %v", svc.metaData)
	}
}

// TestPublishDoesNotRevokeTakenOverKey: a key the table system no longer owns
// was written by a user or the LLM, so a re-parse must leave it alone.
func TestPublishDoesNotRevokeTakenOverKey(t *testing.T) {
	previous := tableProfileForTest(nil)
	encoded, err := previous.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	svc := &stubDocStateSvc{metaData: map[string]any{
		ingestiontable.ProfileMetadataField: encoded,
		"金额":                                []string{"用户写的"},
	}}
	u := &docStateUpdater{docSvc: svc}

	if err := u.apply(context.Background(), &taskpkg.PipelineResult{DocID: "doc-1"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if svc.metaData["金额"] == nil {
		t.Errorf("a key the table system does not own was deleted: %v", svc.metaData)
	}
}

func TestPublishFailureFailsTheRun(t *testing.T) {
	// A profile that could not be published would leave the retrieval path
	// quoting columns with no rows behind them, so the task must not complete.
	svc := &stubDocStateSvc{
		metaData: map[string]any{},
		setErr:   errors.New("engine down"),
	}
	u := &docStateUpdater{docSvc: svc}
	err := u.apply(context.Background(), &taskpkg.PipelineResult{
		DocID:        "doc-1",
		Metadata:     map[string]any{"金额": []string{"100"}},
		TableProfile: tableProfileForTest([]string{"金额"}),
	})
	if err == nil {
		t.Fatal("expected the publish failure to surface")
	}
	if svc.incrementCalled {
		t.Error("a failed publish must not continue to counter bookkeeping")
	}
}

func TestPublishReadFailureFailsTheRun(t *testing.T) {
	svc := &stubDocStateSvc{readErr: errors.New("no record")}
	u := &docStateUpdater{docSvc: svc}
	if err := u.apply(context.Background(), &taskpkg.PipelineResult{
		DocID:        "doc-1",
		TableProfile: tableProfileForTest(nil),
	}); err == nil {
		t.Fatal("expected the read failure to surface")
	}
}

func TestPublishColumnPreservesUnownedValue(t *testing.T) {
	for _, previous := range []any{nil, publishedUnownedProfile(t)} {
		svc := &stubDocStateSvc{metaData: map[string]any{
			ingestiontable.ProfileMetadataField: previous,
			"金额":                                []string{"用户写的"},
		}}
		if err := (&docStateUpdater{docSvc: svc}).apply(t.Context(), &taskpkg.PipelineResult{
			DocID:        "doc-1",
			Metadata:     map[string]any{"金额": []string{"100"}},
			TableProfile: tableProfileForTest([]string{"金额"}),
		}); err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(svc.metaData["金额"]); got != "[用户写的]" {
			t.Errorf("table overwrote an unowned value: %s", got)
		}
		profile, ok, err := ingestiontable.DecodeProfile(svc.metaData[ingestiontable.ProfileMetadataField])
		if err != nil || !ok || len(profile.OwnedMetadata) != 0 {
			t.Fatalf("publisher claimed user metadata: %#v, %v", profile, err)
		}
	}
}

func publishedUnownedProfile(t *testing.T) string {
	t.Helper()
	raw, err := tableProfileForTest(nil).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPublishEmptyRunRevokesPreviousTable(t *testing.T) {
	raw, err := tableProfileForTest([]string{"金额"}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	svc := &stubDocStateSvc{metaData: map[string]any{ingestiontable.ProfileMetadataField: raw, "金额": []string{"100"}}}
	if err := (&docStateUpdater{docSvc: svc}).apply(t.Context(), &taskpkg.PipelineResult{DocID: "doc-1"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.metaData[ingestiontable.ProfileMetadataField]; ok {
		t.Fatal("an empty run left an old table profile queryable")
	}
	if _, ok := svc.metaData["金额"]; ok {
		t.Fatal("an empty run retained retired column values")
	}
}

func TestCancelledRunDoesNotPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	svc := &stubDocStateSvc{}
	err := (&docStateUpdater{docSvc: svc}).apply(ctx, &taskpkg.PipelineResult{DocID: "doc-1", TableProfile: tableProfileForTest(nil)})
	if !errors.Is(err, context.Canceled) || svc.setCalled || svc.incrementCalled {
		t.Fatalf("cancelled run published state: error=%v metadata=%v counts=%v", err, svc.setCalled, svc.incrementCalled)
	}
}

func TestStopRequestedBeforeFinalizationDoesNotPublish(t *testing.T) {
	svc := &stubDocStateSvc{}
	e := &Ingestor{
		docState:    &docStateUpdater{docSvc: svc},
		cancelCheck: func(context.Context, string) bool { return true },
	}
	err := e.finishDocumentTask(t.Context(), &entity.IngestionTask{ID: "task-1"}, "tenant-1", &taskpkg.PipelineResult{DocID: "doc-1", TableProfile: tableProfileForTest(nil)})
	if !errors.Is(err, context.Canceled) || svc.setCalled {
		t.Fatalf("stop request still published: error=%v published=%v", err, svc.setCalled)
	}
}

func TestBuiltInMetadataTakesOverTableKey(t *testing.T) {
	profile := &ingestiontable.Profile{
		Engine:        "infinity",
		Columns:       ingestiontable.DeriveColumns([]string{"file_name"}),
		OwnedMetadata: []string{"file_name"},
	}
	raw, err := profile.Encode()
	if err != nil {
		t.Fatal(err)
	}
	svc := &stubDocStateSvc{metaData: map[string]any{ingestiontable.ProfileMetadataField: raw, "file_name": "table value"}}
	if err := applyBuiltInMetadata(t.Context(), svc, "doc-1", "sales.xlsx", []any{map[string]any{"key": "file_name"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeTableProfile(t.Context(), "doc-1"); err != nil {
		t.Fatal(err)
	}
	if got := svc.metaData["file_name"]; got != "sales.xlsx" {
		t.Fatalf("table revocation removed built-in metadata: %v", got)
	}
}

func containsKey(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func TestPublishColumnPreservesEmptyUserValue(t *testing.T) {
	for _, value := range []any{"", []string{}} {
		svc := &stubDocStateSvc{metaData: map[string]any{"金额": value}}
		if err := (&docStateUpdater{docSvc: svc}).apply(t.Context(), &taskpkg.PipelineResult{DocID: "doc-1", Metadata: map[string]any{"金额": []string{"100"}}, TableProfile: tableProfileForTest([]string{"金额"})}); err != nil {
			t.Fatal(err)
		}
		got, present := svc.metaData["金额"]
		if !present || !reflect.DeepEqual(got, value) {
			t.Errorf("user value %T(%v) became %T(%v), present=%v", value, value, got, got, present)
		}
	}
}
