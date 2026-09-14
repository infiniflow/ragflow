package chunker

import "testing"

// csvQATableHTML is the markup recordsToHTMLTableChunkList renders for the
// CSV rows "q1,a1" and "question,,extra". The second row has three fields,
// which Python qa.py:365 refuses to read as a pair.
const csvQATableHTML = `<table><caption>Data</caption>
<tr><th>q1</th><th>a1</th></tr>
<tr><td>question</td><td></td><td>extra</td></tr>
</table>`

func csvJSONInputs(name string) map[string]any {
	return map[string]any{
		"name":          name,
		"output_format": "json",
		"json": []map[string]any{
			{"text": csvQATableHTML, "doc_type_kwd": "table", "ck_type": "table"},
		},
	}
}

// A CSV row must hold exactly two fields to become a Q&A pair. The CSV
// parser emits output_format "json" with the rows rendered as one table
// item, so the strict contract has to hold on the JSON item path too.
func TestQAChunker_CSVJSONItemsKeepStrictPairs(t *testing.T) {
	chunks := qaInvoke(t, csvJSONInputs("faq.csv"))
	if len(chunks) != 1 {
		for i, c := range chunks {
			t.Logf("chunk %d: %q", i, c["text"])
		}
		t.Fatalf("want 1 pair from the two-field row, got %d", len(chunks))
	}
	txt, _ := chunks[0]["text"].(string)
	if !contains(txt, "q1") || !contains(txt, "a1") {
		t.Errorf("text = %q, want the q1/a1 pair", txt)
	}
	if contains(txt, "extra") {
		t.Errorf("text = %q, want the three-field row dropped", txt)
	}
}

// The strict contract is CSV only. Python's Excel path takes the first two
// non-empty cells of a row, so the same markup from a workbook still pairs.
func TestQAChunker_XLSXJSONItemsStayLenient(t *testing.T) {
	chunks := qaInvoke(t, csvJSONInputs("faq.xlsx"))
	if len(chunks) != 2 {
		for i, c := range chunks {
			t.Logf("chunk %d: %q", i, c["text"])
		}
		t.Fatalf("want 2 pairs on the workbook path, got %d", len(chunks))
	}
}
