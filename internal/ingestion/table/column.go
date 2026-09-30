// Package table defines the column identity model for TableChunker column
// mode: normalized column keys, readable display names, JSON-safe data keys,
// and validation of column mode and role configuration. It is shared by the
// chunker, the column probe, config validation and index projection, and
// depends on nothing above it (no DAO, HTTP, or search-engine types).
package table

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Column is one spreadsheet column carrying its three names. Key matches
// column_roles configuration, DisplayName is for humans, and DataKey is the
// JSON key of chunk_data (and field_map).
type Column struct {
	// Index is the 1-based position of the column in the sheet header.
	Index int
	// Key is the normalized column key used by column_roles and document
	// column metadata.
	Key string
	// DisplayName is the readable name for pages, body text and field
	// descriptions.
	DisplayName string
	// DataKey is the JSON-safe key written to chunk_data: "c_" plus the full
	// lowercase hex SHA-256 of Key.
	DataKey string
}

// emptyKeyPrefix is the key prefix for position-derived names of empty
// headers. A non-empty header can never produce it because headers escape
// '#' (an unescaped '#' only appears in duplicate suffixes and this prefix).
const emptyKeyPrefix = "#column:"

// NormalizeHeader applies Unicode NFC, strips leading/trailing BOM and
// whitespace, then escapes backslash and '#' so the result can be embedded in
// a column key unambiguously. Duplicate disambiguation and empty-header
// position names are added by DeriveColumns.
func NormalizeHeader(raw string) string {
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
func DeriveColumns(headers []string) []Column {
	seen := make(map[string]int, len(headers))
	cols := make([]Column, 0, len(headers))
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
		cols = append(cols, Column{
			Index:       pos,
			Key:         key,
			DisplayName: display,
			DataKey:     DataKey(key),
		})
	}
	return cols
}

// DataKey returns the chunk_data JSON key for a column key. Its character set
// (c_ plus hex) is safe for JSONPath and SQL extraction; the same key maps to
// the same DataKey across files and sheets.
func DataKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "c_" + hex.EncodeToString(sum[:])
}
