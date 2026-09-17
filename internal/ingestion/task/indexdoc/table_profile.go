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
	"strings"

	"ragflow/internal/common"
)

// TableProfile is the parsed view of the table column configuration a run
// executes with: the mode, the per-column roles and the discovered column
// names, as resolved from a dataset's or document's parser_config. The rules
// derived from it (which columns reach the chunk body, which are addressable
// through the dataset field_map, which document metadata keys are stripped on
// reparse) live next to it here, in the layer that applies them.
type TableProfile struct {
	Mode     common.TableColumnMode       `json:"table_column_mode"`
	Roles    map[string]common.ColumnRole `json:"table_column_roles,omitempty"`
	RawRoles map[string]any               `json:"-"`
	Columns  []string                     `json:"table_column_names,omitempty"`
}

// NewTableProfile creates a TableProfile with initialized maps.
func NewTableProfile(mode common.TableColumnMode) *TableProfile {
	return &TableProfile{
		Mode:     mode,
		Roles:    make(map[string]common.ColumnRole),
		RawRoles: make(map[string]any),
		Columns:  make([]string, 0),
	}
}

// ToRolesInterfaceMap returns the column roles formatted as map[string]interface{}.
// The raw shape a caller supplied (RawRoles) wins, so a value the vocabulary
// does not know survives the round trip into a parser setup untouched.
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

// roleFor returns the effective common.ColumnRole for a column.
// In auto mode every column is "both"; in manual mode an unconfigured column is
// "both" too, while a configured value the vocabulary does not know normalizes
// to common.ColumnRoleNone and is therefore excluded everywhere (text,
// chunk_data, field_map) — the same classification the table parser's row
// renderer applies (internal/parser/parser/table_row_render.go).
func roleFor(profile *TableProfile, column string) common.ColumnRole {
	if profile == nil || !isManualProfile(profile) || profile.Roles == nil {
		return common.ColumnRoleBoth
	}
	if role, ok := profile.Roles[column]; ok && role != "" {
		return role
	}
	return common.ColumnRoleBoth
}

func isManualProfile(profile *TableProfile) bool {
	return profile != nil && profile.Mode == common.TableColumnModeManual
}

// BuildFieldMap projects the columns the SQL retrieval path can address into
// the dataset's field_map, mapping each stored column ("metadata" or "both") to
// its human-readable name (underscores become spaces).
func BuildFieldMap(profile *TableProfile, columns []string) map[string]interface{} {
	if profile == nil || len(columns) == 0 {
		return nil
	}
	fieldMap := make(map[string]interface{})
	for _, col := range columns {
		col = strings.TrimSpace(col)
		if col == "" {
			continue
		}
		role := roleFor(profile, col)
		if role == common.ColumnRoleMetadata || role == common.ColumnRoleBoth {
			fieldMap[col] = strings.ReplaceAll(col, "_", " ")
		}
	}
	if len(fieldMap) == 0 {
		return nil
	}
	return fieldMap
}
