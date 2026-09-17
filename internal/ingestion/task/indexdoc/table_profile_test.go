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

package indexdoc

import (
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/parser/parser"
)

func TestRoleFor(t *testing.T) {
	tests := []struct {
		name    string
		profile *TableProfile
		col     string
		want    common.ColumnRole
	}{
		{"nil profile defaults to both", nil, "name", common.ColumnRoleBoth},
		{"auto mode defaults every column to both", &TableProfile{Mode: common.TableColumnModeAuto}, "name", common.ColumnRoleBoth},
		{
			"manual mode returns the configured role",
			&TableProfile{Mode: common.TableColumnModeManual, Roles: map[string]common.ColumnRole{"name": common.ColumnRoleIndexing}},
			"name",
			common.ColumnRoleIndexing,
		},
		{
			"manual mode unconfigured column defaults to both",
			&TableProfile{Mode: common.TableColumnModeManual, Roles: map[string]common.ColumnRole{"name": common.ColumnRoleIndexing}},
			"city",
			common.ColumnRoleBoth,
		},
		{
			"an unknown role stays excluded",
			&TableProfile{Mode: common.TableColumnModeManual, Roles: map[string]common.ColumnRole{"name": common.ColumnRoleNone}},
			"name",
			common.ColumnRoleNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := roleFor(tc.profile, tc.col); got != tc.want {
				t.Fatalf("roleFor(%q) = %q, want %q", tc.col, got, tc.want)
			}
		})
	}
}

func TestBuildFieldMap(t *testing.T) {
	profile := &TableProfile{
		Mode: common.TableColumnModeManual,
		Roles: map[string]common.ColumnRole{
			"user_name": common.ColumnRoleIndexing, // indexing only: not addressable
			"user_age":  common.ColumnRoleMetadata,
			"user_city": common.ColumnRoleBoth,
			"user_note": common.ColumnRoleNone, // unknown role: excluded, as Python does
		},
	}

	fm := BuildFieldMap(profile, []string{"user_name", "user_age", "user_city", "user_note", "empty_col", " "})

	if _, ok := fm["user_name"]; ok {
		t.Errorf("indexing-only column must not be in field_map: %v", fm)
	}
	if _, ok := fm["user_note"]; ok {
		t.Errorf("unknown-role column must not be in field_map: %v", fm)
	}
	if fm["user_age"] != "user age" {
		t.Errorf("user_age = %v, want 'user age'", fm["user_age"])
	}
	if fm["user_city"] != "user city" {
		t.Errorf("user_city = %v, want 'user city'", fm["user_city"])
	}
	if fm["empty_col"] != "empty col" {
		t.Errorf("unconfigured column defaults to both: %v", fm["empty_col"])
	}
	if _, ok := fm[" "]; ok {
		t.Errorf("blank column name must be skipped: %v", fm)
	}

	if got := BuildFieldMap(nil, []string{"a"}); got != nil {
		t.Errorf("nil profile must yield nil, got %v", got)
	}
	if got := BuildFieldMap(profile, nil); got != nil {
		t.Errorf("no columns must yield nil, got %v", got)
	}
}

// The row renderer (internal/parser/parser) decides which columns reach the
// chunk body and chunk_data; this package decides which of them the dataset
// field_map and the document metadata may address. Both read the same
// column_roles value, so they must classify every value identically —
// including one the vocabulary does not know, which used to be dropped by the
// renderer while still being advertised in the field_map.
func TestTableColumnRoleClassification_MatchesRenderLayer(t *testing.T) {
	roles := map[string]string{
		"idx":     "indexing",
		"vec":     "vectorize",
		"meta":    "metadata",
		"both":    "both",
		"upper":   "INDEXING",
		"padded":  "  both  ",
		"unknown": "skip",
		"empty":   "",
	}

	header := make([]string, 0, len(roles))
	row := make([]string, 0, len(roles))
	for col := range roles {
		header = append(header, col)
		row = append(row, col+"_v")
	}
	items, headers := parser.RenderRowsToJSONChunks([][]string{header, row}, "", "manual", roles)
	if len(items) != 1 {
		t.Fatalf("expected one row chunk, got %d", len(items))
	}
	text, _ := items[0]["text"].(string)
	chunkData, _ := items[0]["chunk_data"].(map[string]any)

	profile := ResolveTableProfile(map[string]interface{}{
		"table_column_mode": "manual",
		"table_column_roles": func() map[string]interface{} {
			out := make(map[string]interface{}, len(roles))
			for col, role := range roles {
				out[col] = role
			}
			return out
		}(),
	})
	fieldMap := BuildFieldMap(profile, headers)

	for _, col := range headers {
		role := roleFor(profile, col)
		wantText := role == common.ColumnRoleIndexing || role == common.ColumnRoleBoth
		wantStored := role == common.ColumnRoleMetadata || role == common.ColumnRoleBoth

		gotText := strings.Contains(text, "- "+col+": ")
		_, gotStored := chunkData[col]
		_, inFieldMap := fieldMap[col]

		if gotText != wantText || gotStored != wantStored {
			t.Errorf("column %q (role %q): render text=%v stored=%v, profile says text=%v stored=%v",
				col, roles[col], gotText, gotStored, wantText, wantStored)
		}
		if inFieldMap != wantStored {
			t.Errorf("column %q (role %q): field_map=%v, want %v (it must be addressable only when stored)",
				col, roles[col], inFieldMap, wantStored)
		}
	}

	if _, ok := fieldMap["unknown"]; ok {
		t.Errorf("an unknown role must not be advertised in the field_map: %v", fieldMap)
	}
}

// AggregateTableDocMetadata reads the same profile: a column whose role the
// vocabulary does not know contributes no document metadata (its chunk_data
// entry is never written by the renderer).
func TestAggregateTableDocMetadata_IgnoresUnknownRoleColumns(t *testing.T) {
	parserConfig := map[string]interface{}{
		"table_column_mode":  "manual",
		"table_column_roles": map[string]interface{}{"A": "skip", "B": "both"},
		"table_column_names": []interface{}{"A", "B"},
	}
	chunks := []map[string]any{
		{"chunk_data": map[string]interface{}{"B": "b1"}},
	}

	got := AggregateTableDocMetadata(chunks, parserConfig)
	if !reflect.DeepEqual(got, map[string]any{"B": []string{"b1"}}) {
		t.Fatalf("metadata = %#v, want only B", got)
	}
}
