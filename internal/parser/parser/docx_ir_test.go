package parser

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

// TestBuildDOCXJSONSections_FromJSON feeds an office_oxide IR JSON
// string directly into buildDOCXJSONSections and asserts the
// paragraph / heading / image / table / list branches. This pins the
// pure-Go IR → sections transform without requiring the office_oxide
// native library, so it runs under CGO_ENABLED=0.
func TestBuildDOCXJSONSections_FromJSON(t *testing.T) {
	// A nil sink is the text-only path: the embedded image is skipped, so
	// no image item is emitted.
	irJSON := `{"sections":[{"title":"","elements":[
		{"type":"heading","level":1,"content":[{"type":"text","text":"Title"}]},
		{"type":"paragraph","content":[{"type":"text","text":"Hello"}]},
		{"type":"image","data":"aGVsbG8="},
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"cell"}]}]}]}]},
		{"type":"list","items":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"item1"}]}]}]}
	]}]}`

	got := buildDOCXJSONSections(irJSON, nil)
	want := []map[string]any{
		{"text": "Title", "doc_type_kwd": "text", "layout": "heading"},
		{"text": "Hello", "doc_type_kwd": "text"},
		// The table sits under heading "Title", so it carries the
		// heading-location caption (mirrors Python __get_nearest_title).
		{"doc_type_kwd": "table", "table": &entity.TableData{Grid: [][]entity.TableCell{{{Text: "cell", ColSpan: 1, RowSpan: 1}}}, HeaderRows: 1, Caption: "Table Location: Title"}},
		{"text": "item1", "doc_type_kwd": "text"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildDOCXJSONSections mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

// TestBuildDOCXJSONSections_EmptyTableSkipped verifies that a table
// with no rows is dropped before it can become a section.
func TestBuildDOCXJSONSections_EmptyTableSkipped(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"keep"}]},
		{"type":"table","rows":[]}
	]}]}`
	got := buildDOCXJSONSections(irJSON, nil)
	if len(got) != 1 || got[0]["text"] != "keep" {
		t.Fatalf("expected only the paragraph to survive, got %+v", got)
	}
}

// TestBuildDOCXJSONSections_PreservesMergedHeaderSpans pins that office_oxide's
// row/column spans and repeated-header flags survive the IR -> TableData
// projection. The header is two rows: row 0 carries the colspan/rowspan
// geometry, row 1 fills the seven columns the vertical merges leave open, and
// the body has ten columns.
func TestBuildDOCXJSONSections_PreservesMergedHeaderSpans(t *testing.T) {
	cell := func(text string, colSpan, rowSpan int) docxIRCell {
		return docxIRCell{
			Content: []docxIRElement{para(text)},
			ColSpan: colSpan,
			RowSpan: rowSpan,
		}
	}
	row := func(header bool, cells ...docxIRCell) docxIRRow {
		return docxIRRow{IsHeader: header, Cells: cells}
	}
	ir := docxIRDocument{Sections: []docxIRSection{{
		Elements: []docxIRElement{
			{Type: "heading", Level: 1, Content: rawJSON([]docxIRRun{{Type: "text", Text: "H"}})},
			{Type: "table", Rows: []docxIRRow{
				row(true,
					cell("单位名称", 1, 2),
					cell("集团公司采用", 3, 1),
					cell("中国中铁采用", 2, 1),
					cell("国资委采用", 2, 1),
					cell("得分", 1, 2),
					cell("排名", 1, 2),
				),
				row(true,
					cell("政务", 1, 1),
					cell("政工", 1, 1),
					cell("得分", 1, 1),
					cell("数量", 1, 1),
					cell("得分", 1, 1),
					cell("数量", 1, 1),
					cell("得分", 1, 1),
				),
				row(false,
					cell("西安公司", 1, 1),
					cell("15（1）", 1, 1),
					cell("12", 1, 1),
					cell("265", 1, 1),
					cell("4（3）", 1, 1),
					cell("50", 1, 1),
					cell("2（2）", 1, 1),
					cell("40", 1, 1),
					cell("355", 1, 1),
					cell("1", 1, 1),
				),
			}},
		},
	}}}
	encoded, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}

	got := buildDOCXJSONSections(string(encoded), nil)
	var tables []map[string]any
	for _, it := range got {
		if dt, _ := it["doc_type_kwd"].(string); dt == "table" {
			tables = append(tables, it)
		}
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table item, got %d: %#v", len(tables), got)
	}
	td, ok := tables[0]["table"].(*entity.TableData)
	if !ok || td == nil {
		t.Fatalf("table has no TableData: %#v", tables[0])
	}
	if td.HeaderRows != 2 {
		t.Fatalf("HeaderRows = %d, want 2", td.HeaderRows)
	}
	if td.Caption != "Table Location: H" {
		t.Errorf("caption = %q, want %q", td.Caption, "Table Location: H")
	}
	if len(td.Grid) != 3 {
		t.Fatalf("grid rows = %d, want 3", len(td.Grid))
	}
	for i, row := range td.Grid {
		if len(row) != 10 {
			t.Fatalf("grid row %d width = %d, want 10: %#v", i, len(row), row)
		}
	}
	if got := td.Grid[0][0]; got.Text != "单位名称" || got.RowSpan != 2 {
		t.Errorf("header cell 0 = %#v, want 单位名称 rowspan 2", got)
	}
	if got := td.Grid[0][1]; got.Text != "集团公司采用" || got.ColSpan != 3 {
		t.Errorf("header cell 1 = %#v, want 集团公司采用 colspan 3", got)
	}
	if got := td.Grid[0][4]; got.Text != "中国中铁采用" || got.ColSpan != 2 {
		t.Errorf("header cell at col 4 = %#v, want 中国中铁采用 colspan 2", got)
	}
	if got := td.Grid[0][6]; got.Text != "国资委采用" || got.ColSpan != 2 {
		t.Errorf("header cell at col 6 = %#v, want 国资委采用 colspan 2", got)
	}
	if got := td.Grid[0][8]; got.Text != "得分" || got.RowSpan != 2 {
		t.Errorf("header cell at col 8 = %#v, want 得分 rowspan 2", got)
	}
	if got := td.Grid[0][9]; got.Text != "排名" || got.RowSpan != 2 {
		t.Errorf("header cell at col 9 = %#v, want 排名 rowspan 2", got)
	}
	for _, col := range []int{2, 3, 5, 7} {
		if got := td.Grid[0][col]; !got.Covered {
			t.Errorf("row 0 col %d = %#v, want covered by a colspan", col, got)
		}
	}
	if got := td.Grid[1][0]; !got.Covered {
		t.Errorf("row 1 col 0 = %#v, want covered by the rowspan", got)
	}
	if got := td.Grid[1][1]; got.Text != "政务" {
		t.Errorf("row 1 col 1 = %#v, want 政务", got)
	}
}

// TestBuildDOCXJSONSections_DifferentColumnTablesStaySeparate pins that two
// adjacent tables of different column counts remain separate.
func TestBuildDOCXJSONSections_DifferentColumnTablesStaySeparate(t *testing.T) {
	const cell = `{"content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}`
	irJSON := fmt.Sprintf(`{"sections":[{"title":"","elements":[
		{"type":"table","rows":[{"cells":[%s,%s,%s]}]},
		{"type":"table","rows":[{"cells":[%s,%s]},{"cells":[%s,%s]}]}
	]}]}`, cell, cell, cell, cell, cell, cell, cell)
	got := buildDOCXJSONSections(irJSON, nil)
	var n int
	for _, it := range got {
		if dt, _ := it["doc_type_kwd"].(string); dt == "table" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("expected 2 separate tables (different column counts), got %d: %#v", n, got)
	}
}

// TestBuildDOCXJSONSections_AdjacentTablesStaySeparate pins that adjacent
// tables remain separate items. The parser no longer merges table elements;
// office_oxide now emits one element per logical table.
func TestBuildDOCXJSONSections_AdjacentTablesStaySeparate(t *testing.T) {
	const cell = `{"content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}`
	irJSON := fmt.Sprintf(`{"sections":[{"title":"","elements":[
		{"type":"table","rows":[{"cells":[%s,%s,%s]},{"cells":[%s,%s,%s]}]},
		{"type":"table","rows":[{"cells":[%s,%s,%s]},{"cells":[%s,%s,%s]}]}
	]}]}`, cell, cell, cell, cell, cell, cell, cell, cell, cell, cell, cell, cell)
	got := buildDOCXJSONSections(irJSON, nil)
	var tables []map[string]any
	for _, it := range got {
		if dt, _ := it["doc_type_kwd"].(string); dt == "table" {
			tables = append(tables, it)
		}
	}
	if len(tables) != 2 {
		t.Fatalf("expected 2 separate tables, got %d: %#v", len(tables), got)
	}
	for i, tb := range tables {
		td, ok := tb["table"].(*entity.TableData)
		if !ok || td == nil {
			t.Fatalf("table %d has no TableData: %#v", i, tb)
		}
		if len(td.Grid) != 2 {
			t.Errorf("table %d should keep its 2 rows, got %d: %#v", i, len(td.Grid), td.Grid)
		}
	}
}

func TestBuildDOCXJSONSections_PreservesInlineImageOrder(t *testing.T) {
	imageData := officeTestPNG(t, 1)
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[
			{"type":"text","text":"before"},
			{"type":"image","data":"` + base64.StdEncoding.EncodeToString(imageData) + `"},
			{"type":"text","text":"after"}
		]}
	]}]}`
	sink, cache := officeMediaTestSink(t, officeMediaFormatDOCX)
	got := buildDOCXJSONSections(irJSON, sink)
	if sink.err != nil {
		t.Fatalf("sink error: %v", sink.err)
	}
	if len(got) != 3 {
		t.Fatalf("sections = %+v, want text/image/text sequence", got)
	}
	if got[0]["text"] != "before" || got[0]["doc_type_kwd"] != "text" {
		t.Errorf("first item = %+v, want text before", got[0])
	}
	ref := requireOfficeMediaRef(t, got[1])
	if ref.Format != officeMediaFormatDOCX || ref.MIME != "image/png" {
		t.Errorf("media_ref = %+v, want docx image/png", ref)
	}
	if len(cache.puts) != 1 || !bytes.Equal(cache.puts[0].data, imageData) {
		t.Fatalf("cache writes = %+v, want the inline image bytes", cache.puts)
	}
	if got[2]["text"] != "after" || got[2]["doc_type_kwd"] != "text" {
		t.Errorf("last item = %+v, want text after", got[2])
	}
}

func TestBuildDOCXJSONSections_ExtractsTableCellImages(t *testing.T) {
	first, second := officeTestPNG(t, 1), officeTestPNG(t, 2)
	irJSON := `{"sections":[{"elements":[
		{"type":"table","rows":[{"cells":[
			{"content":[{"type":"paragraph","content":[
				{"type":"text","text":"before"},
				{"type":"image","data":"` + base64.StdEncoding.EncodeToString(first) + `"},
				{"type":"text","text":"after"}
			]}]},
			{"content":[{"type":"image","data":"` + base64.StdEncoding.EncodeToString(second) + `"}]}
		]}]}
	]}]}`

	sink, cache := officeMediaTestSink(t, officeMediaFormatDOCX)
	got := buildDOCXJSONSections(irJSON, sink)
	if sink.err != nil {
		t.Fatalf("sink error: %v", sink.err)
	}
	if len(got) != 3 {
		t.Fatalf("sections = %+v, want table plus two cell image items", got)
	}
	if got[0]["doc_type_kwd"] != "table" || got[0]["source_table_id"] != "docx-table-1" {
		t.Fatalf("table item = %+v", got[0])
	}
	if _, ok := got[0]["table"].(*entity.TableData); !ok {
		t.Fatalf("table item missing structured TableData: %+v", got[0])
	}
	if len(cache.puts) != 2 {
		t.Fatalf("cache writes = %d, want one per cell image", len(cache.puts))
	}
	for i, want := range [][]byte{first, second} {
		item := got[i+1]
		if item["doc_type_kwd"] != "image" {
			t.Errorf("image item %d = %+v", i, item)
		}
		requireOfficeMediaRef(t, item)
		if !bytes.Equal(cache.puts[i].data, want) {
			t.Errorf("cache write %d payload mismatch", i)
		}
		if item["parent_table_id"] != "docx-table-1" || item["row_index"] != 1 || item["column_index"] != i+1 || item["media_order"] != i+1 {
			t.Errorf("image item %d metadata = %+v", i, item)
		}
	}
}

// TestJoinDOCXIRRuns pins that only text-type runs are concatenated;
// non-text runs (e.g. nested image runs) are skipped.
func TestJoinDOCXIRRuns(t *testing.T) {
	runs := []docxIRRun{
		{Type: "text", Text: "Hello"},
		{Type: "image", Text: "ignored"},
		{Type: "text", Text: " World"},
	}
	if got := joinDOCXIRRuns(runs); got != "Hello World" {
		t.Fatalf("joinDOCXIRRuns = %q, want %q", got, "Hello World")
	}
	if got := joinDOCXIRRuns(nil); got != "" {
		t.Fatalf("joinDOCXIRRuns(nil) = %q, want empty", got)
	}
}

// TestJoinDOCXIRRuns_LineBreak pins that a hard line break inline run
// renders as a newline instead of being dropped.
func TestJoinDOCXIRRuns_LineBreak(t *testing.T) {
	runs := []docxIRRun{
		{Type: "text", Text: "before"},
		{Type: "line_break"},
		{Type: "text", Text: "after"},
	}
	if got := joinDOCXIRRuns(runs); got != "before\nafter" {
		t.Fatalf("joinDOCXIRRuns = %q, want %q", got, "before\nafter")
	}
}

// rawJSON marshals v and returns it as json.RawMessage, for populating
// docxIRElement.Content in tests. Defined here (tagless) so it is
// available to both cgo and !cgo test files in package parser.
func rawJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}

// para is a test helper returning a paragraph element whose single text
// run carries the given string.
func para(text string) docxIRElement {
	return docxIRElement{
		Type:    "paragraph",
		Content: rawJSON([]docxIRRun{{Type: "text", Text: text}}),
	}
}

// TestExtractTextFromListItem_Nested verifies that nested sub-list text
// is decoded and recursed, not silently dropped. The IR shape mirrors
// office_oxide ir::ListItem { content, nested: Option<List> } where
// List { items: [ListItem, ...] } is recursive.
func TestExtractTextFromListItem_Nested(t *testing.T) {
	// Top-level item "Top" carries a nested sub-list with two children
	// "Child A" and "Child B"; Child B itself has a deeper sub-list
	// "Grandchild" to exercise multi-level recursion.
	item := docxIRListItem{
		Content: []docxIRElement{para("Top")},
		Nested: &docxIRList{Items: []docxIRListItem{
			{Content: []docxIRElement{para("Child A")}},
			{
				Content: []docxIRElement{para("Child B")},
				Nested: &docxIRList{Items: []docxIRListItem{
					{Content: []docxIRElement{para("Grandchild")}},
				}},
			},
		}},
	}
	got := extractTextFromListItem(item)
	want := "Top\nChild A\nChild B\nGrandchild"
	if got != want {
		t.Errorf("extractTextFromListItem(nested) = %q, want %q", got, want)
	}
}

// TestExtractTextFromListItem_NestedNull pins that "nested": null (the
// serde serialization of Option::None) does not trip recursion and the
// item's own text still comes through.
func TestExtractTextFromListItem_NestedNull(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"list","items":[
			{"content":[{"type":"paragraph","content":[{"type":"text","text":"only item"}]}],"nested":null}
		]}
	]}]}`
	sections := buildDOCXJSONSections(irJSON, nil)
	if len(sections) != 1 || sections[0]["text"] != "only item" {
		t.Fatalf("expected single item \"only item\", got %+v", sections)
	}
}

// TestExtractTextFromListItem_NestedTable verifies that a table nested
// inside a list item remains structured beside the preceding text.
func TestExtractTextFromListItem_NestedTable(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"list","items":[
			{"content":[
				{"type":"paragraph","content":[{"type":"text","text":"before"}]},
				{"type":"table","rows":[
					{"cells":[
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"c1"}]}]},
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"c2"}]}]}
					]}
				]}
			]}
		]}
	]}]}`
	sections := buildDOCXJSONSections(irJSON, nil)
	if len(sections) != 2 || sections[0]["text"] != "before" {
		t.Fatalf("sections=%#v", sections)
	}
	requireTableRows(t, sections[1], [][]string{{"c1", "c2"}})
}

// TestJoinCellText_NestedList verifies that a list nested inside a table
// cell is flattened, not silently dropped. The old path only recognized
// paragraph runs in cell.Content.
func TestJoinCellText_NestedList(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"table","rows":[
			{"cells":[
				{"content":[
					{"type":"paragraph","content":[{"type":"text","text":"head"}]},
					{"type":"list","items":[
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"li1"}]}]},
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"li2"}]}]}
					]}
				]}
			]}
		]}
	]}]}`
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		t.Fatalf("unmarshal IR: %v", err)
	}
	cell := ir.Sections[0].Elements[0].Rows[0].Cells[0]
	if got := joinCellText(cell); got != "head\nli1\nli2" {
		t.Errorf("joinCellText(nested list) = %q, want %q", got, "head\nli1\nli2")
	}
}

// TestBuildDOCXJSONSections_TextBoxTable verifies end-to-end that a table
// wrapped in a text_box (the shape office_oxide emits for grouped slide
// shapes, and a legal DOCX text-box shape) survives into the section
// output. Mirrors the PPTX bwbd.pptx regression at the DOCX layer.
func TestBuildDOCXJSONSections_TextBoxTable(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"text_box","content":[
			{"type":"table","rows":[
				{"cells":[
					{"content":[{"type":"paragraph","content":[{"type":"text","text":"Q1"}]}]},
					{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]}
				]}
			]}
		]}
	]}]}`
	sections := buildDOCXJSONSections(irJSON, nil)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d: %+v", len(sections), sections)
	}
	requireTableRows(t, sections[0], [][]string{{"Q1", "A1"}})
}

// TestBuildDOCXJSONSections_NestedList verifies end-to-end that a
// multi-level list survives into the section output with all levels.
func TestBuildDOCXJSONSections_NestedList(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"list","items":[
			{"content":[{"type":"paragraph","content":[{"type":"text","text":"L1"}]}],
			 "nested":{"items":[
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"L2a"}]}]},
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"L2b"}]}]}
			 ]}}
		]}
	]}]}`
	sections := buildDOCXJSONSections(irJSON, nil)
	if len(sections) != 3 {
		t.Fatalf("sections=%#v", sections)
	}
	for i, want := range []string{"L1", "L2a", "L2b"} {
		if sections[i]["text"] != want {
			t.Fatalf("section[%d]=%#v", i, sections[i])
		}
	}
}

func TestBuildDOCXJSONSections_EmitsListAndTextBoxImages(t *testing.T) {
	images := [][]byte{officeTestPNG(t, 1), officeTestPNG(t, 2), officeTestPNG(t, 3)}
	irJSON := `{"sections":[{"elements":[
		{"type":"list","items":[
			{"content":[{"type":"paragraph","content":[
				{"type":"text","text":"list text"},
				{"type":"image","data":"` + base64.StdEncoding.EncodeToString(images[0]) + `"}
			]}],"nested":{"items":[
				{"content":[{"type":"paragraph","content":[
					{"type":"image","data":"` + base64.StdEncoding.EncodeToString(images[1]) + `"}
				]}]}
			]}}
		]},
		{"type":"text_box","content":[{"type":"paragraph","content":[
			{"type":"text","text":"text box text"},
			{"type":"image","data":"` + base64.StdEncoding.EncodeToString(images[2]) + `"}
		]}]}
	]}]}`
	sink, cache := officeMediaTestSink(t, officeMediaFormatDOCX)
	sections := buildDOCXJSONSections(irJSON, sink)
	if sink.err != nil {
		t.Fatalf("sink error: %v", sink.err)
	}
	want := []struct {
		docType string
		text    string
		image   int
	}{
		{docType: "text", text: "list text"},
		{docType: "image", image: 0},
		{docType: "image", image: 1},
		{docType: "text", text: "text box text"},
		{docType: "image", image: 2},
	}
	if len(sections) != len(want) {
		t.Fatalf("sections = %+v, want %d items", sections, len(want))
	}
	for i, expected := range want {
		if got := sections[i]["doc_type_kwd"]; got != expected.docType {
			t.Errorf("section %d type = %v, want %q", i, got, expected.docType)
		}
		if expected.text != "" && sections[i]["text"] != expected.text {
			t.Errorf("section %d text = %v, want %q", i, sections[i]["text"], expected.text)
		}
		if expected.docType == "image" {
			requireOfficeMediaRef(t, sections[i])
			if !bytes.Equal(cache.puts[expected.image].data, images[expected.image]) {
				t.Errorf("section %d image payload mismatch", i)
			}
		}
	}
}

func TestDOCXImageTraversalStopsAfterFatalFailure(t *testing.T) {
	elements := []docxIRElement{
		{Type: "table", Rows: []docxIRRow{{Cells: []docxIRCell{{Content: []docxIRElement{{Type: "image", Data: []byte("first")}, {Type: "image", Data: []byte("second")}}}}}}},
		{Type: "image", Data: []byte("third")},
	}
	visits := 0
	completed := forEachDOCXIRImage(elements, func([]byte) bool { visits++; return false })
	if completed || visits != 1 {
		t.Fatalf("completed=%v visits=%d; want false, 1", completed, visits)
	}
}
