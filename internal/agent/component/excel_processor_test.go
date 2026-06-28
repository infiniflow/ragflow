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

package component

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/agent/canvas"
)

// excelCtx returns a fresh canvas-state context for component tests.
func excelCtx(t *testing.T) context.Context {
	t.Helper()
	state := canvas.NewCanvasState("run-xlsx", "task-xlsx")
	return canvas.WithState(context.Background(), state)
}

// TestExcelProcessor_WriteThenRead: write a 2x2 grid, then read it
// back from the produced bytes; rows should round-trip.
func TestExcelProcessor_WriteThenRead(t *testing.T) {
	grid := [][]any{
		{"a", "b"},
		{1, 2},
	}
	w, err := NewExcelProcessorComponent(map[string]any{
		"operation":   "write",
		"output_data": grid,
	})
	if err != nil {
		t.Fatalf("NewExcelProcessorComponent (write): %v", err)
	}
	out, err := w.Invoke(excelCtx(t), map[string]any{})
	if err != nil {
		t.Fatalf("write Invoke: %v", err)
	}
	raw, ok := out["bytes"].([]byte)
	if !ok || len(raw) == 0 {
		t.Fatalf("write: bytes output missing or empty (got %T)", out["bytes"])
	}
	if size, _ := out["size"].(int); size != len(raw) {
		t.Errorf("write: size=%d, want %d (len bytes)", size, len(raw))
	}
	if names, _ := out["sheet_names"].([]string); len(names) == 0 {
		t.Errorf("write: sheet_names empty, want >=1")
	}
	// ZIP magic header.
	if !(raw[0] == 'P' && raw[1] == 'K' && raw[2] == 3 && raw[3] == 4) {
		t.Errorf("write: bytes do not start with PK\\x03\\x04 ZIP magic: %x", raw[:4])
	}

	// Read those bytes back with a fresh component.
	r, err := NewExcelProcessorComponent(map[string]any{"operation": "read"})
	if err != nil {
		t.Fatalf("NewExcelProcessorComponent (read): %v", err)
	}
	rout, err := r.Invoke(excelCtx(t), map[string]any{"bytes": raw})
	if err != nil {
		t.Fatalf("read Invoke: %v", err)
	}
	rows, _ := rout["rows"].([][]any)
	if len(rows) != 2 {
		t.Fatalf("read: got %d rows, want 2", len(rows))
	}
	// excelize returns everything as strings via GetRows. Compare cell-
	// by-cell so 1 (int we wrote) and "1" (string excelize reports) line
	// up via fmt.Sprintf("%v", ...).
	if got, want := fmt.Sprintf("%v", rows[0][0]), "a"; got != want {
		t.Errorf("read rows[0][0] = %q, want %q", got, want)
	}
	if got, want := fmt.Sprintf("%v", rows[0][1]), "b"; got != want {
		t.Errorf("read rows[0][1] = %q, want %q", got, want)
	}
	if got, want := fmt.Sprintf("%v", rows[1][0]), "1"; got != want {
		t.Errorf("read rows[1][0] = %q, want %q", got, want)
	}
	if got, want := fmt.Sprintf("%v", rows[1][1]), "2"; got != want {
		t.Errorf("read rows[1][1] = %q, want %q", got, want)
	}
}

// TestExcelProcessor_ReadSheetNames: build a workbook with two sheets
// directly via excelize, then read it back via the component and
// confirm both names appear.
func TestExcelProcessor_ReadSheetNames(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if _, err := f.NewSheet("Alpha"); err != nil {
		t.Fatalf("NewSheet Alpha: %v", err)
	}
	if err := f.SetCellValue("Alpha", "A1", "x"); err != nil {
		t.Fatalf("SetCellValue: %v", err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}

	r, _ := NewExcelProcessorComponent(map[string]any{"operation": "read"})
	out, err := r.Invoke(excelCtx(t), map[string]any{"bytes": buf.Bytes()})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	names, _ := out["sheet_names"].([]string)
	if !reflect.DeepEqual(names, []string{"Sheet1", "Alpha"}) {
		t.Errorf("sheet_names = %v, want [Sheet1 Alpha]", names)
	}
}

// TestExcelProcessor_EmptyFile: writing an empty grid produces a
// valid (small) xlsx; reading it back returns zero rows but a valid
// sheet list.
func TestExcelProcessor_EmptyFile(t *testing.T) {
	w, _ := NewExcelProcessorComponent(map[string]any{
		"operation":   "write",
		"output_data": [][]any{},
	})
	out, err := w.Invoke(excelCtx(t), map[string]any{})
	if err != nil {
		t.Fatalf("write Invoke: %v", err)
	}
	raw, _ := out["bytes"].([]byte)
	if len(raw) == 0 {
		t.Fatal("write: expected non-empty bytes for an empty-grid xlsx")
	}
	if !(raw[0] == 'P' && raw[1] == 'K' && raw[2] == 3 && raw[3] == 4) {
		t.Errorf("write: bytes do not start with PK\\x03\\x04 ZIP magic: %x", raw[:4])
	}

	r, _ := NewExcelProcessorComponent(map[string]any{"operation": "read"})
	rout, err := r.Invoke(excelCtx(t), map[string]any{"bytes": raw})
	if err != nil {
		t.Fatalf("read Invoke: %v", err)
	}
	rows, _ := rout["rows"].([][]any)
	if len(rows) != 0 {
		t.Errorf("read empty: got %d rows, want 0", len(rows))
	}
	names, _ := rout["sheet_names"].([]string)
	if len(names) == 0 {
		t.Errorf("read empty: sheet_names should still list the default sheet")
	}
}

// TestExcelProcessor_ParamCheck: invalid operation rejected.
func TestExcelProcessor_ParamCheck(t *testing.T) {
	if _, err := NewExcelProcessorComponent(map[string]any{"operation": "bogus"}); err == nil {
		t.Fatal("expected error for bogus operation, got nil")
	}
}

// TestExcelProcessor_ReadMissingBytes: read without inputs.bytes
// surfaces a ParamError.
func TestExcelProcessor_ReadMissingBytes(t *testing.T) {
	r, _ := NewExcelProcessorComponent(map[string]any{"operation": "read"})
	_, err := r.Invoke(excelCtx(t), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing bytes, got nil")
	}
}

// TestExcelProcessor_Registered: factory lookup.
func TestExcelProcessor_Registered(t *testing.T) {
	c, err := New("ExcelProcessor", map[string]any{"operation": "read"})
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if c.Name() != "ExcelProcessor" {
		t.Errorf("Name()=%q, want ExcelProcessor", c.Name())
	}
}
