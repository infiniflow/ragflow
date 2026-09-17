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

package common

import (
	"strings"
	"testing"
)

// The role vocabulary is shared by the table parser's row renderer and the
// ingestion layer that aggregates the same rows, so its classification is
// pinned here: a known value maps to its role, an absent/empty value is the
// default "both", and a value the vocabulary does not know is excluded
// (ColumnRoleNone) rather than promoted to "both" — matching Python's
// membership tests (rag/app/table.py:626-635, rag/app/table.py:558).
func TestNormalizeColumnRole(t *testing.T) {
	tests := []struct {
		in   string
		want ColumnRole
	}{
		{"indexing", ColumnRoleIndexing},
		{"vectorize", ColumnRoleIndexing},
		{"  INDEXING  ", ColumnRoleIndexing},
		{"metadata", ColumnRoleMetadata},
		{"both", ColumnRoleBoth},
		{"", ColumnRoleBoth},
		{"   ", ColumnRoleBoth},
		{"skip", ColumnRoleNone},
		{"none", ColumnRoleNone},
		{"vectorise", ColumnRoleNone},
	}
	for _, tc := range tests {
		if got := NormalizeColumnRole(tc.in); got != tc.want {
			t.Errorf("NormalizeColumnRole(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeTableColumnMode(t *testing.T) {
	tests := []struct {
		in   string
		want TableColumnMode
	}{
		{"manual", TableColumnModeManual},
		{"  MANUAL ", TableColumnModeManual},
		{"auto", TableColumnModeAuto},
		{"", TableColumnModeAuto},
		{"manul", TableColumnModeAuto},
	}
	for _, tc := range tests {
		if got := NormalizeTableColumnMode(tc.in); got != tc.want {
			t.Errorf("NormalizeTableColumnMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Validation mirrors Python's API boundary: a mode or role the runtime cannot
// act on is rejected before it is persisted, while an unset value stays valid.
func TestValidateTableColumnSettings(t *testing.T) {
	valid := []struct {
		name   string
		config map[string]interface{}
	}{
		{"nil config", nil},
		{"empty config", map[string]interface{}{}},
		{"unset keys", map[string]interface{}{"chunk_token_num": float64(128)}},
		{"mode auto", map[string]interface{}{"table_column_mode": "auto"}},
		{"mode manual with roles", map[string]interface{}{
			"table_column_mode":  "manual",
			"table_column_roles": map[string]interface{}{"a": "indexing", "b": "metadata", "c": "both"},
		}},
		{"mode case-insensitive and trimmed", map[string]interface{}{"table_column_mode": "  MANUAL "}},
		{"legacy vectorize alias", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": "vectorize"}}},
		{"empty role means default", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": ""}}},
		{"empty mode means unset", map[string]interface{}{"table_column_mode": ""}},
		{"nil values", map[string]interface{}{"table_column_mode": nil, "table_column_roles": nil}},
		{"string-valued roles map", map[string]interface{}{"table_column_roles": map[string]string{"a": "both"}}},
		{"component-shaped spreadsheet", map[string]interface{}{
			"Parser:HipSignsRhyme": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"column_mode":  "manual",
					"column_roles": map[string]interface{}{"a": "indexing"},
				},
			},
		}},
		{"unrelated component entry", map[string]interface{}{
			"Parser:HipSignsRhyme": map[string]interface{}{"pdf": map[string]interface{}{"pages": []interface{}{}}},
		}},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if err := ValidateTableColumnSettings(tc.config); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}

	invalid := []struct {
		name   string
		config map[string]interface{}
		substr string
	}{
		{"unknown mode", map[string]interface{}{"table_column_mode": "manul"}, "table_column_mode must be"},
		{"non-string mode", map[string]interface{}{"table_column_mode": float64(1)}, "must be a string"},
		{"unknown role", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": "skip"}}, `table_column_roles["a"]`},
		{"non-string role", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": float64(1)}}, "must be a string"},
		{"empty column name", map[string]interface{}{"table_column_roles": map[string]interface{}{"": "both"}}, "empty column name"},
		{"roles not a map", map[string]interface{}{"table_column_roles": "indexing"}, "must be an object"},
		{"unknown nested mode", map[string]interface{}{
			"Parser:HipSignsRhyme": map[string]interface{}{
				"spreadsheet": map[string]interface{}{"column_mode": "manul"},
			},
		}, "Parser:HipSignsRhyme.spreadsheet: column_mode must be"},
		{"unknown nested role", map[string]interface{}{
			"Parser:HipSignsRhyme": map[string]interface{}{
				"spreadsheet": map[string]interface{}{"column_roles": map[string]interface{}{"a": "skip"}},
			},
		}, `Parser:HipSignsRhyme.spreadsheet: column_roles["a"]`},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			err := ValidateTableColumnSettings(tc.config)
			if err == nil {
				t.Fatalf("expected an error for %#v", tc.config)
			}
			if !strings.Contains(err.Error(), tc.substr) {
				t.Fatalf("error %q does not mention %q", err, tc.substr)
			}
		})
	}
}
