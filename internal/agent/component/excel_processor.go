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

// Package component — ExcelProcessor (T5, plan §2.11.3 row 22).
//
// ExcelProcessor supports three operations:
//
//	read   — open a workbook, list sheet names, return first-sheet rows
//	write  — create a new workbook from a [][]any grid, return bytes
//	merge  — read 2+ workbooks, concatenate the first-sheet rows
//
// The implementation is built on github.com/xuri/excelize/v2 (BSD-3,
// license-clean per plan §2.11.5). For P4 we ship the read and write
// operations; merge is a basic concatenation of first-sheet rows.
//
// file_ref points at a state-bound binary. For P4 we accept either:
//   - inputs["file_ref"] carrying raw xlsx bytes ([]byte or base64
//     string), or
//   - inputs["file_ref"] / param.file_ref naming a state key whose value
//     resolves to []byte via the canvas state engine.
//
// The write path stores the produced bytes on outputs["bytes"] so
// downstream nodes can attach them to a MinIO upload.
package component

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/agent/runtime"
)

const (
	componentNameExcelProcessor = "ExcelProcessor"

	defaultSheetName = "Sheet1"
)

// excelProcessorParam is the static configuration for an ExcelProcessor
// node. file_ref, output_data, and sheet_name are duplicated in
// inputs for per-call overrides; the param holds the defaults.
type excelProcessorParam struct {
	Operation  string  `json:"operation"`   // "read" | "write" | "merge"
	FileRef    string  `json:"file_ref"`    // state ref to xlsx bytes (read/merge)
	OutputData [][]any `json:"output_data"` // grid for write
	SheetName  string  `json:"sheet_name"`  // default sheet name
}

// Update copies a fresh param map into the receiver.
func (p *excelProcessorParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	p.Operation, _ = conf["operation"].(string)
	if p.Operation == "" {
		p.Operation = "read"
	}
	p.FileRef, _ = conf["file_ref"].(string)
	p.SheetName, _ = conf["sheet_name"].(string)
	if p.SheetName == "" {
		p.SheetName = defaultSheetName
	}

	switch v := conf["output_data"].(type) {
	case [][]any:
		p.OutputData = v
	case []any:
		// Some serializers collapse the outer slice into []any; unwrap.
		out := make([][]any, 0, len(v))
		for _, item := range v {
			if row, ok := item.([]any); ok {
				out = append(out, row)
			}
		}
		p.OutputData = out
	default:
		// nil / unsupported — leave as-is; Check()/Invoke will reject.
	}
	return nil
}

// Check validates the param.
func (p *excelProcessorParam) Check() error {
	switch p.Operation {
	case "read", "write", "merge":
		// ok
	default:
		return &ParamError{Field: "operation", Reason: "must be one of: read, write, merge"}
	}
	if p.SheetName == "" {
		return &ParamError{Field: "sheet_name", Reason: "must not be empty"}
	}
	return nil
}

// AsDict returns the params as a plain map.
func (p *excelProcessorParam) AsDict() map[string]any {
	return map[string]any{
		"operation":   p.Operation,
		"file_ref":    p.FileRef,
		"output_data": p.OutputData,
		"sheet_name":  p.SheetName,
	}
}

// ExcelProcessorComponent implements the read/write/merge Excel node.
type ExcelProcessorComponent struct {
	name  string
	param excelProcessorParam
}

// NewExcelProcessorComponent constructs an ExcelProcessor from the DSL
// param map.
func NewExcelProcessorComponent(params map[string]any) (Component, error) {
	p := &excelProcessorParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("ExcelProcessor: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("ExcelProcessor: param check: %w", err)
	}
	return &ExcelProcessorComponent{
		name:  componentNameExcelProcessor,
		param: *p,
	}, nil
}

// Name returns the registered component name.
func (e *ExcelProcessorComponent) Name() string { return e.name }

// Invoke runs the configured operation and returns the result map.
// Output shape:
//
//	read   — {"rows": [][]any, "sheet_names": []string, "size": <int>}
//	write  — {"rows": [][]any, "sheet_names": []string, "size": <int>,
//	          "bytes": <[]byte>}
//	merge  — {"rows": [][]any, "sheet_names": []string, "size": <int>}
func (e *ExcelProcessorComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// ExcelProcessor does not currently read from canvas state for
	// binary blobs, but we still pull state so a nil-state error is
	// surfaced early.
	if _, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err != nil {
		return nil, fmt.Errorf("ExcelProcessor: %w", err)
	}

	// Resolve operation: input override → param.
	op := e.param.Operation
	if v, ok := inputs["operation"].(string); ok && v != "" {
		op = v
	}
	op = strings.ToLower(strings.TrimSpace(op))
	switch op {
	case "read", "write", "merge":
		// ok
	default:
		return nil, &ParamError{Field: "operation", Reason: "must be one of: read, write, merge"}
	}

	// Resolve sheet_name: input → param.
	sheetName := e.param.SheetName
	if v, ok := inputs["sheet_name"].(string); ok && v != "" {
		sheetName = v
	}

	switch op {
	case "write":
		return e.doWrite(sheetName, inputs)
	case "read":
		return e.doRead(sheetName, inputs)
	case "merge":
		return e.doMerge(sheetName, inputs)
	}
	// Unreachable thanks to the switch above, kept for the compiler.
	return nil, errors.New("ExcelProcessor: unreachable")
}

// Stream mirrors Invoke; ExcelProcessor is a single-shot transform.
func (e *ExcelProcessorComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := e.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns parameter metadata.
func (e *ExcelProcessorComponent) Inputs() map[string]string {
	return map[string]string{
		"operation":   "One of read | write | merge. Defaults to param.operation, then \"read\".",
		"file_ref":    "Reference to xlsx bytes; resolves via inputs or param.file_ref.",
		"output_data": "Grid ([][]any) for write; can be supplied per-invocation.",
		"sheet_name":  "Target sheet name; default \"Sheet1\".",
	}
}

// Outputs returns the response surface.
func (e *ExcelProcessorComponent) Outputs() map[string]string {
	return map[string]string{
		"rows":        "Read/write/merge result rows ([][]any). Empty for empty workbook.",
		"sheet_names": "All sheet names in the workbook ([]string).",
		"size":        "Number of bytes for write; row count for read/merge.",
		"bytes":       "Write-only: raw xlsx bytes ([]byte) ready for storage upload.",
	}
}

// doWrite builds a new workbook and returns its bytes.
func (e *ExcelProcessorComponent) doWrite(sheetName string, inputs map[string]any) (map[string]any, error) {
	grid := e.param.OutputData
	if v, ok := inputs["output_data"].([][]any); ok {
		grid = v
	}
	if sheetName == "" {
		sheetName = defaultSheetName
	}

	f := excelize.NewFile()
	defer f.Close()

	// Replace the default "Sheet1" only if sheetName is non-default; we
	// always set the index/active sheet to whichever the caller asked
	// for so the resulting file opens cleanly.
	if sheetName != defaultSheetName {
		if err := f.SetSheetName(defaultSheetName, sheetName); err != nil {
			return nil, fmt.Errorf("ExcelProcessor: rename default sheet: %w", err)
		}
	}
	idx, err := f.GetSheetIndex(sheetName)
	if err != nil || idx < 0 {
		// Sheet vanished; create it explicitly.
		if _, cerr := f.NewSheet(sheetName); cerr != nil {
			return nil, fmt.Errorf("ExcelProcessor: create sheet %q: %w", sheetName, cerr)
		}
	}

	for r, row := range grid {
		for c, cell := range row {
			cellRef, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err := f.SetCellValue(sheetName, cellRef, cell); err != nil {
				return nil, fmt.Errorf("ExcelProcessor: set cell %s: %w", cellRef, err)
			}
		}
	}

	// Set the active sheet to the one we just wrote.
	idx, _ = f.GetSheetIndex(sheetName)
	if idx >= 0 {
		f.SetActiveSheet(idx)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("ExcelProcessor: write buffer: %w", err)
	}
	out := buf.Bytes()
	sheetNames := f.GetSheetList()
	return map[string]any{
		"rows":        grid,
		"sheet_names": sheetNames,
		"size":        len(out),
		"bytes":       out,
	}, nil
}

// doRead opens the workbook referenced by inputs.file_ref (or
// param.file_ref) and returns its first-sheet rows.
func (e *ExcelProcessorComponent) doRead(sheetName string, inputs map[string]any) (map[string]any, error) {
	raw, err := e.resolveFileBytes(inputs)
	if err != nil {
		return nil, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("ExcelProcessor: open xlsx: %w", err)
	}
	defer f.Close()

	sheetNames := f.GetSheetList()
	if len(sheetNames) == 0 {
		return map[string]any{
			"rows":        [][]any{},
			"sheet_names": []string{},
			"size":        0,
		}, nil
	}
	if sheetName == "" {
		sheetName = sheetNames[0]
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("ExcelProcessor: get rows %q: %w", sheetName, err)
	}
	grid := make([][]any, 0, len(rows))
	for _, row := range rows {
		converted := make([]any, 0, len(row))
		for _, cell := range row {
			converted = append(converted, cell)
		}
		grid = append(grid, converted)
	}
	return map[string]any{
		"rows":        grid,
		"sheet_names": sheetNames,
		"size":        len(grid),
	}, nil
}

// doMerge reads 2+ workbooks and concatenates their first-sheet rows.
// Inputs must supply either:
//   - inputs["file_refs"] as a [][]byte / []any of byte slices, or
//   - inputs["file_ref"] / param.file_ref naming a single workbook
//     (then merge reduces to that workbook's first sheet).
func (e *ExcelProcessorComponent) doMerge(sheetName string, inputs map[string]any) (map[string]any, error) {
	blobs, err := e.resolveMergeBlobs(inputs)
	if err != nil {
		return nil, err
	}
	if sheetName == "" {
		sheetName = defaultSheetName
	}

	merged := make([][]any, 0)
	var firstSheetNames []string
	for i, b := range blobs {
		f, err := excelize.OpenReader(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("ExcelProcessor: merge open #%d: %w", i, err)
		}
		sheets := f.GetSheetList()
		if i == 0 {
			firstSheetNames = sheets
		}
		if len(sheets) == 0 {
			f.Close()
			continue
		}
		target := sheets[0]
		if sheetName != "" && sheetName != defaultSheetName {
			if idx, _ := f.GetSheetIndex(sheetName); idx >= 0 {
				target = sheetName
			}
		}
		rows, err := f.GetRows(target)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("ExcelProcessor: merge rows #%d: %w", i, err)
		}
		for _, row := range rows {
			converted := make([]any, 0, len(row))
			for _, cell := range row {
				converted = append(converted, cell)
			}
			merged = append(merged, converted)
		}
		f.Close()
	}
	return map[string]any{
		"rows":        merged,
		"sheet_names": firstSheetNames,
		"size":        len(merged),
	}, nil
}

// resolveFileBytes returns the raw xlsx bytes for read mode. Accepts:
//   - inputs["bytes"]    as []byte
//   - inputs["file_ref"] as []byte, OR as a base64 string
//
// We don't accept param.file_ref here because binary blobs live in
// canvas state, not in the static DSL; the orchestrator is expected to
// have resolved the state ref into the inputs map already.
func (e *ExcelProcessorComponent) resolveFileBytes(inputs map[string]any) ([]byte, error) {
	if b, ok := inputs["bytes"].([]byte); ok && len(b) > 0 {
		return b, nil
	}
	if b, ok := inputs["file_ref"].([]byte); ok && len(b) > 0 {
		return b, nil
	}
	if s, ok := inputs["file_ref"].(string); ok && s != "" {
		return decodeBase64(s)
	}
	if s, ok := inputs["bytes"].(string); ok && s != "" {
		return decodeBase64(s)
	}
	return nil, &ParamError{Field: "file_ref", Reason: "no xlsx bytes supplied (provide inputs.bytes or inputs.file_ref as []byte or base64)"}
}

// resolveMergeBlobs returns the [][]byte list for merge mode. Accepts
// inputs["file_refs"] as [][]byte or []any of []byte; falls back to a
// single-blob read.
func (e *ExcelProcessorComponent) resolveMergeBlobs(inputs map[string]any) ([][]byte, error) {
	switch v := inputs["file_refs"].(type) {
	case [][]byte:
		if len(v) == 0 {
			return nil, &ParamError{Field: "file_refs", Reason: "must not be empty for merge"}
		}
		return v, nil
	case []any:
		if len(v) == 0 {
			return nil, &ParamError{Field: "file_refs", Reason: "must not be empty for merge"}
		}
		out := make([][]byte, 0, len(v))
		for i, item := range v {
			b, ok := item.([]byte)
			if !ok {
				return nil, fmt.Errorf("ExcelProcessor: file_refs[%d] is %T, want []byte", i, item)
			}
			out = append(out, b)
		}
		return out, nil
	}
	// Single-file fallback: treat file_ref / bytes as the only blob.
	b, err := e.resolveFileBytes(inputs)
	if err != nil {
		return nil, err
	}
	return [][]byte{b}, nil
}

// decodeBase64 returns the base64-decoded byte slice for s. We require
// base64 because binary blobs should never round-trip through strings
// silently — that path is a known source of encoding bugs. Callers with
// already-binary data should pass []byte, not string.
func decodeBase64(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("ExcelProcessor: empty file_ref string")
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("ExcelProcessor: file_ref is not valid base64: %w", err)
	}
	return decoded, nil
}

func init() {
	Register(componentNameExcelProcessor, NewExcelProcessorComponent)
}
