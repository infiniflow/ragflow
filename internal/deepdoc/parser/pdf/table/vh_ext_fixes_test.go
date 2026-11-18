package table

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestMergeTablesAcrossPages_NoContinuation_EmptyVHPageSpans is the adversarial
// guard for the span publication contract: a table that never merges must keep
// VHPageSpans empty so ConstructTable stays on the flat path.
func TestMergeTablesAcrossPages_NoContinuation_EmptyVHPageSpans(t *testing.T) {
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Cells:     []pdf.TSRCell{{Text: "page0", X0: 0, Y0: 0, X1: 10, Y1: 10}},
		},
		{
			Positions: []pdf.Position{{PageNumbers: []int{3}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Cells:     []pdf.TSRCell{{Text: "page3", X0: 0, Y0: 0, X1: 10, Y1: 10}},
		},
	}
	merged := MergeTablesAcrossPages(tables, nil, map[int]float64{0: 842, 3: 842})
	if len(merged) != 2 {
		t.Fatalf("expected 2 unmerged tables, got %d", len(merged))
	}
	for i, m := range merged {
		if len(m.VHPageSpans) != 0 {
			t.Errorf("table %d has no continuation; VHPageSpans must be empty, got %+v", i, m.VHPageSpans)
		}
	}
}

// TestMergeTablesAcrossPages_WithContinuation_HasVHPageSpans is the happy path:
// a genuine cross-page merge must publish one span per contributing page.
func TestMergeTablesAcrossPages_WithContinuation_HasVHPageSpans(t *testing.T) {
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 800}},
			Scale:     1.0,
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "pg0_r0c0"},
				{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "pg0_r0c1"},
			},
		},
		{
			Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 50, Right: 500, Top: 100, Bottom: 300}},
			Scale:     1.0,
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "pg1_r0c0"},
				{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "pg1_r0c1"},
			},
		},
	}
	merged := MergeTablesAcrossPages(tables, nil, map[int]float64{0: 820, 1: 820})
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	if got := len(merged[0].VHPageSpans); got != 2 {
		t.Errorf("merged table should have 2 per-page spans, got %d", got)
	}
}

// TestDropEmptyColumns_RaggedGridDoesNotPanic guards the column-index bound:
// a row longer than the first must not index past the keep slice.
func TestDropEmptyColumns_RaggedGridDoesNotPanic(t *testing.T) {
	grid := [][]pdf.TSRCell{
		{{Text: "a"}},
		{{Text: "b"}, {Text: ""}},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("dropEmptyColumns panicked on a ragged grid: %v", r)
		}
	}()
	out := dropEmptyColumns(grid)
	for i, row := range out {
		if len(row) != 1 {
			t.Errorf("row %d width = %d, want 1 (empty column dropped)", i, len(row))
		}
	}
}

// TestGridToStreamCells_RaggedGridDoesNotPanic guards the same bound through
// the public helper that calls dropEmptyColumns.
func TestGridToStreamCells_RaggedGridDoesNotPanic(t *testing.T) {
	grid := [][]pdf.TSRCell{
		{{Text: "a", X0: 0, Y0: 0, X1: 10, Y1: 10}},
		{{Text: "b", X0: 0, Y0: 10, X1: 10, Y1: 20}, {Text: "", X0: 10, Y0: 10, X1: 20, Y1: 20}},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("gridToStreamCells panicked on a ragged grid: %v", r)
		}
	}()
	if got := len(gridToStreamCells(grid)); got != 2 {
		t.Errorf("gridToStreamCells emitted %d cells, want 2", got)
	}
}

// parseFuncs indexes every function declaration in a source file by name.
func parseFuncs(t *testing.T, path string) map[string]*ast.FuncDecl {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make(map[string]*ast.FuncDecl)
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			out[fn.Name.Name] = fn
		}
	}
	return out
}

// TestConstructTableCoreDocComment guards the doc comment that documents the
// core renderer against being silently dropped or left misplaced.
func TestConstructTableCoreDocComment(t *testing.T) {
	funcs := parseFuncs(t, "table_construct.go")
	core := funcs["constructTableCore"]
	if core == nil {
		t.Fatal("constructTableCore not found")
	}
	if core.Doc == nil || !strings.Contains(core.Doc.Text(), "constructTableCore produces an HTML table") {
		t.Error("constructTableCore must carry its own doc comment")
	}
	strip := funcs["StripCaptionFromCells"]
	if strip != nil && strip.Doc != nil && strings.Contains(strip.Doc.Text(), "constructTable produces an HTML table string") {
		t.Error("StripCaptionFromCells carries the misplaced core-renderer doc comment")
	}
}

// TestGridUnifiedDocCommentAccurate guards the field contract text: the field
// is a reserved hook, not something MergeTablesAcrossPages sets today.
func TestGridUnifiedDocCommentAccurate(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "../../type/types.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse types.go: %v", err)
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "TableItem" {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 || field.Names[0].Name != "GridUnified" {
					continue
				}
				doc := ""
				if field.Doc != nil {
					doc = field.Doc.Text()
				}
				if strings.Contains(doc, "is set by MergeTablesAcrossPages") {
					t.Error("GridUnified doc falsely claims MergeTablesAcrossPages sets it")
				}
				if !strings.Contains(strings.ToLower(doc), "reserved") {
					t.Error("GridUnified doc must describe the field as a reserved hook")
				}
				return
			}
		}
	}
	t.Fatal("GridUnified field not found")
}
