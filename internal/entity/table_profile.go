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
	"strings"
)

// TableColumnMode specifies whether table columns are indexed automatically or manually.
type TableColumnMode string

const (
	TableColumnModeAuto   TableColumnMode = "auto"
	TableColumnModeManual TableColumnMode = "manual"
)

// ColumnRole defines the role of a table column during ingestion and retrieval.
type ColumnRole string

const (
	ColumnRoleIndexing ColumnRole = "indexing"
	ColumnRoleMetadata ColumnRole = "metadata"
	ColumnRoleBoth     ColumnRole = "both"
	// ColumnRoleNone is the outcome for a role value the vocabulary does not
	// know. It is an internal sentinel, never persisted: the written value is
	// always what the caller supplied. Python's table chunker classifies an
	// unknown role by membership tests (rag/app/table.py:626-635 for the chunk
	// body, rag/app/table.py:558 for the dataset field_map), so such a column
	// is excluded from text, chunk_data and the field_map alike — NOT treated
	// as "both".
	ColumnRoleNone ColumnRole = "none"
)

// NormalizeColumnRole converts a role string into a canonical ColumnRole.
// "vectorize" is an alias for "indexing"; an empty (or absent) role is the
// default "both"; any other value is ColumnRoleNone so that a misspelled role
// is excluded rather than silently promoted to "both" (Python parity).
func NormalizeColumnRole(role string) ColumnRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "indexing", "vectorize":
		return ColumnRoleIndexing
	case "metadata":
		return ColumnRoleMetadata
	case "both":
		return ColumnRoleBoth
	case "":
		return ColumnRoleBoth
	default:
		return ColumnRoleNone
	}
}

// NormalizeTableColumnMode converts a mode string into a canonical TableColumnMode.
func NormalizeTableColumnMode(mode string) TableColumnMode {
	if strings.EqualFold(strings.TrimSpace(mode), string(TableColumnModeManual)) {
		return TableColumnModeManual
	}
	return TableColumnModeAuto
}

// TableProfile captures the schema and column configuration of a table
// ingestion run. The behaviour derived from it (role resolution, field_map
// projection) lives with the ingestion rules that apply it, in
// internal/ingestion/task/indexdoc.
type TableProfile struct {
	Mode     TableColumnMode       `json:"table_column_mode"`
	Roles    map[string]ColumnRole `json:"table_column_roles,omitempty"`
	RawRoles map[string]any        `json:"-"`
	Columns  []string              `json:"table_column_names,omitempty"`
}

// NewTableProfile creates a TableProfile with initialized maps.
func NewTableProfile(mode TableColumnMode) *TableProfile {
	return &TableProfile{
		Mode:     mode,
		Roles:    make(map[string]ColumnRole),
		RawRoles: make(map[string]any),
		Columns:  make([]string, 0),
	}
}

// ToRolesInterfaceMap returns the column roles formatted as map[string]interface{}.
func (p *TableProfile) ToRolesInterfaceMap() map[string]interface{} {
	if p == nil {
		return nil
	}
	if len(p.RawRoles) > 0 {
		return p.RawRoles
	}
	if len(p.Roles) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(p.Roles))
	for k, v := range p.Roles {
		out[k] = string(v)
	}
	return out
}
