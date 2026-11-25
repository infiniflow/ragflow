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

package dataset

import (
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

func TestDeleteDatasetsBatchesDatasetQueries(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	store := storage.NewMemoryStorage()
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	ids := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("123e4567e89b12d3a456%012d", 426614174100+i)
		ids = append(ids, id)
		insertDatasetUpdateKB(t, id, "delete-owner", "Dataset")
	}
	ctx := withDatasetTestContext(t.Context(), "delete-owner", "delete-owner")

	capture := &datasetBatchSQLCapture{}
	dao.DB = db.Session(&gorm.Session{Logger: capture})

	result, code, err := testDatasetUpdateService(t).DeleteDatasets(ctx, ids, false)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("DeleteDatasets: code=%d err=%v", code, err)
	}
	if result["success_count"] != len(ids) {
		t.Fatalf("success_count = %v, want %d", result["success_count"], len(ids))
	}
	if n := capture.knowledgebaseSelects(); n > 2 {
		t.Fatalf("knowledgebase SELECTs = %d for %d datasets, want <= 2 (batch); got SQL: %v", n, len(ids), capture.sqls)
	}
}

// Adversarial: an unauthorized dataset is excluded from deletion and reported
// as forbidden even when the authorized IDs are loaded in one batch.
func TestDeleteDatasetsRejectsUnauthorizedIDInBatch(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	store := storage.NewMemoryStorage()
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("123e4567e89b12d3a456%012d", 426614174200+i)
		ids = append(ids, id)
		insertDatasetUpdateKB(t, id, "delete-owner", "Dataset")
	}
	foreign := "923e4567e89b12d3a456426614174999"
	insertDatasetUpdateKB(t, foreign, "other-tenant", "Foreign")
	ctx := withDatasetTestContext(t.Context(), "delete-owner", "delete-owner")

	_, code, err := testDatasetUpdateService(t).DeleteDatasets(ctx, append(ids, foreign), false)
	if code != common.CodeForbidden {
		t.Fatalf("code = %d, want CodeForbidden", code)
	}
	if err == nil {
		t.Fatal("expected forbidden error for foreign dataset")
	}
}
