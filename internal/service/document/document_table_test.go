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
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

func TestSaveDocumentTableColumns_PersistsAndFiltersRoles(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	doc := &entity.Document{
		ID:       "doc-1",
		KbID:     "kb-1",
		ParserID: "table",
		ParserConfig: entity.JSONMap{
			"table_column_mode": "manual",
			"table_column_roles": map[string]interface{}{
				"Name":  "metadata",
				"Stale": "both",
			},
		},
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	svc := testDocumentService(t)
	ctx := t.Context()

	if err := svc.SaveDocumentTableColumns(ctx, "doc-1", []string{"Name", "City"}); err != nil {
		t.Fatalf("SaveDocumentTableColumns: %v", err)
	}

	var updated entity.Document
	if err := db.First(&updated, "id = ?", "doc-1").Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}

	wantNames := []interface{}{"Name", "City"}
	if !reflect.DeepEqual(updated.ParserConfig["table_column_names"], wantNames) {
		t.Fatalf("got names %#v, want %#v", updated.ParserConfig["table_column_names"], wantNames)
	}

	wantRoles := map[string]interface{}{"Name": "metadata"}
	if !reflect.DeepEqual(updated.ParserConfig["table_column_roles"], wantRoles) {
		t.Fatalf("got roles %#v, want %#v (Stale must be filtered)", updated.ParserConfig["table_column_roles"], wantRoles)
	}
}

func TestSaveKBTableState_ReplacesSchemaAndKeepsFieldMapWhenNotOwned(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	status := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:     "kb-1",
		Status: &status,
		ParserConfig: entity.JSONMap{
			"field_map": map[string]interface{}{
				"stale_col": "stale label",
			},
			"table_column_names": []interface{}{"stale_col"},
			"dataset_setting":    "preserved",
		},
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}

	svc := testDocumentService(t)
	ctx := t.Context()

	// The discovered schema is the file's current schema: names and field_map
	// are replaced, not merged, so a dropped column stops being offered.
	if err := svc.SaveKBTableState(ctx, "kb-1", []string{"new_col", "new_col", " "}, map[string]interface{}{
		"new_col": "new label",
	}); err != nil {
		t.Fatalf("SaveKBTableState: %v", err)
	}

	var updated entity.Knowledgebase
	if err := db.First(&updated, "id = ?", "kb-1").Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}

	fm, ok := updated.ParserConfig["field_map"].(map[string]interface{})
	if !ok {
		t.Fatalf("field_map missing or invalid type: %#v", updated.ParserConfig)
	}
	if _, ok := fm["stale_col"]; ok {
		t.Errorf("stale_col must be dropped from field_map, got %#v", fm)
	}
	if fm["new_col"] != "new label" {
		t.Errorf("new_col must be written, got %#v", fm)
	}
	names, ok := updated.ParserConfig["table_column_names"].([]interface{})
	if !ok || !reflect.DeepEqual(names, []interface{}{"new_col"}) {
		t.Errorf("table_column_names = %#v, want [new_col]", updated.ParserConfig["table_column_names"])
	}
	if updated.ParserConfig["dataset_setting"] != "preserved" {
		t.Errorf("unrelated parser_config keys must be preserved: %#v", updated.ParserConfig)
	}
}

// A run on an engine without a chunk_data column publishes only the discovered
// names and must leave the dataset's field_map untouched.
func TestSaveKBTableState_NilFieldMapLeavesFieldMapAlone(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	status := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:     "kb-1",
		Status: &status,
		ParserConfig: entity.JSONMap{
			"field_map": map[string]interface{}{
				"python_written_col": "python written label",
			},
		},
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}

	svc := testDocumentService(t)
	if err := svc.SaveKBTableState(t.Context(), "kb-1", []string{"name"}, nil); err != nil {
		t.Fatalf("SaveKBTableState: %v", err)
	}

	var updated entity.Knowledgebase
	if err := db.First(&updated, "id = ?", "kb-1").Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}
	fm, ok := updated.ParserConfig["field_map"].(map[string]interface{})
	if !ok || fm["python_written_col"] != "python written label" {
		t.Fatalf("field_map must be untouched, got %#v", updated.ParserConfig["field_map"])
	}
	names, ok := updated.ParserConfig["table_column_names"].([]interface{})
	if !ok || !reflect.DeepEqual(names, []interface{}{"name"}) {
		t.Errorf("table_column_names = %#v, want [name]", updated.ParserConfig["table_column_names"])
	}
}
