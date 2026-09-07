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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupUserCanvasTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.UserCanvas{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestUserCanvasDAOUpdateDSL(t *testing.T) {
	db := setupUserCanvasTestDB(t)
	pushDB(t, db)

	dao := NewUserCanvasDAO()
	originalDSL := entity.JSONMap{"graph": map[string]interface{}{"nodes": []interface{}{"old"}}}
	if err := dao.Create(&entity.UserCanvas{
		ID:             "canvas-1",
		UserID:         "user-1",
		Title:          stringPtr("Test Canvas"),
		CanvasCategory: "agent_canvas",
		DSL:            originalDSL,
	}); err != nil {
		t.Fatalf("failed to create canvas: %v", err)
	}

	newDSL := entity.JSONMap{
		"graph": map[string]interface{}{
			"nodes": []interface{}{"start", "end"},
			"edges": []interface{}{"start:end"},
		},
		"path": []interface{}{"start", "end"},
	}
	rows, err := dao.UpdateDSL("canvas-1", newDSL)
	if err != nil {
		t.Fatalf("UpdateDSL failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row affected, got %d", rows)
	}

	canvas, err := dao.GetByID("canvas-1")
	if err != nil {
		t.Fatalf("failed to get canvas: %v", err)
	}
	graph, ok := canvas.DSL["graph"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected graph map, got %T", canvas.DSL["graph"])
	}
	nodes, ok := graph["nodes"].([]interface{})
	if !ok {
		t.Fatalf("expected nodes slice, got %T", graph["nodes"])
	}
	if len(nodes) != 2 || nodes[0] != "start" || nodes[1] != "end" {
		t.Fatalf("unexpected nodes after update: %v", nodes)
	}
	path, ok := canvas.DSL["path"].([]interface{})
	if !ok {
		t.Fatalf("expected path slice, got %T", canvas.DSL["path"])
	}
	if len(path) != 2 || path[0] != "start" || path[1] != "end" {
		t.Fatalf("unexpected path after update: %v", path)
	}
}

func TestUserCanvasDAOUpdateDSLNoMatch(t *testing.T) {
	db := setupUserCanvasTestDB(t)
	pushDB(t, db)

	dao := NewUserCanvasDAO()
	originalDSL := entity.JSONMap{"path": []interface{}{"old"}}
	if err := dao.Create(&entity.UserCanvas{
		ID:             "canvas-1",
		UserID:         "user-1",
		Title:          stringPtr("Test Canvas"),
		CanvasCategory: "agent_canvas",
		DSL:            originalDSL,
	}); err != nil {
		t.Fatalf("failed to create canvas: %v", err)
	}

	rows, err := dao.UpdateDSL("missing-canvas", entity.JSONMap{"path": []interface{}{"new"}})
	if err != nil {
		t.Fatalf("UpdateDSL failed: %v", err)
	}
	if rows != 0 {
		t.Fatalf("expected 0 rows affected, got %d", rows)
	}

	canvas, err := dao.GetByID("canvas-1")
	if err != nil {
		t.Fatalf("failed to get canvas: %v", err)
	}
	path, ok := canvas.DSL["path"].([]interface{})
	if !ok {
		t.Fatalf("expected path slice, got %T", canvas.DSL["path"])
	}
	if len(path) != 1 || path[0] != "old" {
		t.Fatalf("expected original DSL to remain unchanged, got %v", path)
	}
}
