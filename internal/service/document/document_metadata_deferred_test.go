//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package document

import (
	"context"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/service"
)

// deferredMetadataDocEngine wraps metadataDocEngine and records deferred
// writes and refreshes, implementing engine.DeferredMetadataWriter the way the
// Elasticsearch engine does.
type deferredMetadataDocEngine struct {
	*metadataDocEngine
	deferredWrites []string
	refreshCalls   []string
}

func (d *deferredMetadataDocEngine) UpdateMetadataDeferred(ctx context.Context, docID, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	d.deferredWrites = append(d.deferredWrites, docID)
	return d.metadataDocEngine.UpdateMetadata(ctx, docID, datasetID, metaFields, tenantID)
}

func (d *deferredMetadataDocEngine) RefreshMetadataIndex(ctx context.Context, tenantID string) error {
	d.refreshCalls = append(d.refreshCalls, tenantID)
	return nil
}

var _ engine.DeferredMetadataWriter = (*deferredMetadataDocEngine)(nil)

func TestSetDocumentMetadataDeferredUsesDeferredWriter(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 0, 0)

	engineImpl := &deferredMetadataDocEngine{metadataDocEngine: newMetadataDocEngine(nil, map[string]string{})}
	svc := testDocumentService(t)
	svc.docEngine = engineImpl
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), engineImpl)

	ctx := t.Context()
	deferred, err := svc.setDocumentMetadata(ctx, "doc-1", map[string]interface{}{"author": "Alice"}, true)
	if err != nil {
		t.Fatalf("setDocumentMetadata: %v", err)
	}
	if !deferred {
		t.Fatal("setDocumentMetadata(defer=true) did not report a deferred write")
	}
	if len(engineImpl.deferredWrites) != 1 || engineImpl.deferredWrites[0] != "doc-1" {
		t.Fatalf("deferred writes = %v, want [doc-1]", engineImpl.deferredWrites)
	}
	if err := svc.RefreshSyncMetadataIndex(ctx, "tenant-1"); err != nil {
		t.Fatalf("RefreshSyncMetadataIndex: %v", err)
	}
	if len(engineImpl.refreshCalls) != 1 || engineImpl.refreshCalls[0] != "tenant-1" {
		t.Fatalf("refresh calls = %v, want [tenant-1]", engineImpl.refreshCalls)
	}
}

func TestSetDocumentMetadataDeferredFallsBackForNonDeferredEngine(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 0, 0)

	engineImpl := newMetadataDocEngine(nil, map[string]string{})
	svc := testDocumentService(t)
	svc.docEngine = engineImpl
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), engineImpl)

	ctx := t.Context()
	deferred, err := svc.setDocumentMetadata(ctx, "doc-1", map[string]interface{}{"author": "Alice"}, true)
	if err != nil {
		t.Fatalf("setDocumentMetadata: %v", err)
	}
	if deferred {
		t.Fatal("non-deferred engine reported a deferred write")
	}
	if err := svc.RefreshSyncMetadataIndex(ctx, "tenant-1"); err != nil {
		t.Fatalf("RefreshSyncMetadataIndex: %v", err)
	}
}
