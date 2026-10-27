package chunker

import (
	"strings"
	"testing"
)

func TestTableChunkerKeepsEveryHeaderlessRow(t *testing.T) {
	chunks := tableChunksOf(t, map[string]any{"name": "table.html", "output_format": "json", "json": []map[string]any{
		tableItem(t, "<table><tr><td>Owner</td><td>RET</td></tr><tr><td>Cycle</td><td>90</td></tr></table>"),
	}})
	if len(chunks) != 2 {
		t.Fatalf("chunks=%#v, want both data rows", chunks)
	}
	for i, want := range []string{"<td>Owner</td><td>RET</td>", "<td>Cycle</td><td>90</td>"} {
		text, _ := chunks[i]["text"].(string)
		if !strings.Contains(text, want) || strings.Contains(text, "<th>") {
			t.Fatalf("chunk[%d]=%q", i, text)
		}
	}
}
