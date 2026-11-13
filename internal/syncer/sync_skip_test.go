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
	"errors"
	"io"
	"ragflow/internal/dao"
	"ragflow/internal/service"
	"ragflow/internal/syncer/connector"
	"strings"
	"testing"
)

func TestSkippedDocumentDoesNotResolveFetchOrUpsert(t *testing.T) {
	// Nil dependencies make any attempt to resolve IDs or write content fail.
	runner := &SyncRunner{}
	result, err := runner.processDocument(t.Context(), dao.SyncTaskContext{}, "sharepoint", nil,
		connector.SourceDocument{SourceID: "drive:file", SkipReason: "unsupported file type", FetchRef: &connector.FetchReference{Key: "unused"}})
	if err != nil || result.Action != service.DocumentActionSkipped {
		t.Fatalf("skip result = %+v, %v", result, err)
	}
}

func TestSyncFileFailureIncludesIdentity(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	taskDAO := dao.NewSyncTaskDAO(db)
	task, err := taskDAO.GetTaskContext(t.Context(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	runner := NewSyncRunner(SyncRunnerConfig{ItemRetryCount: 1}, taskDAO, nil,
		&fakeSink{errBySourceID: map[string]error{"drive:file": io.ErrUnexpectedEOF}},
		service.NewDocumentIDResolver(fakeStore{}), nil, nil)
	_, err = runner.processDocuments(t.Context(), task, "sharepoint", nil,
		[]connector.SourceDocument{{SourceID: "drive:file", SemanticIdentifier: "Проект.docx", Blob: []byte("file")}})
	if !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(err.Error(), "Проект.docx") || !strings.Contains(err.Error(), "drive:file") {
		t.Fatalf("missing file identity or original error: %v", err)
	}
}
