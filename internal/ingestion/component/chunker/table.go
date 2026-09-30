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

// TableChunker emits one chunk per upstream table row. It is the faithful
// Go port of the Python `table` chunk method, whose docstring states:
// "Every row in table will be treated as a chunk."
//
// Unlike TokenChunker (which token-shreds a row's text into multiple
// pieces) or OneChunker (which merges many rows into a single chunk),
// TableChunker keeps the row as the unit of chunking. Every table arrives as
// rendered HTML — the spreadsheet parsers emit captioned table segments — and
// each data row becomes one chunk whose text repeats the column names in the
// Python "- field: value" line format, carrying its own position tuple when
// the item is a spreadsheet item with a row-aligned matrix. A table whose only
// row is the header stays one whole chunk.
package chunker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	ingestiontable "ragflow/internal/ingestion/table"

	"gorm.io/gorm"
)

const ComponentNameTableChunker = "TableChunker"

type tableChunkerParam struct {
	schema.TableChunkerParam
	updateErr error
}

func (p *tableChunkerParam) Update(conf map[string]any) {
	// The pipeline feeds saved configuration back through this path on every
	// run, and one stale or mistyped value must not abort a document that used
	// to parse: failures are collected and reported by Validate.
	_, _ = p.applyColumnParams(conf)
}

// applyColumnParams reads the column fields from a component parameter map.
// Unlike Update it reports why a value was refused, which is what the settings
// and upload APIs need: a client that sent an unknown mode, role or type has to
// hear about it instead of watching the value dropped.
func (p *tableChunkerParam) applyColumnParams(conf map[string]any) (map[string]struct{}, error) {
	handled := map[string]struct{}{}
	if conf == nil {
		return handled, nil
	}
	if v, ok := conf["column_mode"]; ok {
		handled["column_mode"] = struct{}{}
		mode, err := ingestiontable.ValidateMode(v)
		if err != nil {
			p.updateErr = errors.Join(p.updateErr, err)
		} else {
			p.ColumnMode = mode
		}
	}
	if v, ok := conf["column_roles"]; ok {
		handled["column_roles"] = struct{}{}
		roles, err := ingestiontable.ValidateRoles(v)
		if err != nil {
			p.updateErr = errors.Join(p.updateErr, err)
		} else {
			p.ColumnRoles = roles
		}
	}
	return handled, p.updateErr
}

func (tableChunkerParam) Defaults() tableChunkerParam {
	return tableChunkerParam{
		TableChunkerParam: schema.TableChunkerParam{
			ColumnMode:  ingestiontable.ModeAuto,
			ColumnRoles: map[string]string{},
		},
	}
}

func (p tableChunkerParam) Validate() error {
	if p.updateErr != nil {
		return p.updateErr
	}
	if p.ColumnMode != ingestiontable.ModeAuto && p.ColumnMode != ingestiontable.ModeManual {
		return fmt.Errorf("column_mode %q is invalid: only %q and %q are allowed",
			p.ColumnMode, ingestiontable.ModeAuto, ingestiontable.ModeManual)
	}
	for key, role := range p.ColumnRoles {
		if !ingestiontable.ValidRole(role) {
			return fmt.Errorf("column_roles[%q] has an invalid role %q", key, role)
		}
	}
	return nil
}

type TableChunkerComponent struct {
	name  string
	param tableChunkerParam
}

func NewTableChunker(params map[string]any) (runtime.Component, error) {
	p := tableChunkerParam{}.Defaults()
	(&p).Update(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &TableChunkerComponent{
		name:  ComponentNameTableChunker,
		param: p,
	}, nil
}
func (c *TableChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *TableChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *TableChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

// tableProfile is the column configuration one TableChunker node applies to
// the spreadsheet rows it emits, resolved once per run.
type tableProfile struct {
	nodeID string
	mode   string
	roles  map[string]string
	key    string
	manual bool
}

func (p tableProfile) roleFor(key string) string {
	if !p.manual {
		return ingestiontable.RoleBoth
	}
	if role, ok := p.roles[key]; ok {
		return role
	}
	return ingestiontable.RoleBoth
}

// effectiveRoles resolves each column's role for building a row. An unset
// manual column behaves as both.
func (p tableProfile) effectiveRoles(cols []ingestiontable.Column) map[string]string {
	roles := make(map[string]string, len(cols))
	for _, col := range cols {
		roles[col.Key] = p.roleFor(col.Key)
	}
	return roles
}

// declaredRoles returns only the roles the configuration states. A row carries
// these rather than the effective ones, because "unset means both" must stay
// recoverable after the fact: document-level column values are aggregated for
// explicitly configured metadata/both columns, and a row written under auto must
// not look like it declared every column.
func (p tableProfile) declaredRoles() map[string]string {
	if !p.manual {
		return nil
	}
	return p.roles
}

func (c *TableChunkerComponent) invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{},
			"_ERROR":        fmt.Sprintf("Input error: %v", err),
		}, nil
	}

	profile := tableProfile{
		nodeID: runtime.ComponentNodeID(ctx),
		mode:   c.param.ColumnMode,
		roles:  c.param.ColumnRoles,
		key:    ingestiontable.ProfileKey(c.param.ColumnMode, c.param.ColumnRoles),
		manual: c.param.ColumnMode == ingestiontable.ModeManual,
	}

	switch upstream.OutputFormat {
	case schema.PayloadFormatMarkdown:
		if upstream.MarkdownResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.MarkdownResult, "text"), nil
	case schema.PayloadFormatText:
		if upstream.TextResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.TextResult, "text"), nil
	case schema.PayloadFormatHTML:
		if upstream.HTMLResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.HTMLResult, "text"), nil
	default:
		// Row-structured payload: one chunk per upstream record.
		items, err := tableItems(upstream.JSONResult, upstream.Chunks, profile, upstream.FileType)
		if err != nil {
			return map[string]any{
				"output_format": "chunks",
				"chunks":        []map[string]any{},
				"_ERROR":        err.Error(),
			}, nil
		}
		if len(items) == 0 {
			return emptyOutputs(), nil
		}
		return chunkOutputs(items), nil
	}
}

// supportsColumnMode reports whether a parsed file reaches the chunker on the
// canonical spreadsheet wire that column roles are defined against. The
// spreadsheet parsers emit segmented HTML tables with a row-aligned position
// matrix and a 1-based sheet index; TSV is not routed to the CSV parser at all,
// and a .xls extension alone does not prove the bytes went through a real OLE
// XLS reader. Manual roles are therefore only honoured for csv/xlsx.
func supportsColumnMode(fileType string) bool {
	switch strings.ToLower(strings.TrimPrefix(fileType, ".")) {
	case "csv", "xlsx":
		return true
	}
	return false
}

// tableItems returns the per-row records, preferring JSONResult and
// falling back to Chunks. Each HTML table row becomes exactly one chunk;
// every other payload record passes through as one chunk — including
// pre-upgrade row-IR records (ck_type: table_row/table_header with cells),
// which hold no markup and therefore keep no per-row positions; documents
// from before this wire must be re-parsed rather than re-chunked.
func tableItems(items, chunks []schema.ChunkDoc, profile tableProfile, fileType string) ([]schema.ChunkDoc, error) {
	source := items
	if len(source) == 0 {
		source = chunks
	}
	if len(source) == 0 {
		return nil, nil
	}
	filtered := make([]schema.ChunkDoc, 0, len(source))
	for _, item := range source {
		expanded, err := expandHTMLTableRows(item, profile, fileType)
		if err != nil {
			return nil, err
		}
		filtered = append(filtered, expanded...)
	}
	return filtered, nil
}

// expandHTMLTableRows turns one HTML <table> payload into one chunk per data
// row. Non-table payloads pass through unchanged. A table whose only row is
// the header keeps the whole markup as its single chunk: the header line is
// then the only searchable representation.
//
// On the spreadsheet wire each row additionally carries its structured cells
// (chunk_data) and its source identity, so a row stays addressable even when
// column roles leave its body text empty. In manual mode a row set that
// filters down to nothing emits no chunk rather than falling back to the whole
// table markup, which would leak every excluded column back into the index.
func expandHTMLTableRows(item schema.ChunkDoc, profile tableProfile, fileType string) ([]schema.ChunkDoc, error) {
	if !isTableHTML(item.Text) {
		return []schema.ChunkDoc{item}, nil
	}
	rows, headerCount := tableRowsWithHeader(item.Text)
	spreadsheet := item.SheetIndex != nil && headerCount >= 1
	if profile.manual {
		if !supportsColumnMode(fileType) {
			return nil, fmt.Errorf("TableChunker: column mode %q is not supported for file type %q; column roles apply to csv and xlsx only", profile.mode, fileType)
		}
		if !spreadsheet {
			return nil, fmt.Errorf("TableChunker: column mode %q needs the spreadsheet table wire, which this item (%s) does not carry", profile.mode, describeTableItem(item))
		}
	}
	if len(rows) <= headerCount {
		if profile.manual && spreadsheet {
			// Header only: no data row to index, and the whole-table
			// fallback would publish the unfiltered markup.
			return nil, nil
		}
		return []schema.ChunkDoc{item}, nil
	}
	names := rows[0]
	// R1: a row chunk must carry only its own tuple. That is only sound when
	// the item carries spreadsheet identity and its matrix was built
	// row-aligned (one five-field tuple per <tr>, header included); a
	// whole-table tuple must not be copied onto every row, and PDF items write
	// layout boxes into the same field, so misaligned or non-spreadsheet
	// payloads get no positions at all.
	var matrix [][]float64
	aligned := false
	if item.SheetIndex != nil && len(item.Positions) > 0 {
		if err := json.Unmarshal(item.Positions, &matrix); err == nil && len(matrix) == len(rows) {
			aligned = true
			for _, tuple := range matrix {
				if len(tuple) != 5 {
					aligned = false
					break
				}
			}
		}
	}
	if profile.manual && spreadsheet && !aligned {
		return nil, fmt.Errorf("TableChunker: column mode %q needs one row-aligned position tuple per <tr>; item (%s) has %d rows and %d tuples",
			profile.mode, describeTableItem(item), len(rows), len(matrix))
	}

	cols := ingestiontable.DeriveColumns(names)
	rowRoles := profile.effectiveRoles(cols)
	out := make([]schema.ChunkDoc, 0, len(rows)-headerCount)
	for i, row := range rows[headerCount:] {
		sourceRow := 0
		if aligned {
			sourceRow = int(matrix[headerCount+i][1])
		}
		var tuple json.RawMessage
		if aligned {
			if raw, err := json.Marshal([][]float64{matrix[headerCount+i]}); err == nil {
				tuple = raw
			}
		}
		doc := item
		doc.TKNums = nil
		doc.Positions = tuple

		if !spreadsheet {
			doc.Text = tableRowRecordText(names, row)
			if doc.Text == "" {
				continue
			}
			out = append(out, doc)
			continue
		}

		// A row wider than the header has cells no column identity covers;
		// dropping them silently would index a row that cannot be read back
		// through chunk_data.
		if len(row) > len(cols) {
			return nil, fmt.Errorf("TableChunker: data row has %d cells but the header has %d columns (sheet %d, row %d)",
				len(row), len(cols), *item.SheetIndex, sourceRow)
		}
		text, data := projectTableRow(cols, rowRoles, row)
		if text == "" && len(data) == 0 {
			continue
		}
		if text == "" {
			// Every visible column is metadata-only: the row still has to
			// carry text the tokenizer can work on, and it must not repeat
			// the cell values that are excluded from the body.
			text = fmt.Sprintf("Sheet %d, row %d", *item.SheetIndex, sourceRow)
		}
		doc.Text = text
		doc.ChunkData = data
		doc.TableRowInt = 1
		doc.TableProfileKey = profile.key
		doc.TableRowSource = &schema.TableRowSource{
			NodeID:     profile.nodeID,
			SheetIndex: *item.SheetIndex,
			SourceRow:  sourceRow,
			Mode:       profile.mode,
			Columns:    cols,
			Roles:      profile.declaredRoles(),
		}
		out = append(out, doc)
	}
	if len(out) == 0 {
		if profile.manual && spreadsheet {
			return nil, nil
		}
		return []schema.ChunkDoc{item}, nil
	}
	return out, nil
}

// projectTableRow renders one spreadsheet row under its effective column
// roles: body text carries the columns visible to indexing, chunk_data carries
// the columns readable as structured values, with an empty cell written as an
// empty string so the row's field set stays the sheet's field set. A row whose
// cells are all empty yields nothing.
func projectTableRow(cols []ingestiontable.Column, roles map[string]string, cells []string) (string, map[string]any) {
	lines := make([]string, 0, len(cells))
	data := make(map[string]any, len(cols))
	hasValue := false
	for j, col := range cols {
		value := ""
		if j < len(cells) {
			value = strings.TrimSpace(cells[j])
		}
		if value != "" {
			hasValue = true
		}
		role := roles[col.Key]
		if role != ingestiontable.RoleMetadata && value != "" {
			lines = append(lines, "- "+col.DisplayName+": "+value)
		}
		if role != ingestiontable.RoleIndexing {
			data[col.DataKey] = value
		}
	}
	if !hasValue {
		return "", nil
	}
	return strings.Join(lines, "\n"), data
}

// tableRowIdentity returns the hash input that identifies a spreadsheet row
// chunk by where it came from, or ok=false for any other chunk. Two different
// source rows that render the same text, and the same row re-parsed under
// different column roles, both keep this identity.
func tableRowIdentity(ck map[string]any) (string, bool) {
	src, ok := ck["table_row_source"].(map[string]any)
	if !ok {
		return "", false
	}
	sheet, okSheet := jsonNumber(src["sheet_index"])
	row, okRow := jsonNumber(src["source_row"])
	if !okSheet || !okRow || row == 0 {
		return "", false
	}
	return fmt.Sprintf("table-row:v1:%d:%d", sheet, row), true
}

// jsonNumber reads a number that reaches this layer either as an int (in the
// same process) or as a float64 (after a JSON round trip through the Tokenizer
// or a checkpoint).
func jsonNumber(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// describeTableItem names an item in a column-mode error by the identity a
// reader would recognise: sheet and table id rather than a dump of the markup.
func describeTableItem(item schema.ChunkDoc) string {
	if item.SheetIndex != nil {
		return fmt.Sprintf("sheet_index=%d, sheet=%q", *item.SheetIndex, item.Sheet)
	}
	return fmt.Sprintf("ck_type=%q, sheet=%q", item.CKType, item.Sheet)
}

// tableRowRecordText renders one row in the Python table chunker's line format:
// "- {field}: {value}" per non-empty cell, newline joined. A cell whose
// column has no header name keeps its value alone rather than being dropped.
func tableRowRecordText(names, cells []string) string {
	lines := make([]string, 0, len(cells))
	for j, cell := range cells {
		value := strings.TrimSpace(cell)
		if value == "" {
			continue
		}
		name := ""
		if j < len(names) {
			name = strings.TrimSpace(names[j])
		}
		if name != "" {
			lines = append(lines, "- "+name+": "+value)
			continue
		}
		lines = append(lines, "- "+value)
	}
	return strings.Join(lines, "\n")
}

func init() {
	MustRegisterChunker(ComponentNameTableChunker)
}
