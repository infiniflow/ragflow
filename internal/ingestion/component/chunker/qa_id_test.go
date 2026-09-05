package chunker

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/ingestion/task/indexdoc"
)

// xlsxQATableHTML is the HTML the spreadsheet parser emits for a
// headerless two-column Q&A workbook (renderSheetTables repeats the
// first row as the <th> header; extractQATable still reads it as a pair).
const xlsxQATableHTML = `<table><caption>Sheet1</caption>
<tr><th>门打不开怎么办</th><th>请家人回来开门</th></tr>
<tr><td>门的材质是什么</td><td>蜂窝纸板</td></tr>
<tr><td>怎么确定门的真伪</td><td>提供钢印号</td></tr>
</table>`

// TestQAChunker_HTMLTable_TextCarrierAndTopInt pins the spreadsheet Q&A
// contract the index write depends on: every row becomes its own chunk
// carried by `text` (not content_with_weight, which ingestion renames
// from text — a content_with_weight-only chunk would hash an empty text
// into a shared id and the index write would collapse all rows into one
// chunk), and each chunk carries the pair index in top_int like Python
// qa.py's beAdoc(..., row_num=ii).
func TestQAChunker_HTMLTable_TextCarrierAndTopInt(t *testing.T) {
	inputs := map[string]any{
		"name":          "售后常见问题.xlsx",
		"output_format": "html",
		"html":          xlsxQATableHTML,
	}
	chunks := qaInvoke(t, inputs)
	if len(chunks) != 3 {
		t.Fatalf("want one chunk per Q&A row, got %d", len(chunks))
	}
	for i, c := range chunks {
		if _, exists := c["content_with_weight"]; exists {
			t.Errorf("chunk %d must carry text, not content_with_weight (ingestion renames): %#v", i, c)
		}
		txt, _ := c["text"].(string)
		if txt == "" || !contains(txt, "问题：") || !contains(txt, "回答：") {
			t.Errorf("chunk %d text = %q, want 问题/回答 pair", i, txt)
		}
		raw, ok := c["top_int"].([]any)
		if !ok || len(raw) != 1 || int(raw[0].(float64)) != i {
			t.Errorf("chunk %d top_int = %#v, want [%d]", i, c["top_int"], i)
		}
	}
}

// TestQAChunker_ChunksGetDistinctIndexIDs runs the QA chunker output
// through the ingestion post-processing that assigns index ids, guarding
// the end-to-end invariant behind the reported bug: an N-row Q&A
// workbook must yield N distinct chunk ids (countDistinctChunkIDs == N),
// not one shared empty-text id with only the last row surviving.
func TestQAChunker_ChunksGetDistinctIndexIDs(t *testing.T) {
	inputs := map[string]any{
		"name":          "售后常见问题.xlsx",
		"output_format": "html",
		"html":          xlsxQATableHTML,
	}
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := indexdoc.NormalizeChunks(out)
	if _, err := indexdoc.ProcessChunksForPipeline(chunks, "doc-1", "售后常见问题.xlsx", time.Now()); err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	ids := map[string]struct{}{}
	for i, ck := range chunks {
		id, _ := ck["id"].(string)
		if id == "" {
			t.Fatalf("chunk %d has no id", i)
		}
		ids[id] = struct{}{}
		if _, hasText := ck["text"]; hasText {
			t.Errorf("chunk %d: text must be renamed to content_with_weight by ingestion", i)
		}
		if cw, _ := ck["content_with_weight"].(string); cw == "" {
			t.Errorf("chunk %d: content_with_weight empty after rename", i)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("want 3 distinct chunk ids, got %d", len(ids))
	}
}
