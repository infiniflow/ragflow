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
