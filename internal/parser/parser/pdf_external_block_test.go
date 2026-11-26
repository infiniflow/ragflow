package parser

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func externalBlockItems(t *testing.T, opts ExternalBlockOptions, blocks ...ExternalBlock) []map[string]any {
	t.Helper()
	items, _ := ExternalBlocksToItems(blocks, opts)
	return ItemsToMaps(items)
}

func TestExternalBlockPosition(t *testing.T) {
	letter := map[int][2]float64{1: {612, 792}}
	// A 600x800 pixel raster of a 300x400 point page.
	raster := ExternalBlockOptions{PageSizes: map[int][2]float64{3: {300, 400}}, SourceSizes: map[int][2]float64{3: {600, 800}}}
	lenientRaster := raster
	lenientRaster.KeepUnscaled = true
	for _, tc := range []struct {
		name   string
		opts   ExternalBlockOptions
		region ExternalRegion
		want   []any
	}{
		{"points need no page size", ExternalBlockOptions{}, ExternalRegion{Page: 2, Box: [4]float64{10, 20, 110, 40}}, []any{2, 10.0, 110.0, 20.0, 40.0}},
		{"points are clamped to a known page", ExternalBlockOptions{PageSizes: letter}, ExternalRegion{Page: 1, Box: [4]float64{-20, 700, 620, 800}}, []any{1, 0.0, 612.0, 700.0, 792.0}},
		{"bottom-left origin is flipped", ExternalBlockOptions{PageSizes: letter}, ExternalRegion{Page: 1, Box: [4]float64{100, 50, 200, 80}, BottomLeft: true}, []any{1, 100.0, 200.0, 712.0, 742.0}},
		{"bottom-left origin flips with height alone", ExternalBlockOptions{PageSizes: map[int][2]float64{1: {0, 792}}}, ExternalRegion{Page: 1, Box: [4]float64{100, 50, 700, 80}, BottomLeft: true}, []any{1, 100.0, 700.0, 712.0, 742.0}},
		{"bottom-left origin needs the page height", ExternalBlockOptions{}, ExternalRegion{Page: 1, Box: [4]float64{100, 50, 200, 80}, BottomLeft: true}, nil},
		{"bleed inside the tolerance is clamped", ExternalBlockOptions{PageSizes: letter, BleedTolerance: 0.1}, ExternalRegion{Page: 1, Box: [4]float64{-20, 12, 620, 92}}, []any{1, 0.0, 612.0, 12.0, 92.0}},
		{"bleed beyond the tolerance is another space", ExternalBlockOptions{PageSizes: letter, BleedTolerance: 0.1}, ExternalRegion{Page: 1, Box: [4]float64{0, 0, 1275, 1650}}, nil},
		{"bleed at tolerance with non-integer scaling is clamped", ExternalBlockOptions{
			PageSizes:      map[int][2]float64{1: {612, 792}},
			SourceSizes:    map[int][2]float64{1: {1000, 2000}},
			BleedTolerance: 0.1,
		}, ExternalRegion{Page: 1, Box: [4]float64{-100, -200, 1100, 2200}}, []any{1, 0.0, 612.0, 0.0, 792.0}},
		{"bleed beyond tolerance with scaling is dropped", ExternalBlockOptions{
			PageSizes:      map[int][2]float64{1: {612, 792}},
			SourceSizes:    map[int][2]float64{1: {1000, 2000}},
			BleedTolerance: 0.1,
		}, ExternalRegion{Page: 1, Box: [4]float64{-101, -200, 1100, 2200}}, nil},
		{"pixels are scaled to points", raster, ExternalRegion{Page: 3, Box: [4]float64{30, 40, 300, 160}}, []any{3, 15.0, 150.0, 20.0, 80.0}},
		{"normalized box scales by the page alone", ExternalBlockOptions{PageSizes: map[int][2]float64{2: {300, 400}}, SourceSizes: map[int][2]float64{2: {1, 1}}}, ExternalRegion{Page: 2, Box: [4]float64{0.25, 0.5, 0.75, 0.75}}, []any{2, 75.0, 225.0, 200.0, 300.0}},
		{"scaled box is clipped to the page", raster, ExternalRegion{Page: 3, Box: [4]float64{-30, -40, 1200, 1000}}, []any{3, 0.0, 300.0, 0.0, 400.0}},
		{"box outside the page has no area left", raster, ExternalRegion{Page: 3, Box: [4]float64{1000, 900, 1100, 1000}}, nil},
		{"unknown scale drops the box", raster, ExternalRegion{Page: 4, Box: [4]float64{30, 40, 300, 160}}, nil},
		{"unknown scale keeps the box on request", lenientRaster, ExternalRegion{Page: 4, Box: [4]float64{30, 40, 300, 160}}, []any{4, 30.0, 300.0, 40.0, 160.0}},
		// A kept box stays in the service's units, so its origin is turned
		// against the source height, never against the page height in points.
		{"kept bottom-left box is flipped in its own space", ExternalBlockOptions{SourceSizes: map[int][2]float64{4: {600, 800}}, KeepUnscaled: true}, ExternalRegion{Page: 4, Box: [4]float64{30, 40, 300, 160}, BottomLeft: true}, []any{4, 30.0, 300.0, 640.0, 760.0}},
		{"kept bottom-left box needs the source height", ExternalBlockOptions{PageSizes: map[int][2]float64{4: {300, 400}}, SourceSizes: map[int][2]float64{}, KeepUnscaled: true}, ExternalRegion{Page: 4, Box: [4]float64{30, 40, 300, 160}, BottomLeft: true}, nil},
		{"zero area", raster, ExternalRegion{Page: 3, Box: [4]float64{1, 1, 1, 2}}, nil},
		{"non-finite", raster, ExternalRegion{Page: 3, Box: [4]float64{0, 0, math.Inf(1), 100}}, nil},
		{"not a number", ExternalBlockOptions{}, ExternalRegion{Page: 1, Box: [4]float64{0, math.NaN(), 10, 100}}, nil},
		{"inverted edges are rejected", ExternalBlockOptions{}, ExternalRegion{Page: 1, Box: [4]float64{200, 90, 100, 50}}, nil},
		{"inverted edges are reordered on request", ExternalBlockOptions{ReorderEdges: true}, ExternalRegion{Page: 1, Box: [4]float64{200, 90, 100, 50}}, []any{1, 100.0, 200.0, 50.0, 90.0}},
		{"page zero", ExternalBlockOptions{}, ExternalRegion{Box: [4]float64{10, 20, 110, 40}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := newExternalBlockSpace(tc.opts).position(tc.region); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("position = %v, want %v", got, tc.want)
			}
		})
	}
}

// A raster stretched differently from its PDF page was cropped or rotated
// before recognition, so none of its boxes can be trusted.
func TestExternalBlockAspectMismatchDropsThePage(t *testing.T) {
	opts := ExternalBlockOptions{
		Provider:        "OCR",
		PageSizes:       map[int][2]float64{1: {300, 400}, 2: {300, 400}, 3: {300, 400}},
		SourceSizes:     map[int][2]float64{1: {600, 800}, 2: {600, 600}, 3: {604, 800}},
		AspectTolerance: 0.02,
	}
	items, warnings := ExternalBlocksToItems([]ExternalBlock{
		{Text: "matches", Page: 1, Regions: []ExternalRegion{{Page: 1, Box: [4]float64{30, 40, 300, 160}}}},
		{Text: "stretched", Page: 2, Regions: []ExternalRegion{{Page: 2, Box: [4]float64{30, 40, 300, 160}}}},
		{Text: "rounding", Page: 3, Regions: []ExternalRegion{{Page: 3, Box: [4]float64{30, 40, 300, 160}}}},
	}, opts)
	located := []bool{true, false, true}
	mapped := ItemsToMaps(items)
	if len(mapped) != len(located) {
		t.Fatalf("items = %d, want %d: %v", len(mapped), len(located), mapped)
	}
	for i, item := range mapped {
		if _, ok := ExtractPDFPositions(item); ok != located[i] {
			t.Errorf("item %q located = %v, want %v", item["text"], ok, located[i])
		}
		if item["page_number"] != i+1 {
			t.Errorf("item %q page_number = %v, want %d", item["text"], item["page_number"], i+1)
		}
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "OCR page 2") || !strings.Contains(warnings[0], "aspect ratio") {
		t.Fatalf("warnings = %q, want one for page 2", warnings)
	}
}

func TestExternalBlockWarningsOnePerPageInPageOrder(t *testing.T) {
	foreign := func(page int) ExternalBlock {
		return ExternalBlock{Text: "x", Page: page, Regions: []ExternalRegion{{Page: page, Box: [4]float64{0, 0, 2000, 2000}}}}
	}
	_, warnings := ExternalBlocksToItems([]ExternalBlock{foreign(3), foreign(1), foreign(3)}, ExternalBlockOptions{
		Provider:       "Svc",
		PageSizes:      map[int][2]float64{1: {612, 792}, 3: {612, 792}},
		BleedTolerance: 0.1,
	})
	if len(warnings) != 2 || !strings.HasPrefix(warnings[0], "Svc page 1:") || !strings.HasPrefix(warnings[1], "Svc page 3:") {
		t.Fatalf("warnings = %q, want one per page in page order", warnings)
	}
}

func TestExternalBlocksToItemsTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block ExternalBlock
		want  map[string]any
	}{
		{"text", ExternalBlock{Type: ExternalBlockText, Text: " body "}, map[string]any{"doc_type_kwd": "text", "layout": "text", "text": "body"}},
		{"unlisted type is body text", ExternalBlock{Type: "abstract", Text: "body"}, map[string]any{"doc_type_kwd": "text", "layout": "text", "text": "body"}},
		{"zero type is body text", ExternalBlock{Text: "body"}, map[string]any{"doc_type_kwd": "text", "layout": "text", "text": "body"}},
		{"title", ExternalBlock{Type: ExternalBlockTitle, Text: "Scope"}, map[string]any{"doc_type_kwd": "text", "layout": "title", "text": "Scope"}},
		{"title with level", ExternalBlock{Type: ExternalBlockTitle, Text: "Scope", HeadingLevel: 3}, map[string]any{"doc_type_kwd": "text", "layout": "title", "text": "### Scope"}},
		{"heading with level", ExternalBlock{Type: ExternalBlockHeading, Text: "Scope", HeadingLevel: 2}, map[string]any{"doc_type_kwd": "text", "layout": "heading", "text": "## Scope", "heading_level": 2}},
		{"heading without level", ExternalBlock{Type: ExternalBlockHeading, Text: "Scope"}, map[string]any{"doc_type_kwd": "text", "layout": "heading", "text": "Scope"}},
		{"level is ignored on body text", ExternalBlock{Type: ExternalBlockText, Text: "body", HeadingLevel: 2}, map[string]any{"doc_type_kwd": "text", "layout": "text", "text": "body"}},
		{"list", ExternalBlock{Type: ExternalBlockList, Text: "- one"}, map[string]any{"doc_type_kwd": "text", "layout": "list", "text": "- one"}},
		{"code", ExternalBlock{Type: ExternalBlockCode, Text: "x := 1"}, map[string]any{"doc_type_kwd": "text", "layout": "code", "text": "x := 1"}},
		{"equation", ExternalBlock{Type: ExternalBlockEquation, Text: "a+b"}, map[string]any{"doc_type_kwd": "text", "layout": "equation", "text": "a+b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := externalBlockItems(t, ExternalBlockOptions{}, tc.block)
			if len(items) != 1 || !reflect.DeepEqual(items[0], tc.want) {
				t.Fatalf("items = %v, want %v", items, tc.want)
			}
		})
	}
	for _, blockType := range []ExternalBlockType{ExternalBlockText, ExternalBlockTitle, ExternalBlockHeading, ExternalBlockList, ExternalBlockCode, ExternalBlockEquation, ExternalBlockTable} {
		if items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{Type: blockType, Text: "  ", HeadingLevel: 1}); len(items) != 0 {
			t.Errorf("%s block without text emitted %v", blockType, items)
		}
	}
}

func TestExternalBlocksToItemsTable(t *testing.T) {
	const markup = "<table><tr><th>Item</th></tr><tr><td>A</td></tr></table>"
	cells := [][]string{{"k", "v"}, {"a", "1"}}

	t.Run("markup wins over cells and text", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{Type: ExternalBlockTable, HTML: markup, TableCells: cells, Text: "ignored", Page: 4})
		td, _ := items[0]["table"].(*entity.TableData)
		if len(items) != 1 || td == nil || items[0]["doc_type_kwd"] != "table" || items[0]["layout"] != "table" {
			t.Fatalf("items = %v, want one projected table", items)
		}
		if _, hasText := items[0]["text"]; hasText {
			t.Fatalf("a projected table must not keep its markup as text: %v", items[0])
		}
		if rows := td.CellTexts(); len(rows) != 2 || rows[0][0] != "Item" || td.Page != 4 {
			t.Fatalf("table rows = %v page = %d, want the markup rows on page 4", rows, td.Page)
		}
	})
	t.Run("cells are rendered when there is no markup", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{Type: ExternalBlockTable, TableCells: cells, Text: "ignored"})
		td, _ := items[0]["table"].(*entity.TableData)
		if td == nil || !reflect.DeepEqual(td.CellTexts(), cells) || td.HeaderRows != 1 {
			t.Fatalf("items = %v, want the cell rows with one header row", items)
		}
	})
	t.Run("blank cells do not replace the text", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{Type: ExternalBlockTable, TableCells: [][]string{{"", " "}}, Text: "Item | Value"})
		if len(items) != 1 || items[0]["doc_type_kwd"] != "text" || items[0]["layout"] != "table" || items[0]["text"] != "Item | Value" {
			t.Fatalf("items = %v, want the free text of the table region", items)
		}
	})
	t.Run("caption goes to the table and footnotes to its note", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{
			Type: ExternalBlockTable, HTML: markup, Captions: []string{" Table 1 ", ""}, Footnotes: []string{"a first", "b second"},
		})
		td, _ := items[0]["table"].(*entity.TableData)
		if td == nil || td.Caption != "Table 1" || items[0]["text"] != "a first\nb second" {
			t.Fatalf("items = %v caption = %q, want the caption in TableData and the footnotes as note text", items, td.Caption)
		}
	})
	t.Run("a table with no readable cells keeps its caption and footnotes", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{
			Type: ExternalBlockTable, TableCells: [][]string{{"", " "}}, Captions: []string{"Table 1"}, Footnotes: []string{"a note"}, Page: 2,
		})
		want := map[string]any{"doc_type_kwd": "text", "layout": "table", "text": "Table 1\na note", "page_number": 2}
		if len(items) != 1 || !reflect.DeepEqual(items[0], want) {
			t.Fatalf("items = %v, want %v", items, want)
		}
	})
	t.Run("free text keeps its caption and footnotes", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{
			Type: ExternalBlockTable, Text: "Item: 42", Captions: []string{"Table 1"}, Footnotes: []string{"a note"},
		})
		if items[0]["doc_type_kwd"] != "text" || items[0]["text"] != "Item: 42\nTable 1\na note" {
			t.Fatalf("items = %v, want the text with its caption and footnote", items)
		}
	})
}

func TestExternalBlocksToItemsFigure(t *testing.T) {
	region := []ExternalRegion{{Page: 1, Box: [4]float64{10, 20, 110, 40}}}
	t.Run("placeholder", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{FigurePlaceholder: "[Image]"}, ExternalBlock{Type: ExternalBlockFigure})
		if len(items) != 1 || items[0]["doc_type_kwd"] != "image" || items[0]["layout"] != "figure" || items[0]["text"] != "[Image]" {
			t.Fatalf("items = %v, want a figure with the placeholder", items)
		}
	})
	t.Run("captions and footnotes are the text", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{FigurePlaceholder: "[Image]"}, ExternalBlock{
			Type: ExternalBlockFigure, Text: "Chart", Captions: []string{"Figure 1"}, Footnotes: []string{"Source: A"},
		})
		if items[0]["text"] != "Chart\nFigure 1\nSource: A" {
			t.Fatalf("text = %q, want the body, caption and footnote", items[0]["text"])
		}
	})
	t.Run("region without text carries no text", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{Type: ExternalBlockFigure, Page: 1, Regions: region})
		if len(items) != 1 || !HasPDFCropRegion(items[0]) {
			t.Fatalf("items = %v, want the region kept for cropping", items)
		}
		if _, hasText := items[0]["text"]; hasText {
			t.Fatalf("a figure without text must not get a synthetic body: %v", items[0])
		}
	})
	t.Run("neither text nor region", func(t *testing.T) {
		unplaced := []ExternalRegion{{Page: 1, Box: [4]float64{5, 5, 5, 9}}}
		if items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{Type: ExternalBlockFigure, Page: 1, Regions: unplaced}); len(items) != 0 {
			t.Fatalf("items = %v, want the figure dropped", items)
		}
	})
}

func TestExternalBlocksToItemsAttribution(t *testing.T) {
	t.Run("page, order hint and both position keys", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{
			Text: "spans two pages", Page: 2, PreserveOrder: true,
			Regions: []ExternalRegion{{Page: 2, Box: [4]float64{10, 700, 110, 780}}, {Page: 3, Box: [4]float64{10, 20, 110, 60}}},
		})
		want := [][]any{{2, 10.0, 110.0, 700.0, 780.0}, {3, 10.0, 110.0, 20.0, 60.0}}
		if items[0]["page_number"] != 2 || items[0]["_preserve_order"] != true {
			t.Fatalf("item = %v, want page 2 with the order hint", items[0])
		}
		for _, key := range []string{"positions", "_pdf_positions"} {
			if !reflect.DeepEqual(items[0][key], want) {
				t.Fatalf("%s = %v, want %v", key, items[0][key], want)
			}
		}
	})
	t.Run("page identity survives an unusable region", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{
			Text: "body", Page: 7, Regions: []ExternalRegion{{Page: 7, Box: [4]float64{5, 5, 5, 9}}},
		})
		if len(items) != 1 || items[0]["page_number"] != 7 {
			t.Fatalf("items = %v, want the text on page 7", items)
		}
		if _, ok := ExtractPDFPositions(items[0]); ok {
			t.Fatalf("a zero-area region produced positions: %v", items[0])
		}
	})
	t.Run("unknown page sets nothing", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{Text: "body"})
		for _, key := range []string{"page_number", "positions", "_pdf_positions", "_preserve_order"} {
			if _, ok := items[0][key]; ok {
				t.Fatalf("item = %v, want no %s", items[0], key)
			}
		}
	})
	t.Run("pre-split items keep their own shape", func(t *testing.T) {
		td := &entity.TableData{}
		items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{
			// Type and Text describe nothing once the producer supplies items.
			Type: ExternalBlockFigure, Text: "ignored", Page: 5, PreserveOrder: true,
			Regions: []ExternalRegion{{Page: 5, Box: [4]float64{10, 20, 110, 40}}},
			Items: []map[string]any{
				{"text": "# Heading", "doc_type_kwd": "text", "layout": "heading"},
				{"table": td, "doc_type_kwd": "table"},
			},
		})
		if len(items) != 2 || items[0]["layout"] != "heading" || items[0]["text"] != "# Heading" || items[1]["doc_type_kwd"] != "table" {
			t.Fatalf("items = %v, want the supplied items unchanged", items)
		}
		for i, item := range items {
			if item["page_number"] != 5 || item["_preserve_order"] != true {
				t.Fatalf("item %d = %v, want page 5 with the order hint", i, item)
			}
			if _, ok := ExtractPDFPositions(item); !ok {
				t.Fatalf("item %d = %v, want the block's region", i, item)
			}
		}
		if td.Page != 5 {
			t.Fatalf("TableData page = %d, want the block's page", td.Page)
		}
		// Rows are per item: rewriting one item's region must not move the other's.
		items[0]["_pdf_positions"].([][]any)[0][1] = 99.0
		if rows, _ := ExtractPDFPositions(items[1]); rows[0][1] != 10.0 {
			t.Fatalf("items of one block share position rows: %v", rows)
		}
	})
	t.Run("an empty pre-split body emits nothing", func(t *testing.T) {
		if items := externalBlockItems(t, ExternalBlockOptions{}, ExternalBlock{Text: "ignored", Items: []map[string]any{}}); len(items) != 0 {
			t.Fatalf("items = %v, want none", items)
		}
	})
	t.Run("KeepPagePosition retains page-only position for text and table items without regions", func(t *testing.T) {
		items := externalBlockItems(t, ExternalBlockOptions{},
			ExternalBlock{Text: "body", Page: 3, KeepPagePosition: true},
			ExternalBlock{Type: ExternalBlockFigure, Page: 3, KeepPagePosition: true},
			ExternalBlock{Type: ExternalBlockFigure, Text: "caption", Page: 3, KeepPagePosition: true},
			ExternalBlock{
				Page: 3, KeepPagePosition: true,
				Items: []map[string]any{
					{DocTypeKey: DocTypeText, "text": "mixed text"},
					{DocTypeKey: DocTypeTable, "text": "mixed table"},
					{DocTypeKey: DocTypeImage, "text": "mixed image"},
				},
			},
			ExternalBlock{
				Page: 3, KeepPagePosition: true,
				Regions: []ExternalRegion{{Page: 3, Box: [4]float64{10, 20, 30, 40}}},
				Items: []map[string]any{
					{DocTypeKey: DocTypeText, "text": "located text"},
					{DocTypeKey: DocTypeImage, "text": "located image"},
				},
			},
		)
		if len(items) != 7 {
			t.Fatalf("items = %d, want 7 (unlocated empty figure dropped)", len(items))
		}
		wantPageOnly := [][]any{{3, 0.0, 0.0, 0.0, 0.0}}
		for _, idx := range []int{0, 2, 3} {
			if rows, ok := ExtractPDFPositions(items[idx]); !ok || !reflect.DeepEqual(rows, wantPageOnly) {
				t.Fatalf("item %d (%v) positions = %v, want %v", idx, items[idx][DocTypeKey], items[idx]["positions"], wantPageOnly)
			}
		}
		for _, idx := range []int{1, 4} {
			if _, ok := ExtractPDFPositions(items[idx]); ok {
				t.Fatalf("image %d without region should not get a synthetic page position: %v", idx, items[idx]["positions"])
			}
		}
		wantReal := [][]any{{3, 10.0, 30.0, 20.0, 40.0}}
		for _, idx := range []int{5, 6} {
			if rows, ok := ExtractPDFPositions(items[idx]); !ok || !reflect.DeepEqual(rows, wantReal) {
				t.Fatalf("located item %d (%v) positions = %v, want %v", idx, items[idx][DocTypeKey], items[idx]["positions"], wantReal)
			}
		}
	})
}
