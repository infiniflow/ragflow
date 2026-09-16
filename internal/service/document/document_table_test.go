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

func TestSaveKBTableFieldMap_MergesFieldMap(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	status := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:     "kb-1",
		Status: &status,
		ParserConfig: entity.JSONMap{
			"field_map": map[string]interface{}{
				"existing_col": "existing label",
			},
		},
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}

	svc := testDocumentService(t)
	ctx := t.Context()

	newFields := map[string]interface{}{
		"new_col": "new label",
	}
	if err := svc.SaveKBTableFieldMap(ctx, "kb-1", newFields); err != nil {
		t.Fatalf("SaveKBTableFieldMap: %v", err)
	}

	var updated entity.Knowledgebase
	if err := db.First(&updated, "id = ?", "kb-1").Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}

	fm, ok := updated.ParserConfig["field_map"].(map[string]interface{})
	if !ok {
		t.Fatalf("field_map missing or invalid type: %#v", updated.ParserConfig)
	}

	if fm["existing_col"] != "existing label" {
		t.Errorf("existing_col should be preserved, got %v", fm["existing_col"])
	}
	if fm["new_col"] != "new label" {
		t.Errorf("new_col should be merged, got %v", fm["new_col"])
	}
}
