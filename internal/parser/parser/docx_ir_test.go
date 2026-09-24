package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestBuildDOCXJSONSections_FromJSON feeds an office_oxide IR JSON
// string directly into buildDOCXJSONSections and asserts the
// paragraph / heading / image / table / list branches. This pins the
// pure-Go IR → sections transform without requiring the office_oxide
// native library, so it runs under CGO_ENABLED=0.
func TestBuildDOCXJSONSections_FromJSON(t *testing.T) {
	// "data":"aGVsbG8=" is base64 for "hello"; buildDOCXJSONSections
	// re-encodes el.Data, so the item's "image" must equal "aGVsbG8=".
	irJSON := `{"sections":[{"title":"","elements":[
		{"type":"heading","level":1,"content":[{"type":"text","text":"Title"}]},
		{"type":"paragraph","content":[{"type":"text","text":"Hello"}]},
		{"type":"image","data":"aGVsbG8="},
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"cell"}]}]}]}]},
		{"type":"list","items":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"item1"}]}]}]}
	]}]}`

	got := buildDOCXJSONSections(irJSON, newEmbeddedMediaBudget())
	want := []map[string]any{
		{"text": "Title", "image": nil, "doc_type_kwd": "text", "ck_type": "heading"},
		{"text": "Hello", "image": nil, "doc_type_kwd": "text"},
		{"text": "", "image": "aGVsbG8=", "doc_type_kwd": "image"},
		{"text": "<table><tr><td>cell</td></tr></table>", "image": nil, "doc_type_kwd": "table"},
		{"text": "item1", "image": nil, "doc_type_kwd": "text"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildDOCXJSONSections mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

// TestBuildDOCXJSONSections_EmptyTableSkipped verifies that a table
// with no rows renders as "<table></table>" and is dropped by the
// "if html == \"<table></table>\" { continue }" guard.
func TestBuildDOCXJSONSections_EmptyTableSkipped(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"keep"}]},
		{"type":"table","rows":[]}
	]}]}`
	got := buildDOCXJSONSections(irJSON, newEmbeddedMediaBudget())
	if len(got) != 1 || got[0]["text"] != "keep" {
		t.Fatalf("expected only the paragraph to survive, got %+v", got)
	}
}

func TestBuildDOCXJSONSections_PreservesInlineImageOrder(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[
			{"type":"text","text":"before"},
			{"type":"image","data":"aGVsbG8="},
			{"type":"text","text":"after"}
		]}
	]}]}`
	got := buildDOCXJSONSections(irJSON, newEmbeddedMediaBudget())
	if len(got) != 3 {
		t.Fatalf("sections = %+v, want text/image/text sequence", got)
	}
	if got[0]["text"] != "before" || got[0]["doc_type_kwd"] != "text" {
		t.Errorf("first item = %+v, want text before", got[0])
	}
	if got[1]["image"] != "aGVsbG8=" || got[1]["doc_type_kwd"] != "image" {
		t.Errorf("inline image item = %+v", got[1])
	}
	if got[2]["text"] != "after" || got[2]["doc_type_kwd"] != "text" {
		t.Errorf("last item = %+v, want text after", got[2])
	}
}

func TestBuildDOCXJSONSections_ExtractsTableCellImages(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"table","rows":[{"cells":[
			{"content":[{"type":"paragraph","content":[
				{"type":"text","text":"before"},
				{"type":"image","data":"aGVsbG8="},
				{"type":"text","text":"after"}
			]}]},
			{"content":[{"type":"image","data":"aW1hZ2U="}]}
		]}]}
	]}]}`

	got := buildDOCXJSONSections(irJSON, newEmbeddedMediaBudget())
	if len(got) != 3 {
		t.Fatalf("sections = %+v, want table plus two cell image items", got)
	}
	if got[0]["doc_type_kwd"] != "table" || got[0]["text"] != "<table><tr><td>beforeafter</td><td></td></tr></table>" || got[0]["source_table_id"] != "docx-table-1" {
		t.Fatalf("table item = %+v", got[0])
	}
	for i, want := range []string{"aGVsbG8=", "aW1hZ2U="} {
		item := got[i+1]
		if item["doc_type_kwd"] != "image" || item["image"] != want {
			t.Errorf("image item %d = %+v, want payload %q", i, item, want)
		}
		if item["parent_table_id"] != "docx-table-1" || item["row_index"] != 1 || item["column_index"] != i+1 || item["media_order"] != i+1 {
			t.Errorf("image item %d metadata = %+v", i, item)
		}
	}
}

func TestBuildDOCXJSONSections_BudgetKeepsInlineTextAroundOmittedImage(t *testing.T) {
	irJSON := `{"sections":[{"elements":[{"type":"paragraph","content":[
		{"type":"text","text":"before"},
		{"type":"image","data":"YWJjZA=="},
		{"type":"text","text":"after"}
	]}]}]}`
	budget := &embeddedMediaBudget{maxImageBytes: 3, maxTotalBytes: 5, maxItems: 10}

	items := buildDOCXJSONSections(irJSON, budget)
	if len(items) != 3 {
		t.Fatalf("items = %+v, want text/image/text with omitted image metadata", items)
	}
	if items[0]["text"] != "before" || items[2]["text"] != "after" {
		t.Fatalf("inline text was lost around omitted image: %+v", items)
	}
	if items[1]["doc_type_kwd"] != "image" || items[1]["image"] != nil || items[1]["media_omitted"] != true {
		t.Fatalf("omitted inline image item = %+v, want metadata without payload", items[1])
	}
	if len(budget.warnings()) == 0 {
		t.Fatal("omitted image should produce a parser warning")
	}
}

func TestBuildDOCXJSONSections_BudgetKeepsTableImageMetadata(t *testing.T) {
	irJSON := `{"sections":[{"elements":[{"type":"table","rows":[{"cells":[
		{"content":[{"type":"image","data":"YWJj"}]},
		{"content":[{"type":"image","data":"ZGVm"}]}
	]}]}]}]}`
	budget := &embeddedMediaBudget{maxImageBytes: 4, maxTotalBytes: 5, maxItems: 10}

	items := buildDOCXJSONSections(irJSON, budget)
	if len(items) != 3 {
		t.Fatalf("items = %+v, want table and two image metadata items", items)
	}
	if items[1]["image"] != "YWJj" || items[1]["media_omitted"] == true {
		t.Fatalf("first table image = %+v, want accepted payload", items[1])
	}
	omitted := items[2]
	if omitted["image"] != nil || omitted["media_omitted"] != true {
		t.Fatalf("second table image = %+v, want omitted payload", omitted)
	}
	if omitted["parent_table_id"] != items[0]["source_table_id"] || omitted["row_index"] != 1 || omitted["column_index"] != 2 || omitted["media_order"] != 2 {
		t.Fatalf("omitted table image lost location metadata: %+v", omitted)
	}
}

func TestBuildDOCXJSONSections_BudgetCapsMediaCount(t *testing.T) {
	var ir strings.Builder
	ir.WriteString(`{"sections":[{"elements":[`)
	for i := 0; i < 257; i++ {
		if i > 0 {
			ir.WriteByte(',')
		}
		ir.WriteString(`{"type":"image","data":"YQ=="}`)
	}
	ir.WriteString(`]}]}`)

	budget := newEmbeddedMediaBudget()
	items := buildDOCXJSONSections(ir.String(), budget)
	imageCount := 0
	for _, item := range items {
		if item["doc_type_kwd"] == "image" {
			imageCount++
		}
	}
	if imageCount != budget.maxItems {
		t.Fatalf("image items = %d, want count limit %d", imageCount, budget.maxItems)
	}
	if len(budget.warnings()) == 0 {
		t.Fatal("truncated media count should produce a parser warning")
	}
}

func TestBuildPPTXJSONSections_BudgetKeepsSlideMetadata(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"slide text"}]},
		{"type":"image","data":"YWJjZA=="}
	]}]}`
	budget := &embeddedMediaBudget{maxImageBytes: 3, maxTotalBytes: 5, maxItems: 10}

	items, err := buildPPTXJSONSections(irJSON, budget)
	if err != nil {
		t.Fatalf("buildPPTXJSONSections: %v", err)
	}
	if len(items) != 2 || items[0]["text"] != "slide text" {
		t.Fatalf("items = %+v, want text and omitted image items", items)
	}
	omitted := items[1]
	if omitted["image"] != nil || omitted["media_omitted"] != true || omitted["slide_number"] != 1 || omitted["media_order"] != 1 {
		t.Fatalf("omitted slide image lost metadata: %+v", omitted)
	}
	if itemsAllEmpty([]map[string]any{omitted}) {
		t.Fatal("an image-only slide with a budget-omitted image must not be replaced by whole-deck fallback")
	}
}

func TestBuildPPTXJSONSections_BudgetCapsMediaCount(t *testing.T) {
	var ir strings.Builder
	ir.WriteString(`{"sections":[{"elements":[`)
	for i := 0; i < 257; i++ {
		if i > 0 {
			ir.WriteByte(',')
		}
		ir.WriteString(`{"type":"image","data":"YQ=="}`)
	}
	ir.WriteString(`]}]}`)

	budget := newEmbeddedMediaBudget()
	items, err := buildPPTXJSONSections(ir.String(), budget)
	if err != nil {
		t.Fatalf("buildPPTXJSONSections: %v", err)
	}
	imageCount := 0
	for _, item := range items {
		if item["doc_type_kwd"] == "image" {
			imageCount++
		}
	}
	if imageCount != budget.maxItems {
		t.Fatalf("image items = %d, want count limit %d", imageCount, budget.maxItems)
	}
	if len(budget.warnings()) == 0 {
		t.Fatal("truncated media count should produce a parser warning")
	}
}

func TestEmbeddedMediaBudgetRejectsPerImageAndDocumentByteLimits(t *testing.T) {
	budget := &embeddedMediaBudget{maxImageBytes: 4, maxTotalBytes: 5, maxItems: 4}
	if included, keepGoing := budget.include([]byte("abc")); !included || !keepGoing {
		t.Fatalf("first image admission = (%v, %v), want (true, true)", included, keepGoing)
	}
	if included, keepGoing := budget.include([]byte("def")); included || !keepGoing {
		t.Fatalf("aggregate overflow admission = (%v, %v), want (false, true)", included, keepGoing)
	}
	if included, keepGoing := budget.include([]byte("12345")); included || !keepGoing {
		t.Fatalf("per-image overflow admission = (%v, %v), want (false, true)", included, keepGoing)
	}
	if len(budget.warnings()) == 0 {
		t.Fatal("omitted byte payloads should produce a parser warning")
	}
}

func TestBuildDOCXJSONSectionsEncodesOnlyAcceptedImageData(t *testing.T) {
	data := []byte("text is not a valid image, but the parser preserves it")
	encoded := base64.StdEncoding.EncodeToString(data)
	irJSON := fmt.Sprintf(`{"sections":[{"elements":[{"type":"image","data":%q}]}]}`, encoded)
	budget := &embeddedMediaBudget{maxImageBytes: 8, maxTotalBytes: 8, maxItems: 4}
	items := buildDOCXJSONSections(irJSON, budget)
	if len(items) != 1 || items[0]["image"] != nil || items[0]["media_omitted"] != true {
		t.Fatalf("oversized image items = %+v, want metadata without payload", items)
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

// TestExtractDOCXFiguresFromIR verifies the image-figure context
// extraction: an image block carries the immediately surrounding text
// as ContextAbove / ContextBelow / Marker. Pure-Go, runs under !cgo.
func TestExtractDOCXFiguresFromIR(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"before"}]},
		{"type":"image","data":"aGVsbG8="},
		{"type":"paragraph","content":[{"type":"text","text":"after"}]}
	]}]}`

	figs := extractDOCXFiguresFromIR(irJSON, newEmbeddedMediaBudget())
	if len(figs) != 1 {
		t.Fatalf("expected 1 figure, got %d", len(figs))
	}
	fig := figs[0]
	want := DOCXFigure{
		Image:        "aGVsbG8=",
		ContextAbove: "before",
		ContextBelow: "after",
		Marker:       "before",
	}
	if !reflect.DeepEqual(fig, want) {
		t.Fatalf("figure mismatch:\n got: %+v\nwant: %+v", fig, want)
	}
}

func TestExtractDOCXFiguresFromIR_InlineImage(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[
			{"type":"text","text":"before"},
			{"type":"image","data":"aGVsbG8="},
			{"type":"text","text":"after"}
		]}
	]}]}`
	figs := extractDOCXFiguresFromIR(irJSON, newEmbeddedMediaBudget())
	if len(figs) != 1 {
		t.Fatalf("expected 1 inline figure, got %d", len(figs))
	}
	if figs[0].Image != "aGVsbG8=" || figs[0].ContextAbove != "before" || figs[0].ContextBelow != "after" {
		t.Fatalf("inline figure = %+v", figs[0])
	}
}

func TestExtractDOCXFiguresFromIR_BudgetKeepsContextWithoutPayload(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"before"}]},
		{"type":"image","data":"YWJjZA=="},
		{"type":"paragraph","content":[{"type":"text","text":"after"}]}
	]}]}`
	budget := &embeddedMediaBudget{maxImageBytes: 3, maxTotalBytes: 5, maxItems: 10}

	figures := extractDOCXFiguresFromIR(irJSON, budget)
	if len(figures) != 1 {
		t.Fatalf("figures = %+v, want one metadata record", figures)
	}
	if figures[0].Image != "" || figures[0].ContextAbove != "before" || figures[0].ContextBelow != "after" || figures[0].Marker != "before" {
		t.Fatalf("budgeted figure = %+v, want context metadata without payload", figures[0])
	}
}

// TestExtractDOCXFiguresFromIR_NoImage returns nil when the IR has no
// image blocks.
func TestExtractDOCXFiguresFromIR_NoImage(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"only text"}]}
	]}]}`
	if figs := extractDOCXFiguresFromIR(irJSON, newEmbeddedMediaBudget()); figs != nil {
		t.Fatalf("expected nil, got %+v", figs)
	}
}

// TestExtractDOCXFiguresFromIR_BadJSON returns nil on unparseable IR,
// mirroring the json.Unmarshal error guard.
func TestExtractDOCXFiguresFromIR_BadJSON(t *testing.T) {
	if figs := extractDOCXFiguresFromIR("{not json", newEmbeddedMediaBudget()); figs != nil {
		t.Fatalf("expected nil for bad JSON, got %+v", figs)
	}
}

// TestExtractDOCXFiguresFromIR_NonParagraphContext verifies that
// tables, lists, and text boxes adjacent to an image contribute their
// text to ContextAbove / ContextBelow. Previously the flatten loop
// called joinDOCXIRRuns(el.contentRuns()) for every non-image element,
// which returned "" for table/list/text_box (their Content is not text
// runs), silently dropping the surrounding VLM context.
func TestExtractDOCXFiguresFromIR_NonParagraphContext(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"table cell"}]}]}]}]},
		{"type":"image","data":"aGVsbG8="},
		{"type":"list","items":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"list item"}]}]}]},
		{"type":"text_box","content":[{"type":"paragraph","content":[{"type":"text","text":"box text"}]}]}
	]}]}`
	figs := extractDOCXFiguresFromIR(irJSON, newEmbeddedMediaBudget())
	if len(figs) != 1 {
		t.Fatalf("expected 1 figure, got %d", len(figs))
	}
	fig := figs[0]
	if fig.ContextAbove != "table cell" {
		t.Errorf("ContextAbove = %q, want %q", fig.ContextAbove, "table cell")
	}
	// Below the image: list item then text_box, joined by newline.
	if fig.ContextBelow != "list item\nbox text" {
		t.Errorf("ContextBelow = %q, want %q", fig.ContextBelow, "list item\nbox text")
	}
}

// TestCollectDOCXText_BoundedByMaxLen pins that the joined context
// never exceeds maxLen runes, even though newline separators are
// inserted between blocks (they are not counted by the per-block
// `remaining` decrement). Previously 512 one-rune blocks produced
// 1023 chars (512 runes + 511 separators).
func TestCollectDOCXText_BoundedByMaxLen(t *testing.T) {
	// 600 one-rune text blocks before and after an image block.
	const maxLen = 512
	var flat []flatBlock
	for i := 0; i < 600; i++ {
		flat = append(flat, flatBlock{text: "x"})
	}
	imgIdx := len(flat)
	flat = append(flat, flatBlock{imageData: []byte("img")})
	for i := 0; i < 600; i++ {
		flat = append(flat, flatBlock{text: "x"})
	}

	prev := collectDOCXPrevText(flat, imgIdx, maxLen)
	next := collectDOCXNextText(flat, imgIdx, maxLen)
	if r := []rune(prev); len(r) > maxLen {
		t.Errorf("prev context = %d runes, want <= %d", len(r), maxLen)
	}
	if r := []rune(next); len(r) > maxLen {
		t.Errorf("next context = %d runes, want <= %d", len(r), maxLen)
	}
	// Both must still carry content (the closest blocks survive).
	if prev == "" {
		t.Error("prev context empty; expected some text")
	}
	if next == "" {
		t.Error("next context empty; expected some text")
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
	sections := buildDOCXJSONSections(irJSON, newEmbeddedMediaBudget())
	if len(sections) != 1 || sections[0]["text"] != "only item" {
		t.Fatalf("expected single item \"only item\", got %+v", sections)
	}
}

// TestExtractTextFromListItem_NestedTable verifies that a table nested
// inside a list item's content is flattened, not silently dropped. The old
// path only recognized paragraph/heading blocks in item.Content.
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
	sections := buildDOCXJSONSections(irJSON, newEmbeddedMediaBudget())
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d: %+v", len(sections), sections)
	}
	if got := sections[0]["text"]; got != "before\nc1\nc2" {
		t.Errorf("list item with nested table text = %q, want %q", got, "before\nc1\nc2")
	}
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
	sections := buildDOCXJSONSections(irJSON, newEmbeddedMediaBudget())
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d: %+v", len(sections), sections)
	}
	if got := sections[0]["text"]; got != "Q1\nA1" {
		t.Errorf("text_box table text = %q, want %q", got, "Q1\nA1")
	}
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
	sections := buildDOCXJSONSections(irJSON, newEmbeddedMediaBudget())
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d: %+v", len(sections), sections)
	}
	if got := sections[0]["text"]; got != "L1\nL2a\nL2b" {
		t.Errorf("nested list text = %q, want %q", got, "L1\nL2a\nL2b")
	}
}
