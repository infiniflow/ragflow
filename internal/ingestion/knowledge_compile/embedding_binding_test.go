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

package knowledge_compile

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestStableVectorWriteRejectsChangedBinding(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	status := "1"
	if err := db.Create(&entity.Knowledgebase{ID: "kb", TenantID: "tenant", Name: "kb", Status: &status, EmbdID: "new"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := ingestion.WithEmbeddingBinding(t.Context())
	if err := ingestion.RecordEmbeddingBinding(ctx, "old"); err != nil {
		t.Fatal(err)
	}
	consumer := &Consumer{db: db}
	called := false
	write := func() error { called = true; return nil }
	products := []kccommon.Product{{Vector: []float32{1}}}
	if err := consumer.withStableVectorWrite(ctx, "tenant", "kb", products, write); err == nil || called {
		t.Fatalf("stale vector accepted: err=%v called=%v", err, called)
	}
	if err := consumer.withStableVectorWrite(ctx, "tenant", "kb", []kccommon.Product{{}}, write); err != nil || !called {
		t.Fatalf("non-vector product rejected: err=%v called=%v", err, called)
	}
}

func TestCompilerWriteReusesSchedulerTransaction(t *testing.T) {
	scheduler := newSQLTestScheduler(t)
	pool, err := scheduler.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	if err := scheduler.db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	status := "1"
	if err := scheduler.db.Create(&entity.Knowledgebase{ID: "kb", TenantID: "tenant", Name: "kb", Status: &status, EmbdID: "model"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(ingestion.WithEmbeddingBinding(t.Context()), time.Second)
	defer cancel()
	if err := ingestion.RecordEmbeddingBinding(ctx, "model"); err != nil {
		t.Fatal(err)
	}
	consumer := &Consumer{db: scheduler.db, scheduler: scheduler}
	products := []kccommon.Product{{Vector: []float32{1}}}
	err = consumer.withWriteLock(ctx, "kb", "", func(ctx context.Context) error {
		return consumer.withStableVectorWrite(ctx, "tenant", "kb", products, func() error {
			var count int64
			return dao.ScopedDB(ctx, scheduler.db).Model(&entity.Knowledgebase{}).Count(&count).Error
		})
	})
	if err != nil {
		t.Fatalf("compiler write could not reuse scheduler transaction: %v", err)
	}
}

// TestDatasetCompileEmbedsWithDatasetBinding leaves the embedding model to the
// dataset binding, so refreshed Wiki vectors match the dataset's vectors.
func TestDatasetCompileEmbedsWithDatasetBinding(t *testing.T) {
	SetModelConfig("chat")
	t.Cleanup(func() { SetModelConfig(""); kccommon.SetDepsResolver(nil) })
	var embeddingRefs []string
	kccommon.SetDepsResolver(func(_ context.Context, _, llmID, embeddingModel string) (kccommon.Deps, error) {
		embeddingRefs = append(embeddingRefs, embeddingModel)
		return kccommon.Deps{}, nil
	})
	_, _ = defaultDeduperFactory(t.Context(), "tenant")
	refreshWikiProductVectors(t.Context(), "tenant", []kccommon.Product{{Content: "page"}})
	if len(embeddingRefs) != 2 || embeddingRefs[0] != "" || embeddingRefs[1] != "" {
		t.Fatalf("embedding refs = %q, want the dataset binding (empty) for every call", embeddingRefs)
	}
}

// TestBatchForDeletedDatasetCompletes lets a removed dataset's batch be
// acknowledged, while an invalid-status dataset keeps failing for retry.
func TestBatchForDeletedDatasetCompletes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	invalid := "0"
	if err := db.Create(&entity.Knowledgebase{ID: "invalid", TenantID: "tenant", Name: "kb", Status: &invalid}).Error; err != nil {
		t.Fatal(err)
	}
	consumer := &Consumer{db: db}
	entries := []BacklogEntry{{DocID: "doc"}}
	if err := consumer.processBatch(t.Context(), "tenant", "deleted", "token", entries, nil); err != nil {
		t.Fatalf("deleted dataset batch: %v", err)
	}
	if err := consumer.processBatch(t.Context(), "tenant", "invalid", "token", entries, nil); err == nil {
		t.Fatal("invalid-status dataset batch was treated as complete")
	}
}
