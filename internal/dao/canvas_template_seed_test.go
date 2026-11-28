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
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSeedCanvasTemplatesIsIdempotentAndRemovesStaleRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.CanvasTemplate{}); err != nil {
		t.Fatalf("migrate canvas templates: %v", err)
	}

	dir := t.TempDir()
	writeTemplate := func(name, id, title string) {
		t.Helper()
		body := `{"id":"` + id + `","title":{"en":"` + title + `"},"description":{},"dsl":{}}`
		if err = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write template: %v", err)
		}
	}
	writeTemplate("kept.json", "kept", "first")
	writeTemplate("removed.json", "removed", "removed")

	seed := func() {
		t.Helper()
		ctx := t.Context()
		var entries []os.DirEntry
		entries, err = os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read templates: %v", err)
		}
		if _, err = seedCanvasTemplates(ctx, db, dir, entries); err != nil {
			t.Fatalf("seed templates: %v", err)
		}
	}
	seed()

	writeTemplate("kept.json", "kept", "updated")
	if err = os.Remove(filepath.Join(dir, "removed.json")); err != nil {
		t.Fatalf("remove stale template file: %v", err)
	}
	seed()

	var templates []entity.CanvasTemplate
	if err = db.Find(&templates).Error; err != nil {
		t.Fatalf("read seeded templates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(templates))
	}
	if templates[0].ID != "kept" || templates[0].Title["en"] != "updated" {
		t.Fatalf("unexpected template after reseed: %#v", templates[0])
	}
}

func TestSeedWorkflowTemplatesRemovesIngestionTemplates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.CanvasTemplate{}); err != nil {
		t.Fatalf("migrate canvas templates: %v", err)
	}
	for _, id := range []string{"23", "24", "25", "33"} {
		if err := db.Create(&entity.CanvasTemplate{ID: id, CanvasCategory: "dataflow_canvas"}).Error; err != nil {
			t.Fatalf("seed ingestion template %s: %v", id, err)
		}
	}

	dir := findAgentTemplatesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read workflow templates: %v", err)
	}
	for range 2 {
		if _, err := seedCanvasTemplates(t.Context(), db, dir, entries); err != nil {
			t.Fatalf("seed workflow templates: %v", err)
		}
	}

	var templates []entity.CanvasTemplate
	if err := db.Find(&templates).Error; err != nil {
		t.Fatalf("read seeded templates: %v", err)
	}
	if len(templates) == 0 {
		t.Fatal("workflow templates were not seeded")
	}
	for _, tmpl := range templates {
		if tmpl.CanvasCategory != "agent_canvas" {
			t.Errorf("template %s has category %q, want agent_canvas", tmpl.ID, tmpl.CanvasCategory)
		}
		if tmpl.ID == "23" || tmpl.ID == "24" || tmpl.ID == "25" || tmpl.ID == "33" {
			t.Errorf("ingestion template %s remains in canvas_template", tmpl.ID)
		}
	}
}
