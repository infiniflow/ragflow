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

package entity

import (
	"reflect"
	"testing"
)

func TestTableProfile_RoleFor(t *testing.T) {
	tests := []struct {
		name     string
		profile  *TableProfile
		col      string
		expected ColumnRole
	}{
		{
			name:     "nil profile defaults to both",
			profile:  nil,
			col:      "name",
			expected: ColumnRoleBoth,
		},
		{
			name:     "auto mode defaults all columns to both",
			profile:  &TableProfile{Mode: TableColumnModeAuto},
			col:      "name",
			expected: ColumnRoleBoth,
		},
		{
			name: "manual mode returns explicit role",
			profile: &TableProfile{
				Mode: TableColumnModeManual,
				Roles: map[string]ColumnRole{
					"name": ColumnRoleIndexing,
					"age":  ColumnRoleMetadata,
				},
			},
			col:      "name",
			expected: ColumnRoleIndexing,
		},
		{
			name: "manual mode unconfigured column defaults to both",
			profile: &TableProfile{
				Mode: TableColumnModeManual,
				Roles: map[string]ColumnRole{
					"name": ColumnRoleIndexing,
				},
			},
			col:      "city",
			expected: ColumnRoleBoth,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.profile.RoleFor(tc.col)
			if got != tc.expected {
				t.Fatalf("expected role %s, got %s", tc.expected, got)
			}
		})
	}
}

func TestTableProfile_BuildFieldMap(t *testing.T) {
	profile := &TableProfile{
		Mode: TableColumnModeManual,
		Roles: map[string]ColumnRole{
			"user_name": ColumnRoleIndexing, // should not be in field_map
			"user_age":  ColumnRoleMetadata, // should be in field_map
			"user_city": ColumnRoleBoth,     // should be in field_map
		},
	}

	cols := []string{"user_name", "user_age", "user_city", "empty_col"}
	fm := profile.BuildFieldMap(cols)

	if _, ok := fm["user_name"]; ok {
		t.Errorf("user_name (indexing only) should not be in field_map")
	}
	if fm["user_age"] != "user age" {
		t.Errorf("expected 'user age', got %v", fm["user_age"])
	}
	if fm["user_city"] != "user city" {
		t.Errorf("expected 'user city', got %v", fm["user_city"])
	}
	if fm["empty_col"] != "empty col" { // unconfigured column defaults to both
		t.Errorf("expected 'empty col', got %v", fm["empty_col"])
	}
}

func TestNormalizeColumnRole(t *testing.T) {
	if got := NormalizeColumnRole("vectorize"); got != ColumnRoleIndexing {
		t.Errorf("expected vectorize to normalize to indexing, got %s", got)
	}
	if got := NormalizeColumnRole("  INDEXING  "); got != ColumnRoleIndexing {
		t.Errorf("expected uppercase trimmed to normalize to indexing, got %s", got)
	}
	if got := NormalizeColumnRole("metadata"); got != ColumnRoleMetadata {
		t.Errorf("expected metadata, got %s", got)
	}
	if got := NormalizeColumnRole("unknown"); got != ColumnRoleBoth {
		t.Errorf("expected unknown to default to both, got %s", got)
	}
}

func TestTableProfile_FilterRoles(t *testing.T) {
	profile := &TableProfile{
		Mode: TableColumnModeManual,
		Roles: map[string]ColumnRole{
			"colA": ColumnRoleIndexing,
			"colB": ColumnRoleMetadata,
			"colC": ColumnRoleBoth,
		},
	}

	validCols := map[string]struct{}{
		"colA": {},
		"colC": {},
	}

	filtered := profile.FilterRoles(validCols)
	expected := map[string]ColumnRole{
		"colA": ColumnRoleIndexing,
		"colC": ColumnRoleBoth,
	}

	if !reflect.DeepEqual(filtered, expected) {
		t.Errorf("expected %v, got %v", expected, filtered)
	}
}
