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
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestListLogsPaginationModes(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.Knowledgebase{}, &entity.SyncLogs{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}

	if err := db.Create(&entity.Connector{
		ID:          "conn-1",
		TenantID:    "user-1",
		Name:        "conn-1",
		Source:      "rss",
		InputType:   "poll",
		Config:      entity.JSONMap{},
		Status:      string(entity.TaskStatusDone),
		RefreshFreq: 5,
		PruneFreq:   5,
		TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:        "kb-1",
		TenantID:  "user-1",
		Name:      "kb-1",
		CreatedBy: "user-1",
		EmbdID:    "embd",
	}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if err := db.Create(&entity.Connector2Kb{
		ID:          "conn-1-kb-1",
		ConnectorID: "conn-1",
		KbID:        "kb-1",
		AutoParse:   "1",
	}).Error; err != nil {
		t.Fatalf("insert connector2kb: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := db.Create(&entity.SyncLogs{
			ID:          fmt.Sprintf("task-%d", i),
			ConnectorID: "conn-1",
			KbID:        "kb-1",
			TaskType:    dao.TaskTypeSync,
			Status:      dao.SyncStatusDone,
			ErrorMsg:    "",
		}).Error; err != nil {
			t.Fatalf("insert sync log %d: %v", i, err)
		}
	}

	svc := NewConnectorService()
	ctx := context.Background()

	all, total, code, err := svc.ListLogs(ctx, "user-1", "", 1, 0)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListLogs(all): code=%v err=%v", code, err)
	}
	if total != 5 || len(all) != 5 {
		t.Fatalf("ListLogs(all) = total %d len %d, want 5/5", total, len(all))
	}

	first, total, code, err := svc.ListLogs(ctx, "user-1", "", 1, 2)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListLogs(page): code=%v err=%v", code, err)
	}
	if total != 5 || len(first) != 2 {
		t.Fatalf("ListLogs(page) = total %d len %d, want 5/2", total, len(first))
	}

	second, total, code, err := svc.ListLogs(ctx, "user-1", "", 2, 2)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListLogs(page2): code=%v err=%v", code, err)
	}
	if total != 5 || len(second) != 2 {
		t.Fatalf("ListLogs(page2) = total %d len %d, want 5/2", total, len(second))
	}

	normalized, total, code, err := svc.ListLogs(ctx, "user-1", "", 0, 101)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListLogs(normalize): code=%v err=%v", code, err)
	}
	if total != 5 || len(normalized) != 5 {
		t.Fatalf("ListLogs(normalize) = total %d len %d, want 5/5", total, len(normalized))
	}
}
