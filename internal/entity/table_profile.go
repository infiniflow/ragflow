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
)

// NormalizeColumnRole converts a role string into a canonical ColumnRole.
// "vectorize" is an alias for "indexing". Empty or unrecognized roles default to "both".
func NormalizeColumnRole(role string) ColumnRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "indexing", "vectorize":
		return ColumnRoleIndexing
	case "metadata":
		return ColumnRoleMetadata
	case "both":
		return ColumnRoleBoth
	default:
		return ColumnRoleBoth
	}
}

// NormalizeTableColumnMode converts a mode string into a canonical TableColumnMode.
func NormalizeTableColumnMode(mode string) TableColumnMode {
	if strings.EqualFold(strings.TrimSpace(mode), string(TableColumnModeManual)) {
		return TableColumnModeManual
	}
	return TableColumnModeAuto
}

// TableProfile captures the strongly-typed schema and column configuration for table ingestion.
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

// IsManual returns true if the table column mode is manual.
func (p *TableProfile) IsManual() bool {
	return p != nil && p.Mode == TableColumnModeManual
}

// RoleFor returns the effective ColumnRole for the given column.
// In auto mode, all columns default to ColumnRoleBoth.
// In manual mode, unspecified columns default to ColumnRoleBoth.
func (p *TableProfile) RoleFor(column string) ColumnRole {
	if p == nil || !p.IsManual() || p.Roles == nil {
		return ColumnRoleBoth
	}
	if role, ok := p.Roles[column]; ok && role != "" {
		return role
	}
	return ColumnRoleBoth
}

// FilterRoles returns a map of roles filtered only to columns present in knownColumns.
func (p *TableProfile) FilterRoles(knownColumns map[string]struct{}) map[string]ColumnRole {
	if p == nil || len(p.Roles) == 0 {
		return nil
	}
	filtered := make(map[string]ColumnRole)
	for col, role := range p.Roles {
		if _, ok := knownColumns[col]; ok {
			filtered[col] = role
		}
	}
	return filtered
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

// ToRolesStringMap returns the column roles formatted as map[string]string.
func (p *TableProfile) ToRolesStringMap() map[string]string {
	if p == nil || len(p.Roles) == 0 {
		return nil
	}
	out := make(map[string]string, len(p.Roles))
	for k, v := range p.Roles {
		out[k] = string(v)
	}
	return out
}

// BuildFieldMap constructs a field_map mapping stored columns ("metadata" or "both")
// to human-readable names with spaces replacing underscores.
func (p *TableProfile) BuildFieldMap(columns []string) map[string]interface{} {
	if p == nil || len(columns) == 0 {
		return nil
	}
	fieldMap := make(map[string]interface{})
	for _, col := range columns {
		col = strings.TrimSpace(col)
		if col == "" {
			continue
		}
		role := p.RoleFor(col)
		if role == ColumnRoleMetadata || role == ColumnRoleBoth {
			fieldMap[col] = strings.ReplaceAll(col, "_", " ")
		}
	}
	if len(fieldMap) == 0 {
		return nil
	}
	return fieldMap
}
