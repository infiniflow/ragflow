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
	"fmt"
	"sort"
	"strings"
)

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
	// unknown role by membership tests (rag/app/table.py:687-689 for the chunk
	// body, rag/app/table.py:616 for the dataset field_map), so such a column
	// is excluded from text, chunk_data and the field_map alike — NOT treated
	// as "both".
	ColumnRoleNone ColumnRole = "none"
)

// NormalizeColumnRole converts one persisted role value into a canonical
// ColumnRole. "vectorize" is an alias for "indexing"; anything else the
// vocabulary does not know — the empty string, a different case, surrounding
// whitespace included — is ColumnRoleNone, so such a value is excluded rather
// than silently promoted to "both". This is Python's membership test verbatim
// (rag/app/table.py:688-689), which compares the stored string and neither
// trims nor case-folds it. The default "both" belongs to the lookup, not to
// this function: it applies to a column the roles map does not carry, which is
// what Python's `column_roles.get(col, "both")` expresses.
func NormalizeColumnRole(role string) ColumnRole {
	switch role {
	case "indexing", "vectorize":
		return ColumnRoleIndexing
	case "metadata":
		return ColumnRoleMetadata
	case "both":
		return ColumnRoleBoth
	default:
		return ColumnRoleNone
	}
}

// NormalizeTableColumnMode maps a persisted mode onto the vocabulary. Only the
// exact "manual" selects manual, matching Python's
// `parser_config.get("table_column_mode") == "manual"`
// (rag/app/table.py:586); every other value — absent, blank, unknown or
// differently cased — is auto.
func NormalizeTableColumnMode(mode string) TableColumnMode {
	if mode == string(TableColumnModeManual) {
		return TableColumnModeManual
	}
	return TableColumnModeAuto
}

// ValidateTableColumnSettings rejects table column values the runtime cannot
// act on: a mode other than auto/manual, a role outside the three known values
// (plus the legacy "vectorize" alias), and column names that are not a list of
// strings. An absent key, and an empty mode, are accepted — that is "unset",
// which the runtime reads as its default.
//
// Both shapes a request can use are covered: the root-level `table_column_*`
// keys (upload override, dataset settings) and the component-shaped
// `column_*` keys a parser dialog writes under a `Parser:<id>` spreadsheet
// entry.
//
// Python validates the same contract at its API boundary
// (api/utils/validation_utils.py:430 defines the role Literal, :457/:459/:461
// the fields, on a strict model at :436), so an unknown value never reaches
// parsing there. This is the Go equivalent: the runtime's "unknown role is
// excluded" rule stays a safety net for legacy rows instead of the normal write
// path.
//
// The checks accept what Python accepts (a role entry keyed by an empty column
// name is meaningless but not rejected there, and an absent value is "unset"),
// and reject what Python rejects: an unknown, blank, differently cased or
// non-string role, a mode outside auto/manual, and names that are not a list of
// strings. Comparison is exact because Python's Literal is.
func ValidateTableColumnSettings(config map[string]interface{}) error {
	if config == nil {
		return nil
	}
	if err := validateTableColumnKeys(config, "table_column_mode", "table_column_roles", "table_column_names"); err != nil {
		return err
	}
	for _, cpnID := range sortedKeys(config) {
		if !strings.HasPrefix(cpnID, "Parser:") {
			continue
		}
		component, ok := config[cpnID].(map[string]interface{})
		if !ok {
			continue
		}
		spreadsheet, ok := component["spreadsheet"].(map[string]interface{})
		if !ok {
			continue
		}
		if err := validateTableColumnKeys(spreadsheet, "column_mode", "column_roles", "column_names"); err != nil {
			return fmt.Errorf("%s.spreadsheet: %w", cpnID, err)
		}
	}
	return nil
}

func validateTableColumnKeys(config map[string]interface{}, modeKey, rolesKey, namesKey string) error {
	if raw, ok := config[modeKey]; ok && raw != nil {
		mode, isString := raw.(string)
		if !isString {
			return fmt.Errorf("%s must be a string", modeKey)
		}
		if mode != "" && mode != string(TableColumnModeAuto) && mode != string(TableColumnModeManual) {
			return fmt.Errorf("%s must be %q or %q, got %q", modeKey, TableColumnModeAuto, TableColumnModeManual, mode)
		}
	}

	if raw, ok := config[rolesKey]; ok && raw != nil {
		switch roles := raw.(type) {
		case map[string]interface{}:
			if err := validateTableColumnRoles(rolesKey, roles); err != nil {
				return err
			}
		case map[string]string:
			converted := make(map[string]interface{}, len(roles))
			for column, role := range roles {
				converted[column] = role
			}
			if err := validateTableColumnRoles(rolesKey, converted); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s must be an object mapping column name to role", rolesKey)
		}
	}

	if raw, ok := config[namesKey]; ok && raw != nil {
		names, isList := raw.([]interface{})
		if !isList {
			if _, isStringList := raw.([]string); isStringList {
				return nil
			}
			return fmt.Errorf("%s must be a list of strings", namesKey)
		}
		for _, name := range names {
			if _, isString := name.(string); !isString {
				return fmt.Errorf("%s must contain only strings, got %T", namesKey, name)
			}
		}
	}
	return nil
}

func validateTableColumnRoles(rolesKey string, roles map[string]interface{}) error {
	for _, column := range sortedKeys(roles) {
		role, isString := roles[column].(string)
		if !isString {
			return fmt.Errorf("%s[%q] must be a string", rolesKey, column)
		}
		if NormalizeColumnRole(role) == ColumnRoleNone {
			return fmt.Errorf("%s[%q] must be one of %q, %q, %q or the legacy %q, got %q",
				rolesKey, column, ColumnRoleIndexing, ColumnRoleMetadata, ColumnRoleBoth, "vectorize", role)
		}
	}
	return nil
}

// sortedKeys returns the map's keys in ascending order so validation reports
// the first offending entry deterministically.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
