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
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission/testsupport"

	"gorm.io/gorm"
)

func noopTagLoader(context.Context, string, string) (map[string]int, error) {
	return nil, nil
}

func TestAggregateTagsBatchesDatasetQueries(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ids := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		rawID := fmt.Sprintf("123e4567-e89b-12d3-a456-%012d", 426614174000+i)
		ids = append(ids, rawID)
		insertAggregateTagsKB(t, strings.ReplaceAll(rawID, "-", ""), "tag-owner", string(entity.TenantPermissionMe), "", 0)
	}
	ctx := withDatasetTestContext(t.Context(), "tag-owner", "tag-owner")

	capture := &datasetBatchSQLCapture{}
	dao.DB = db.Session(&gorm.Session{Logger: capture})

	_, code, err := testDatasetServiceForAggregateTags(t, noopTagLoader).AggregateTags(ctx, ids)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("AggregateTags: code=%d err=%v", code, err)
	}
	if n, limit := capture.knowledgebaseSelects(), 1+testsupport.DatasetPermissionQueryCount(len(ids)); n > limit {
		t.Fatalf("knowledgebase SELECTs = %d for %d datasets, want <= %d; got SQL: %v", n, len(ids), limit, capture.sqls)
	}
}

// Adversarial: an inaccessible dataset among valid ones is rejected with the
// exact code and message, regardless of how datasets are loaded in a batch.
func TestAggregateTagsRejectsUnauthorizedDatasetInBatch(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		rawID := fmt.Sprintf("123e4567-e89b-12d3-a456-%012d", 426614174100+i)
		ids = append(ids, rawID)
		insertAggregateTagsKB(t, strings.ReplaceAll(rawID, "-", ""), "tag-owner", string(entity.TenantPermissionMe), "", 0)
	}
	foreign := "923e4567-e89b-12d3-a456-426614174999"
	foreignID := strings.ReplaceAll(foreign, "-", "")
	// A different owner in the same tenant exercises a denial in both editions.
	insertAggregateTagsKB(t, foreignID, "tag-owner", string(entity.TenantPermissionMe), "", 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", foreignID).
		Updates(map[string]any{"user_id": "other-user", "created_by": "other-user"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := withDatasetTestContext(t.Context(), "tag-owner", "tag-owner")

	_, code, err := testDatasetServiceForAggregateTags(t, noopTagLoader).AggregateTags(ctx, []string{ids[0], foreign, ids[1]})
	if code != common.CodeForbidden {
		t.Fatalf("code = %d, want CodeForbidden", code)
	}
	if err == nil || err.Error() != "Permission denied" {
		t.Fatalf("error = %v, want Permission denied", err)
	}
}
