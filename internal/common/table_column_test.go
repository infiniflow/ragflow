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
// pinned here: a known value maps to its role and anything else — blank,
// padded, differently cased, misspelled — is excluded (ColumnRoleNone) rather
// than promoted to "both". Python compares the stored string as is
// (rag/app/table.py:629-631 for the chunk body, :558 for the dataset
// field_map), and the "both" default belongs to the lookup, not to this
// mapping: it covers a column the roles map does not carry.
func TestNormalizeColumnRole(t *testing.T) {
	tests := []struct {
		in   string
		want ColumnRole
	}{
		{"indexing", ColumnRoleIndexing},
		{"vectorize", ColumnRoleIndexing},
		{"metadata", ColumnRoleMetadata},
		{"both", ColumnRoleBoth},
		{"skip", ColumnRoleNone},
		{"none", ColumnRoleNone},
		{"vectorise", ColumnRoleNone},
		{"", ColumnRoleNone},
		{"   ", ColumnRoleNone},
		{"  INDEXING  ", ColumnRoleNone},
		{"METADATA", ColumnRoleNone},
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
		{"auto", TableColumnModeAuto},
		{"", TableColumnModeAuto},
		{"manul", TableColumnModeAuto},
		// Python compares the persisted string with == "manual", so a padded
		// or differently cased mode is auto there; the runtime must not read it
		// as manual.
		{"MANUAL", TableColumnModeAuto},
		{"  MANUAL ", TableColumnModeAuto},
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
		{"legacy vectorize alias", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": "vectorize"}}},
		{"empty mode means unset", map[string]interface{}{"table_column_mode": ""}},
		{"nil values", map[string]interface{}{"table_column_mode": nil, "table_column_roles": nil}},
		{"names list", map[string]interface{}{"table_column_names": []interface{}{"a", "b"}}},
		{"empty names list", map[string]interface{}{"table_column_names": []interface{}{}}},
		// Python's dict[str, Literal[...]] accepts an empty key; the runtime never
		// matches it against a real column, so it is not an error.
		{"empty column name", map[string]interface{}{"table_column_roles": map[string]interface{}{"": "both"}}},
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
		{"differently cased mode", map[string]interface{}{"table_column_mode": "MANUAL"}, "table_column_mode must be"},
		{"padded mode", map[string]interface{}{"table_column_mode": "  manual "}, "table_column_mode must be"},
		{"non-string mode", map[string]interface{}{"table_column_mode": float64(1)}, "must be a string"},
		{"unknown role", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": "skip"}}, `table_column_roles["a"]`},
		// Python rejects both of these at the API (api/utils/validation_utils.py:430),
		// and its chunker would exclude the column, so Go must not persist them either.
		{"empty role", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": ""}}, `table_column_roles["a"]`},
		{"differently cased role", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": "INDEXING"}}, `table_column_roles["a"]`},
		{"non-string role", map[string]interface{}{"table_column_roles": map[string]interface{}{"a": float64(1)}}, "must be a string"},
		{"names not a list", map[string]interface{}{"table_column_names": "a,b"}, "must be a list of strings"},
		{"names with a non-string entry", map[string]interface{}{"table_column_names": []interface{}{"a", float64(2)}}, "must contain only strings"},
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
		{"non-list nested names", map[string]interface{}{
			"Parser:HipSignsRhyme": map[string]interface{}{
				"spreadsheet": map[string]interface{}{"column_names": float64(1)},
			},
		}, "Parser:HipSignsRhyme.spreadsheet: column_names must be a list of strings"},
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
