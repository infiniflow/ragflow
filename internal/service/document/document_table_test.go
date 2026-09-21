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

// The upload path shares this filter with the post-ingestion state save: both
// narrow the dataset's roles to the columns one file actually has. Anything that
// is not a role map collapses to an empty map, which the resolvers read the same
// way as an absent key.
func TestFilterTableColumnRoles(t *testing.T) {
	columns := map[string]struct{}{"Name": {}, "City": {}}
	tests := []struct {
		name string
		raw  any
		want map[string]interface{}
	}{
		{"object", map[string]interface{}{"Name": "metadata", "Stale": "both"}, map[string]interface{}{"Name": "metadata"}},
		{"absent", nil, map[string]interface{}{}},
		{"not an object", "manual", map[string]interface{}{}},
	}
	for _, tt := range tests {
		if got := filterTableColumnRoles(tt.raw, columns); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: filterTableColumnRoles(%#v) = %#v, want %#v", tt.name, tt.raw, got, tt.want)
		}
	}
}

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
			// The parser component entry is the canvas DSL's projection, so a
			// run's discovered schema must not be written into it: that would
			// turn system output into a claim the canvas author never made.
			"Parser:HipSignsRhyme": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"column_mode":  "manual",
					"column_roles": map[string]interface{}{"Name": "indexing"},
				},
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

	component, _ := updated.ParserConfig["Parser:HipSignsRhyme"].(map[string]interface{})
	spreadsheet, _ := component["spreadsheet"].(map[string]interface{})
	if _, ok := spreadsheet["column_names"]; ok {
		t.Fatalf("column_names = %#v, want discovery left out of the component entry", spreadsheet["column_names"])
	}
	if !reflect.DeepEqual(spreadsheet["column_roles"], map[string]interface{}{"Name": "indexing"}) {
		t.Fatalf("column_roles = %#v, want the canvas entry untouched", spreadsheet["column_roles"])
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
	// A blank name survives: it is a real column of a delimited file, and the
	// published schema has to name the columns exactly as the run indexed them
	// (rag/app/table.py:590-595).
	if !ok || !reflect.DeepEqual(names, []interface{}{"new_col", " "}) {
		t.Errorf("table_column_names = %#v, want [new_col, ' ']", updated.ParserConfig["table_column_names"])
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

// A document parser_config update rebuilds the configuration from the pipeline
// DSL, which only carries component-scoped entries. The root-level table schema
// this document already carries — the columns the last run discovered, and the
// roles configured against them — has to survive that write, or the next parse
// silently reverts to the canvas' auto mode.
func TestUpdateDatasetDocumentKeepsPublishedTableSchema(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update("parser_id", "table").Error; err != nil {
		t.Fatalf("seed table dataset: %v", err)
	}
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("parser_config", entity.JSONMap{
		"table_column_mode":  "manual",
		"table_column_names": []interface{}{"Name", "City"},
		"table_column_roles": map[string]interface{}{"Name": "metadata"},
	}).Error; err != nil {
		t.Fatalf("seed published table schema: %v", err)
	}

	svc := testDocumentService(t)
	_, code, err := svc.UpdateDatasetDocument(t.Context(), "tenant-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{ParserConfig: map[string]any{
			"Parser:HipSignsRhyme": map[string]any{
				"spreadsheet": map[string]any{"output_format": "markdown"},
			},
		}}, map[string]bool{"parser_config": true})
	if err != nil {
		t.Fatalf("UpdateDatasetDocument: code=%v err=%v", code, err)
	}

	var updated entity.Document
	if err := db.First(&updated, "id = ?", "doc-1").Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if updated.ParserConfig["table_column_mode"] != "manual" {
		t.Fatalf("table_column_mode = %#v, want manual", updated.ParserConfig["table_column_mode"])
	}
	names, ok := updated.ParserConfig["table_column_names"].([]interface{})
	if !ok || !reflect.DeepEqual(names, []interface{}{"Name", "City"}) {
		t.Fatalf("table_column_names = %#v, want [Name City]", updated.ParserConfig["table_column_names"])
	}
	roles, ok := updated.ParserConfig["table_column_roles"].(map[string]interface{})
	if !ok || roles["Name"] != "metadata" {
		t.Fatalf("table_column_roles = %#v, want the stored metadata role", updated.ParserConfig["table_column_roles"])
	}
	spreadsheet, ok := updated.ParserConfig["Parser:HipSignsRhyme"].(map[string]any)["spreadsheet"].(map[string]any)
	if !ok || spreadsheet["output_format"] != "markdown" {
		t.Fatalf("requested spreadsheet override lost: %#v", updated.ParserConfig["Parser:HipSignsRhyme"])
	}
}

// A document update whose pipeline DSL cannot be loaded merges the request onto
// the stored configuration instead of rebuilding it from the canvas. The merge
// keeps keys the request never mentions, so the root column settings survive
// without the normalization a rebuild needs.
func TestUpdateDocumentParserConfig_MergeKeepsRootColumnSettings(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	doc := &entity.Document{
		ID:       "doc-1",
		KbID:     "kb-1",
		ParserID: "table",
		ParserConfig: entity.JSONMap{
			"chunk_token_num":    float64(128),
			"table_column_mode":  "manual",
			"table_column_names": []interface{}{"Name", "City"},
			"table_column_roles": map[string]interface{}{"Name": "metadata"},
		},
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	svc := testDocumentService(t)
	if err := svc.updateDocumentParserConfig(t.Context(), "doc-1", map[string]any{
		"chunk_token_num": float64(256),
	}); err != nil {
		t.Fatalf("updateDocumentParserConfig: %v", err)
	}

	var updated entity.Document
	if err := db.First(&updated, "id = ?", "doc-1").Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if updated.ParserConfig["chunk_token_num"] != float64(256) {
		t.Fatalf("chunk_token_num = %#v, want the requested 256", updated.ParserConfig["chunk_token_num"])
	}
	if updated.ParserConfig["table_column_mode"] != "manual" {
		t.Fatalf("table_column_mode = %#v, want manual", updated.ParserConfig["table_column_mode"])
	}
	if names, ok := updated.ParserConfig["table_column_names"].([]interface{}); !ok || len(names) != 2 {
		t.Fatalf("table_column_names = %#v, want the published schema kept", updated.ParserConfig["table_column_names"])
	}
	roles, ok := updated.ParserConfig["table_column_roles"].(map[string]interface{})
	if !ok || roles["Name"] != "metadata" {
		t.Fatalf("table_column_roles = %#v, want the stored metadata role", updated.ParserConfig["table_column_roles"])
	}
}
