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

package document

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupProgressMsgTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Document{}); err != nil {
		t.Fatalf("migrate document: %v", err)
	}
	previous := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previous })
	return db
}

func TestUpdateProgressMsgPersistsTerminalReason(t *testing.T) {
	db := setupProgressMsgTestDB(t)
	if err := db.Create(&entity.Document{ID: "doc-1", ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}

	svc := &DocumentService{documentDAO: dao.NewDocumentDAO()}
	if err := svc.UpdateProgressMsg(context.Background(), "doc-1", "Task failed: boom"); err != nil {
		t.Fatalf("UpdateProgressMsg: %v", err)
	}

	var got entity.Document
	if err := db.First(&got, "id = ?", "doc-1").Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if got.ProgressMsg == nil || *got.ProgressMsg != "Task failed: boom" {
		t.Fatalf("progress_msg = %v, want %q", got.ProgressMsg, "Task failed: boom")
	}
}

func TestUpdateProgressMsgEmptyMessageIsNoop(t *testing.T) {
	// Adversarial: an empty terminal message must not wipe a seeded progress
	// message (a blank terminal reason carries no information to persist).
	db := setupProgressMsgTestDB(t)
	seed := "seeded progress"
	if err := db.Create(&entity.Document{ID: "doc-2", ProgressMsg: &seed, ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}

	svc := &DocumentService{documentDAO: dao.NewDocumentDAO()}
	if err := svc.UpdateProgressMsg(context.Background(), "doc-2", ""); err != nil {
		t.Fatalf("UpdateProgressMsg: %v", err)
	}

	var got entity.Document
	if err := db.First(&got, "id = ?", "doc-2").Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if got.ProgressMsg == nil || *got.ProgressMsg != seed {
		t.Fatalf("progress_msg = %v, want unchanged %q", got.ProgressMsg, seed)
	}
}
