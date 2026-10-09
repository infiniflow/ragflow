package common

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// TableColumn is one spreadsheet column carrying its three names. Key matches
// column_roles configuration, DisplayName is for humans, and DataKey is the
// JSON key of chunk_data (and field_map).
type TableColumn struct {
	// Index is the 1-based position of the column in the sheet header.
	Index int `json:"index"`
	// Key is the normalized column key used by column_roles and document
	// column metadata.
	Key string `json:"key"`
	// DisplayName is the readable name for pages, body text and field
	// descriptions.
	DisplayName string `json:"display_name"`
	// DataKey is the JSON-safe key written to chunk_data: "c_" plus the full
	// lowercase hex SHA-256 of Key.
	DataKey string `json:"data_key"`
}

// emptyKeyPrefix is the key prefix for position-derived names of empty
// headers. A non-empty header can never produce it because headers escape
// '#' (an unescaped '#' only appears in duplicate suffixes and this prefix).
const emptyKeyPrefix = "#column:"

// NormalizeTableHeader applies Unicode NFC, strips leading/trailing BOM and
// whitespace, then escapes backslash and '#' so the result can be embedded in
// a column key unambiguously. Duplicate disambiguation and empty-header
// position names are added by DeriveColumns.
func NormalizeTableHeader(raw string) string {
	return escapeKey(normalizeCore(raw))
}

func normalizeCore(raw string) string {
	s := norm.NFC.String(raw)
	s = strings.Trim(s, "\ufeff")
	s = strings.TrimFunc(s, unicode.IsSpace)
	return s
}

func escapeKey(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "#", `\#`)
}

// DeriveColumns derives the key, display name and data key for one sheet's
// full header row. Repeated normalized headers get "#2", "#3", ... from the
// second occurrence onward; empty headers get a stable position-based key.
// Callers must pass the complete header row: renumbering depends on it, not
// on which data cells are non-empty.
func DeriveTableColumns(headers []string) []TableColumn {
	seen := make(map[string]int, len(headers))
	cols := make([]TableColumn, 0, len(headers))
	for i, raw := range headers {
		core := normalizeCore(raw)
		pos := i + 1
		key := core
		display := core
		if core == "" {
			key = emptyKeyPrefix + strconv.Itoa(pos)
			display = fmt.Sprintf("列 %d", pos)
		} else {
			base := escapeKey(core)
			seen[base]++
			key = base
			if seen[base] > 1 {
				key += "#" + strconv.Itoa(seen[base])
				display += " (" + strconv.Itoa(seen[base]) + ")"
			}
		}
		cols = append(cols, TableColumn{
			Index:       pos,
			Key:         key,
			DisplayName: display,
			DataKey:     TableDataKey(key),
		})
	}
	return cols
}

// TableDataKey returns the chunk_data JSON key for a column key. Its character set
// (c_ plus hex) is safe for JSONPath and SQL extraction; the same key maps to
// the same DataKey across files and sheets.
func TableDataKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "c_" + hex.EncodeToString(sum[:])
}

// Column modes. auto indexes every column into both body text and chunk_data
// without document-level column metadata; manual routes columns by their
// configured role.
const (
	TableModeAuto   = "auto"
	TableModeManual = "manual"
)

// Column roles for manual mode. indexing puts the column in body text only;
// metadata puts it in chunk_data and document aggregation; both does all
// three.
const (
	TableRoleIndexing = "indexing"
	TableRoleMetadata = "metadata"
	TableRoleBoth     = "both"
)

// IsValidTableRole reports whether role is one of the supported column roles.
func IsValidTableRole(role string) bool {
	switch role {
	case TableRoleIndexing, TableRoleMetadata, TableRoleBoth:
		return true
	}
	return false
}

// ValidateTableMode checks a raw column_mode value from configuration. Wrong types
// and unknown values are errors; nothing is silently dropped.
func ValidateTableMode(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("column_mode must be a string, got %T", v)
	}
	switch s {
	case TableModeAuto, TableModeManual:
		return s, nil
	default:
		return "", fmt.Errorf("column_mode %q is invalid: only %q and %q are allowed", s, TableModeAuto, TableModeManual)
	}
}

// ValidateTableRoles checks a raw column_roles value and returns the typed map.
// Keys must be non-empty normalized column keys; values must be supported
// role strings.
func ValidateTableRoles(v any) (map[string]string, error) {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("column_roles must be a JSON object, got %T", v)
	}
	roles := make(map[string]string, len(raw))
	for key, val := range raw {
		if key == "" {
			return nil, fmt.Errorf("column_roles contains an empty column key")
		}
		role, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("column_roles[%q] must be a string, got %T", key, val)
		}
		if !IsValidTableRole(role) {
			return nil, fmt.Errorf("column_roles[%q] has an invalid role %q: allowed roles are %s",
				key, role, strings.Join([]string{TableRoleIndexing, TableRoleMetadata, TableRoleBoth}, ", "))
		}
		roles[key] = role
	}
	return roles, nil
}
