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

package dao

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/entity"
)

func TestValidateDatasetEmbeddingBindingsRejectsChangedModel(t *testing.T) {
	db := setupKBTestDB(t)
	status := "1"
	kb := &entity.Knowledgebase{ID: "kb", TenantID: "tenant", Name: "kb", Status: &status, EmbdID: "old"}
	if err := db.Create(kb).Error; err != nil {
		t.Fatal(err)
	}
	if err := WithStableDatasetEmbeddingBindings(t.Context(), db, []*entity.Knowledgebase{kb}, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb").Update("embd_id", "new").Error; err != nil {
		t.Fatal(err)
	}
	if err := WithStableDatasetEmbeddingBindings(t.Context(), db, []*entity.Knowledgebase{kb}, func(context.Context) error { return nil }); err == nil {
		t.Fatal("accepted retrieval using a changed model")
	}
}

func TestStableRetrievalReusesItsTransaction(t *testing.T) {
	db := setupKBTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	status := "1"
	kb := &entity.Knowledgebase{ID: "kb", TenantID: "tenant", Name: "kb", Status: &status, EmbdID: "model"}
	if err := db.Create(kb).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err = WithStableDatasetEmbeddingBindings(ctx, db, []*entity.Knowledgebase{kb}, func(ctx context.Context) error {
		var count int64
		return ScopedDB(ctx, db).Model(&entity.Knowledgebase{}).Count(&count).Error
	})
	if err != nil {
		t.Fatalf("nested retrieval lookup could not reuse the lock connection: %v", err)
	}
}
