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

package dao

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/entity"
)

func setupPipelineDSLVersionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "pipeline_dsl_version.db")
	dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.PipelineDSLVersion{}); err != nil {
		t.Fatalf("auto-migrate pipeline DSL versions: %v", err)
	}
	return db
}

func TestPipelineDSLVersionDAOGetOrCreate(t *testing.T) {
	db := setupPipelineDSLVersionTestDB(t)
	versionDAO := NewPipelineDSLVersionDAO()

	first, err := versionDAO.GetOrCreate(t.Context(), db, "canvas:pipeline-1", entity.JSONMap{
		"components": map[string]any{"parser": map[string]any{"revision": 1}},
		"path":       []any{"parser"},
	})
	if err != nil {
		t.Fatalf("create first version: %v", err)
	}
	if first.Version != 1 {
		t.Fatalf("first version = %d, want 1", first.Version)
	}

	// Map order and the int-to-float64 conversion performed by JSON decoding
	// must not create a new version for the same JSON definition.
	reused, err := versionDAO.GetOrCreate(t.Context(), db, "canvas:pipeline-1", entity.JSONMap{
		"path":       []any{"parser"},
		"components": map[string]any{"parser": map[string]any{"revision": float64(1)}},
	})
	if err != nil {
		t.Fatalf("reuse first version: %v", err)
	}
	if reused.Version != 1 {
		t.Fatalf("reused version = %d, want 1", reused.Version)
	}

	second, err := versionDAO.GetOrCreate(t.Context(), db, "canvas:pipeline-1", entity.JSONMap{
		"components": map[string]any{"parser": map[string]any{"revision": 2}},
		"path":       []any{"parser"},
	})
	if err != nil {
		t.Fatalf("create changed version: %v", err)
	}
	if second.Version != 2 {
		t.Fatalf("changed version = %d, want 2", second.Version)
	}

	third, err := versionDAO.GetOrCreate(t.Context(), db, "canvas:pipeline-1", first.DSL)
	if err != nil {
		t.Fatalf("create reverted version: %v", err)
	}
	if third.Version != 3 {
		t.Fatalf("reverted version = %d, want 3", third.Version)
	}

	var count int64
	if err = db.Model(&entity.PipelineDSLVersion{}).Where("dsl_id = ?", "canvas:pipeline-1").Count(&count).Error; err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if count != 3 {
		t.Fatalf("version count = %d, want 3", count)
	}
}

func TestPipelineDSLVersionDAOConcurrentSameDSL(t *testing.T) {
	db := setupPipelineDSLVersionTestDB(t)
	versionDAO := NewPipelineDSLVersionDAO()
	dsl := entity.JSONMap{"components": map[string]any{"parser": map[string]any{"revision": 1}}}

	const workers = 8
	start := make(chan struct{})
	results := make(chan *entity.PipelineDSLVersion, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			version, err := versionDAO.GetOrCreate(t.Context(), db, "canvas:concurrent", dsl)
			if err != nil {
				errs <- err
				return
			}
			results <- version
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent GetOrCreate: %v", err)
		}
	}
	for version := range results {
		if version.Version != 1 {
			t.Fatalf("concurrent version = %d, want 1", version.Version)
		}
	}

	var count int64
	if err := db.Model(&entity.PipelineDSLVersion{}).Where("dsl_id = ?", "canvas:concurrent").Count(&count).Error; err != nil {
		t.Fatalf("count concurrent versions: %v", err)
	}
	if count != 1 {
		t.Fatalf("concurrent version count = %d, want 1", count)
	}
}

func TestPipelineDSLVersionDAORejectsInvalidInput(t *testing.T) {
	db := setupPipelineDSLVersionTestDB(t)
	versionDAO := NewPipelineDSLVersionDAO()

	if _, err := versionDAO.GetOrCreate(t.Context(), db, " ", entity.JSONMap{}); err == nil {
		t.Fatal("empty DSL ID error = nil")
	}
	if _, err := versionDAO.GetOrCreate(t.Context(), db, "canvas:pipeline-1", nil); err == nil {
		t.Fatal("nil DSL error = nil")
	}
	if _, err := versionDAO.GetOrCreate(t.Context(), db, "canvas:pipeline-1", entity.JSONMap{"invalid": make(chan int)}); err == nil {
		t.Fatal("non-serializable DSL error = nil")
	}

	duplicate := &entity.PipelineDSLVersion{DSLID: "canvas:duplicate", Version: 1, DSL: entity.JSONMap{}}
	if err := db.Create(duplicate).Error; err != nil {
		t.Fatalf("seed composite key: %v", err)
	}
	if err := db.Create(duplicate).Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate composite key error = %v, want gorm.ErrDuplicatedKey", err)
	}
}

func TestWaitForPipelineDSLVersionRetryHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := waitForPipelineDSLVersionRetry(ctx, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("retry wait error = %v, want context.Canceled", err)
	}
}
