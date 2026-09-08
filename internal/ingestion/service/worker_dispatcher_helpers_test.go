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
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"gorm.io/gorm"
)

func seedBurstTasks(t *testing.T, db *gorm.DB, n int) []string {
	t.Helper()
	const tenantID, kbID = "burst-tenant", "burst-kb"
	if err := db.Create(&entity.Tenant{ID: tenantID, LLMID: "gpt-4", Status: testutil.StrPtr("1")}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID: kbID, TenantID: tenantID, EmbdID: "embd-1", Status: testutil.StrPtr("1"), ParserConfig: entity.JSONMap{},
	}).Error; err != nil {
		t.Fatalf("create knowledgebase: %v", err)
	}

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		docID := fmt.Sprintf("burst-doc-%d", i)
		taskID := fmt.Sprintf("burst-task-%d", i)
		location := "doc_store/" + docID
		if err := db.Create(&entity.Document{
			ID: docID, KbID: kbID, Name: &docID, ParserID: "naive", ParserConfig: entity.JSONMap{},
			PipelineID: testutil.StrPtr("flow-1"), Status: testutil.StrPtr("1"), Type: "pdf", Location: &location,
		}).Error; err != nil {
			t.Fatalf("create document %s: %v", docID, err)
		}
		if err := db.Create(&entity.IngestionTask{
			ID: taskID, UserID: "u1", DocumentID: docID, DatasetID: kbID, Status: common.CREATED,
		}).Error; err != nil {
			t.Fatalf("create ingestion task %s: %v", taskID, err)
		}
		ids = append(ids, taskID)
	}
	return ids
}
