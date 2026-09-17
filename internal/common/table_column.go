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

import "strings"

// The table column vocabulary below is the persisted contract of the
// `table_column_mode` / `table_column_roles` keys in a dataset's or document's
// `parser_config` JSON. It lives in this shared-kernel package because the
// layers that must agree on it sit on both sides of the parser: the table
// parser decodes the values while rendering rows
// (internal/parser/parser/table_row_render.go), and the ingestion layer
// resolves a profile from parser_config to aggregate document metadata and the
// dataset field_map (internal/ingestion/task/indexdoc). Keeping one
// implementation here is what makes those two classify every value identically.

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
