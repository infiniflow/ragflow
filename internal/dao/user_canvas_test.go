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

	ctx := t.Context()
	dao := NewUserCanvasDAO()
	originalDSL := entity.JSONMap{"graph": map[string]interface{}{"nodes": []interface{}{"old"}}}
	if err := dao.Create(ctx, db, &entity.UserCanvas{
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
	rows, err := dao.UpdateDSL(ctx, db, "canvas-1", newDSL)
	if err != nil {
		t.Fatalf("UpdateDSL failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row affected, got %d", rows)
	}

	canvas, err := dao.GetByID(ctx, db, "canvas-1")
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
	ctx := t.Context()
	dao := NewUserCanvasDAO()
	originalDSL := entity.JSONMap{"path": []interface{}{"old"}}
	if err := dao.Create(ctx, db, &entity.UserCanvas{
		ID:             "canvas-1",
		UserID:         "user-1",
		Title:          stringPtr("Test Canvas"),
		CanvasCategory: "agent_canvas",
		DSL:            originalDSL,
	}); err != nil {
		t.Fatalf("failed to create canvas: %v", err)
	}

	rows, err := dao.UpdateDSL(ctx, db, "missing-canvas", entity.JSONMap{"path": []interface{}{"new"}})
	if err != nil {
		t.Fatalf("UpdateDSL failed: %v", err)
	}
	if rows != 0 {
		t.Fatalf("expected 0 rows affected, got %d", rows)
	}

	canvas, err := dao.GetByID(ctx, db, "canvas-1")
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

func TestUserCanvasDAOListTagsIncludesPipelineWhenCategoryIsEmpty(t *testing.T) {
	db := setupUserCanvasTestDB(t)
	pushDB(t, db)

	dao := NewUserCanvasDAO()
	pipelineType := "pipeline"
	rows := []*entity.UserCanvas{
		{
			ID:             "agent-canvas",
			UserID:         "user-1",
			Title:          stringPtr("Agent Canvas"),
			Permission:     "me",
			CanvasCategory: "agent_canvas",
			Tags:           "agent-tag,shared",
		},
		{
			ID:             "pipeline-canvas",
			UserID:         "user-1",
			Title:          stringPtr("Pipeline Canvas"),
			Permission:     "me",
			CanvasCategory: "dataflow_canvas",
			CanvasType:     &pipelineType,
			Tags:           "pipeline-tag,shared",
		},
	}
	ctx := t.Context()
	for _, row := range rows {
		if err := dao.Create(ctx, db, row); err != nil {
			t.Fatalf("failed to create canvas %s: %v", row.ID, err)
		}
	}

	counts, err := dao.ListTags(ctx, db, []string{"user-1"}, "user-1", "")
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}
	if counts["agent-tag"] != 1 {
		t.Fatalf("agent-tag count = %d, want 1", counts["agent-tag"])
	}
	if counts["pipeline-tag"] != 1 {
		t.Fatalf("pipeline-tag count = %d, want 1", counts["pipeline-tag"])
	}
	if counts["shared"] != 2 {
		t.Fatalf("shared count = %d, want 2", counts["shared"])
	}

	counts, err = dao.ListTags(ctx, db, []string{"user-1"}, "user-1", "agent_canvas")
	if err != nil {
		t.Fatalf("ListTags with category failed: %v", err)
	}
	if counts["pipeline-tag"] != 0 {
		t.Fatalf("pipeline-tag count with agent_canvas filter = %d, want 0", counts["pipeline-tag"])
	}
	if counts["agent-tag"] != 1 {
		t.Fatalf("agent-tag count with agent_canvas filter = %d, want 1", counts["agent-tag"])
	}
}

func TestUserCanvasDAOOwnerAndCategoryFilters(t *testing.T) {
	db := setupUserCanvasTestDB(t)
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("failed to migrate user: %v", err)
	}
	pushDB(t, db)
	ctx := t.Context()
	d := NewUserCanvasDAO()

	users := []entity.User{
		{ID: "user-1", Nickname: "Alice", Email: "alice@example.com"},
		{ID: "user-2", Nickname: "Bob", Email: "bob@example.com"},
	}
	for i := range users {
		if err := db.WithContext(ctx).Create(&users[i]).Error; err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
	}

	canvases := []entity.UserCanvas{
		{ID: "c1", UserID: "user-1", Permission: "me", CanvasCategory: "agent_canvas"},
		{ID: "c2", UserID: "user-1", Permission: "me", CanvasCategory: "dataflow_canvas"},
		{ID: "c3", UserID: "user-2", Permission: "team", CanvasCategory: "agent_canvas"},
		{ID: "c4", UserID: "user-2", Permission: "me", CanvasCategory: "agent_canvas"},
	}
	for i := range canvases {
		if err := db.WithContext(ctx).Create(&canvases[i]).Error; err != nil {
			t.Fatalf("failed to create canvas: %v", err)
		}
	}

	ownerIDs := []string{"user-1", "user-2"}

	owners, err := d.GetOwnerFilter(ctx, db, ownerIDs, "user-1")
	if err != nil {
		t.Fatalf("GetOwnerFilter failed: %v", err)
	}
	if len(owners) != 2 {
		t.Fatalf("owner filter rows = %d, want 2", len(owners))
	}
	byID := make(map[string]*OwnerFilterItem, len(owners))
	for _, o := range owners {
		byID[o.ID] = o
	}
	// user-1 sees both of their own canvases; user-2 only the team one (c4 is "me").
	if byID["user-1"].Count != 2 {
		t.Fatalf("user-1 count = %d, want 2", byID["user-1"].Count)
	}
	if byID["user-2"].Count != 1 {
		t.Fatalf("user-2 count = %d, want 1", byID["user-2"].Count)
	}
	if byID["user-1"].Label == nil || *byID["user-1"].Label != "Alice" {
		t.Fatalf("user-1 label = %v, want Alice", byID["user-1"].Label)
	}

	categories, err := d.GetCategoryFilter(ctx, db, ownerIDs, "user-1")
	if err != nil {
		t.Fatalf("GetCategoryFilter failed: %v", err)
	}
	catByID := make(map[string]int64, len(categories))
	for _, c := range categories {
		catByID[c.ID] = c.Count
	}
	// agent_canvas: c1 (own) + c3 (team); c4 hidden. dataflow_canvas: c2.
	if catByID["agent_canvas"] != 2 {
		t.Fatalf("agent_canvas count = %d, want 2", catByID["agent_canvas"])
	}
	if catByID["dataflow_canvas"] != 1 {
		t.Fatalf("dataflow_canvas count = %d, want 1", catByID["dataflow_canvas"])
	}
}

// TestUserCanvasDAOKeywordSearchIncludesTags verifies that the keyword
// search matches agents by tag in addition to title (issue #14774:
// "Tag metadata should be searchable across the Agent list").
func TestUserCanvasDAOKeywordSearchIncludesTags(t *testing.T) {
	db := setupUserCanvasTestDB(t)
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("failed to migrate user: %v", err)
	}
	pushDB(t, db)
	ctx := t.Context()
	d := NewUserCanvasDAO()

	if err := db.Create(&entity.User{ID: "u1", Nickname: "Owner", Email: "o@example.com"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	canvases := []entity.UserCanvas{
		{ID: "c1", UserID: "u1", Permission: "me", CanvasCategory: "agent_canvas", Title: stringPtr("Budget Report"), Tags: "finance,budget"},
		{ID: "c2", UserID: "u1", Permission: "me", CanvasCategory: "agent_canvas", Title: stringPtr("Sales Bot"), Tags: "customer-support"},
	}
	for i := range canvases {
		if err := db.Create(&canvases[i]).Error; err != nil {
			t.Fatalf("create canvas: %v", err)
		}
	}

	results, _, err := d.ListByTenantIDs(ctx, db, []string{"u1"}, "u1", 1, 10, "create_time", false, "finance", "", "", nil)
	if err != nil {
		t.Fatalf("ListByTenantIDs: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("keyword search 'finance' returned %d rows, want 1 (matched by tag not title)", len(results))
	}
	if results[0].ID != "c1" {
		t.Errorf("matched canvas id = %s, want c1", results[0].ID)
	}
}

// TestUserCanvasDAOOrderByTags verifies that agents can be sorted by
// their tags column (issue #14774: "Agents can be sorted by tag").
func TestUserCanvasDAOOrderByTags(t *testing.T) {
	db := setupUserCanvasTestDB(t)
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("failed to migrate user: %v", err)
	}
	pushDB(t, db)
	ctx := t.Context()
	d := NewUserCanvasDAO()

	if err := db.Create(&entity.User{ID: "u1", Nickname: "Owner", Email: "o@example.com"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	canvases := []entity.UserCanvas{
		{ID: "c-z", UserID: "u1", Permission: "me", CanvasCategory: "agent_canvas", Title: stringPtr("Zeta"), Tags: "zebra"},
		{ID: "c-a", UserID: "u1", Permission: "me", CanvasCategory: "agent_canvas", Title: stringPtr("Alpha"), Tags: "alpha"},
		{ID: "c-m", UserID: "u1", Permission: "me", CanvasCategory: "agent_canvas", Title: stringPtr("Mid"), Tags: "middle"},
	}
	for i := range canvases {
		if err := db.Create(&canvases[i]).Error; err != nil {
			t.Fatalf("create canvas: %v", err)
		}
	}

	results, _, err := d.ListByTenantIDs(ctx, db, []string{"u1"}, "u1", 1, 10, "tags", false, "", "", "", nil)
	if err != nil {
		t.Fatalf("ListByTenantIDs: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("returned %d rows, want 3", len(results))
	}
	want := []string{"c-a", "c-m", "c-z"}
	for i, r := range results {
		if r.ID != want[i] {
			t.Errorf("row[%d] id = %s, want %s (ascending tag sort)", i, r.ID, want[i])
		}
	}
}
