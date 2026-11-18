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

package file

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

func TestCheckUploadInfoHealthEnforcesUserQuotaForTenantCreatedDocuments(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("CREATE TABLE document (id TEXT PRIMARY KEY, kb_id TEXT NOT NULL, created_by TEXT NOT NULL, user_id TEXT NOT NULL)").Error; err != nil {
		t.Fatalf("create document table: %v", err)
	}
	if err := db.Exec("INSERT INTO document (id, kb_id, created_by, user_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
		"doc-1", "kb-1", "tenant-1", "user-1",
		"doc-2", "kb-1", "tenant-1", "user-1",
	).Error; err != nil {
		t.Fatalf("insert documents: %v", err)
	}
	oldDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = oldDB })
	t.Setenv(common.EnvMaxFileNumPerUser, "2")

	ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-1")
	err = testFileService().checkUploadInfoHealth(ctx, "upload.txt")
	if err == nil || !strings.Contains(err.Error(), "exceed the maximum file number") {
		t.Fatalf("checkUploadInfoHealth() error = %v, want quota rejection", err)
	}
}
