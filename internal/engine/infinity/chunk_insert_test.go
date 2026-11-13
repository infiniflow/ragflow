// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package infinity

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	infinity "github.com/infiniflow/infinity-go-sdk"

	"ragflow/internal/utility"
)

func loadMappingSchema(t *testing.T, name string) *orderedFields {
	t.Helper()
	path, err := utility.FindConfFileInProject(name)
	if err != nil || path == nil {
		t.Fatalf("locate %s: %v", name, err)
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		t.Fatalf("read %s: %v", *path, err)
	}
	var schema orderedFields
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse %s: %v", *path, err)
	}
	return &schema
}

// TestSchemaDeclaresMediaOrdinals pins which mappings declare the parser media
// ordinal columns. The reconcile must follow the selected mapping: the chunk
// mapping declares slide_number/media_order, the skill mapping declares
// neither — a skill table must not get columns its schema never wanted.
func TestSchemaDeclaresMediaOrdinals(t *testing.T) {
	if !schemaDeclaresMediaOrdinals(loadMappingSchema(t, "infinity_mapping.json")) {
		t.Error("conf/infinity_mapping.json must declare the media ordinal columns")
	}
	if schemaDeclaresMediaOrdinals(loadMappingSchema(t, "skill_infinity_mapping.json")) {
		t.Error("conf/skill_infinity_mapping.json declares no media ordinal columns; the skill table must not get them")
	}
}

// fakeColumnAdder records the AddColumns calls a reconcile makes and fails
// them on demand.
type fakeColumnAdder struct {
	calls   int
	added   []string
	failErr error
}

func (f *fakeColumnAdder) AddColumns(columnDefs infinity.TableSchema) (interface{}, error) {
	f.calls++
	for _, def := range columnDefs {
		if def != nil {
			f.added = append(f.added, def.Name)
		}
	}
	if f.failErr != nil {
		return nil, f.failErr
	}
	return nil, nil
}

// TestReconcileMediaOrdinalColumnsAbortsOnFailure pins that a failed ordinal
// reconcile aborts the write instead of being logged and ignored: the insert
// that follows rejects a batch naming an undeclared column, so continuing
// would drop the matching rows on the delete and only then fail (#20500).
func TestReconcileMediaOrdinalColumnsAbortsOnFailure(t *testing.T) {
	adder := &fakeColumnAdder{failErr: errors.New("add columns failed")}
	chunks := []map[string]interface{}{{"id": "c1", "slide_number": 1}}

	err := (&Engine{}).reconcileMediaOrdinalColumns(adder, chunks, false, false)
	if err == nil {
		t.Fatal("a failed reconcile must abort the write, not be swallowed")
	}
	if !strings.Contains(err.Error(), "add columns failed") {
		t.Fatalf("error %q must wrap the AddColumns failure", err)
	}
	if adder.calls != 1 {
		t.Fatalf("AddColumns called %d times, want 1", adder.calls)
	}
}

// TestEnsureMediaOrdinalColumnsClassifiesTransportFailures pins the decision
// the connection eviction keys on. A reconcile that failed at the transport
// level leaves the socket with an incomplete Thrift exchange, so the caller
// must be able to tell that from the error the reconcile returns and drop the
// pooled connection. An ordinary schema failure must not be classified as a
// transport failure: the connection is still healthy and must survive it.
func TestEnsureMediaOrdinalColumnsClassifiesTransportFailures(t *testing.T) {
	tests := []struct {
		name        string
		addErr      error
		wantConnErr bool
	}{
		{
			name:        "infinity EOF",
			addErr:      errors.New("InfinityException(7018, Failed to execute query: EOF)"),
			wantConnErr: true,
		},
		{
			name:        "thrift transport failure",
			addErr:      errors.New("TProtocolException: transport is not open"),
			wantConnErr: true,
		},
		{
			name:        "schema rejection",
			addErr:      errors.New("add columns failed"),
			wantConnErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adder := &fakeColumnAdder{failErr: tc.addErr}
			chunks := []map[string]interface{}{{"id": "c1", "slide_number": 1}}

			err := (&Engine{}).reconcileMediaOrdinalColumns(adder, chunks, false, false)
			if err == nil {
				t.Fatal("a failed reconcile must return the AddColumns error")
			}
			if !strings.Contains(err.Error(), tc.addErr.Error()) {
				t.Fatalf("error %q must wrap the AddColumns failure", err)
			}
			if got := isConnectionLevelError(err); got != tc.wantConnErr {
				t.Fatalf("isConnectionLevelError(%q) = %v, want %v", err, got, tc.wantConnErr)
			}
		})
	}
}

// TestReconcileMediaOrdinalColumnsSkipsBatchesWithoutOrdinals pins that only a
// batch carrying slide_number/media_order reconciles the columns onto the
// table.
func TestReconcileMediaOrdinalColumnsSkipsBatchesWithoutOrdinals(t *testing.T) {
	adder := &fakeColumnAdder{}
	chunks := []map[string]interface{}{{"id": "c1"}}

	if err := (&Engine{}).reconcileMediaOrdinalColumns(adder, chunks, false, false); err != nil {
		t.Fatalf("a batch without ordinal columns must not reconcile: %v", err)
	}
	if adder.calls != 0 {
		t.Fatalf("AddColumns called %d times, want 0", adder.calls)
	}
}

// TestReconcileMediaOrdinalColumnsAddsMissingColumns pins that the reconcile
// adds exactly the missing ordinal columns and leaves the existing ones alone.
func TestReconcileMediaOrdinalColumnsAddsMissingColumns(t *testing.T) {
	adder := &fakeColumnAdder{}
	chunks := []map[string]interface{}{{"id": "c1", "media_order": 2}}

	if err := (&Engine{}).reconcileMediaOrdinalColumns(adder, chunks, true, false); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(adder.added) != 1 || adder.added[0] != "media_order" {
		t.Fatalf("added columns = %v, want [media_order]", adder.added)
	}
}

// TestParserIDFromChunks pins the table-parser detection that gates the
// chunk_data column reconcile: only a batch that carries chunk_data needs the
// parser-specific column, and any position in the batch counts (a mixed batch
// still writes chunk_data on the rows that have it).
func TestParserIDFromChunks(t *testing.T) {
	var nilMap map[string]interface{}

	tests := []struct {
		name   string
		chunks []map[string]interface{}
		want   string
	}{
		{name: "no chunks", chunks: nil, want: ""},
		{name: "plain chunk", chunks: []map[string]interface{}{{"id": "c1"}}, want: ""},
		{name: "nil chunk_data", chunks: []map[string]interface{}{{"chunk_data": nil}}, want: ""},
		{name: "typed nil chunk_data", chunks: []map[string]interface{}{{"chunk_data": nilMap}}, want: ""},
		{name: "empty chunk_data", chunks: []map[string]interface{}{{"chunk_data": map[string]interface{}{}}}, want: "table"},
		{
			name:   "table chunk after a plain chunk",
			chunks: []map[string]interface{}{{"id": "c1"}, {"chunk_data": map[string]interface{}{"rows": []interface{}{}}}},
			want:   "table",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parserIDFromChunks(tc.chunks); got != tc.want {
				t.Fatalf("parserIDFromChunks() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBatchNeedsMediaOrdinalColumns pins the gate for the media ordinal
// reconcile on the insert path: only a batch that actually carries
// slide_number or media_order (the PPTX slide/image rows) should reconcile
// the columns onto a legacy table, in any batch position.
func TestBatchNeedsMediaOrdinalColumns(t *testing.T) {
	tests := []struct {
		name   string
		chunks []map[string]interface{}
		want   bool
	}{
		{name: "no chunks", chunks: nil, want: false},
		{name: "plain chunk", chunks: []map[string]interface{}{{"id": "c1"}}, want: false},
		{name: "slide row", chunks: []map[string]interface{}{{"slide_number": 3}}, want: true},
		{name: "image row", chunks: []map[string]interface{}{{"media_order": 2}}, want: true},
		{
			name: "slide row after a plain chunk",
			chunks: []map[string]interface{}{
				{"id": "c1"},
				{"id": "c2", "slide_number": 1},
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := batchNeedsMediaOrdinalColumns(tc.chunks); got != tc.want {
				t.Fatalf("batchNeedsMediaOrdinalColumns() = %v, want %v", got, tc.want)
			}
		})
	}
}

// insertChunksWriteOrder reports where InsertChunks guards the media ordinal
// reconcile relative to the delete of the rows with the same ids, and whether
// the reconcile error aborts the write. The offsets are byte positions in
// chunk.go; a zero offset means the construct was not found.
func insertChunksWriteOrder(t *testing.T) (reconcileOffset, deleteOffset int, abortsOnError bool) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "chunk.go", nil, 0)
	if err != nil {
		t.Fatalf("parse chunk.go: %v", err)
	}

	var insert *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "InsertChunks" {
			insert = fn
			break
		}
	}
	if insert == nil {
		t.Fatal("chunk.go does not declare InsertChunks")
	}

	ast.Inspect(insert.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			if !callsMethod(node, "reconcileMediaOrdinalColumns") {
				return true
			}
			reconcileOffset = fset.Position(node.Pos()).Offset
			for _, stmt := range node.Body.List {
				ret, ok := stmt.(*ast.ReturnStmt)
				if !ok {
					continue
				}
				for _, res := range ret.Results {
					if ident, ok := res.(*ast.Ident); ok && ident.Name == "err" {
						abortsOnError = true
					}
				}
			}
		case *ast.AssignStmt:
			if callsTableDelete(node) {
				deleteOffset = fset.Position(node.Pos()).Offset
			}
		}
		return true
	})
	return reconcileOffset, deleteOffset, abortsOnError
}

// callsTableDelete reports whether the node contains a call to table.Delete.
func callsTableDelete(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Delete" {
			return true
		}
		if recv, ok := sel.X.(*ast.Ident); ok && recv.Name == "table" {
			found = true
		}
		return true
	})
	return found
}

// callsMethod reports whether the node contains a call to the named method.
func callsMethod(node ast.Node, method string) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == method {
			found = true
		}
		return true
	})
	return found
}

// TestInsertChunksReconcilesBeforeDeleting is the caller-level regression
// test for the insert path: the media ordinal reconcile must run before the
// delete of the rows with the same ids, and its error must abort the write.
//
// The insert path holds a real *infinity.Table from db.GetTable, so an
// AddColumns failure cannot be produced without a live Infinity instance.
// This test reads chunk.go instead and pins the two properties the caller
// depends on: a change that ignores the reconcile error, or that moves the
// delete ahead of the reconcile, fails here.
//
// The order matters because of what a failed reconcile means: the batch still
// names an undeclared ordinal column, so the insert below fails anyway —
// after the delete has already dropped the matching rows (#20500).
func TestInsertChunksReconcilesBeforeDeleting(t *testing.T) {
	reconcileOffset, deleteOffset, abortsOnError := insertChunksWriteOrder(t)

	if reconcileOffset == 0 {
		t.Fatal("InsertChunks does not guard the media ordinal reconcile with an if statement")
	}
	if !abortsOnError {
		t.Error("InsertChunks must return the reconcile error instead of continuing")
	}
	if deleteOffset == 0 {
		t.Fatal("InsertChunks does not delete the rows with the same ids")
	}
	if reconcileOffset > deleteOffset {
		t.Errorf("the media ordinal reconcile (offset %d) must run before the delete (offset %d): a failed reconcile leaves the batch naming an undeclared column, so the insert would fail after the delete already dropped the matching rows (#20500)",
			reconcileOffset, deleteOffset)
	}
}
