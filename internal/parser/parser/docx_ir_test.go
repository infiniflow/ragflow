package parser

import (
	"encoding/json"
	"reflect"
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

	got := buildDOCXJSONSections(irJSON)
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
	got := buildDOCXJSONSections(irJSON)
	if len(got) != 1 || got[0]["text"] != "keep" {
		t.Fatalf("expected only the paragraph to survive, got %+v", got)
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

// TestExtractDOCXFiguresFromIR verifies the image-figure context
// extraction: an image block carries the immediately surrounding text
// as ContextAbove / ContextBelow / Marker. Pure-Go, runs under !cgo.
func TestExtractDOCXFiguresFromIR(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"before"}]},
		{"type":"image","data":"aGVsbG8="},
		{"type":"paragraph","content":[{"type":"text","text":"after"}]}
	]}]}`

	figs := extractDOCXFiguresFromIR(irJSON)
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

// TestExtractDOCXFiguresFromIR_NoImage returns nil when the IR has no
// image blocks.
func TestExtractDOCXFiguresFromIR_NoImage(t *testing.T) {
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"only text"}]}
	]}]}`
	if figs := extractDOCXFiguresFromIR(irJSON); figs != nil {
		t.Fatalf("expected nil, got %+v", figs)
	}
}

// TestExtractDOCXFiguresFromIR_BadJSON returns nil on unparseable IR,
// mirroring the json.Unmarshal error guard.
func TestExtractDOCXFiguresFromIR_BadJSON(t *testing.T) {
	if figs := extractDOCXFiguresFromIR("{not json"); figs != nil {
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
	figs := extractDOCXFiguresFromIR(irJSON)
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
	flat = append(flat, flatBlock{image: "img"})
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
	sections := buildDOCXJSONSections(irJSON)
	if len(sections) != 1 || sections[0]["text"] != "only item" {
		t.Fatalf("expected single item \"only item\", got %+v", sections)
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
	sections := buildDOCXJSONSections(irJSON)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d: %+v", len(sections), sections)
	}
	if got := sections[0]["text"]; got != "L1\nL2a\nL2b" {
		t.Errorf("nested list text = %q, want %q", got, "L1\nL2a\nL2b")
	}
}
