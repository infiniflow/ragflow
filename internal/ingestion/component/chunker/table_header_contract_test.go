package chunker

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/parser/tableutil"
)

func TestPredictedJSONTableRowsThroughChunkers(t *testing.T) {
	for _, markup := range []string{
		`<table><caption>People</caption><tr><th>Alice</th><th>18</th></tr><tr><td>Bob</td><td>20</td></tr></table>`,
		`<table><caption>People</caption><tr><th>Name</th><th>Age</th></tr><tr><td>Alice</td><td>18</td></tr><tr><td>Bob</td><td>20</td></tr></table>`,
	} {
		items := []map[string]any{{"doc_type_kwd": "table", "text": markup, "page_number": 1, "_pdf_positions": [][]float64{{1, 10, 110, 20, 40}}}}
		parser.ProjectPredictedTableItems(items)
		wire, err := json.Marshal(map[string]any{"name": "data.pdf", "output_format": "json", "json": items})
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err = json.Unmarshal(wire, &payload); err != nil {
			t.Fatal(err)
		}
		table, _ := NewTableChunker(nil)
		qa, _ := NewQAChunker(nil)
		for _, consumer := range []struct {
			name string
			run  func() (map[string]any, error)
		}{
			{"table", func() (map[string]any, error) { return table.Invoke(t.Context(), nil, payload) }},
			{"qa", func() (map[string]any, error) { return qa.Invoke(t.Context(), nil, payload) }},
		} {
			out, err := consumer.run()
			if err != nil {
				t.Fatal(err)
			}
			chunks := outputChunks(t, out)
			if len(chunks) != 2 {
				t.Fatalf("%s got %d chunks, want 2 data rows", consumer.name, len(chunks))
			}
			for i, name := range []string{"Alice", "Bob"} {
				text := chunks[i]["text"].(string)
				if !strings.Contains(text, name) || strings.Contains(text, []string{"Bob", "Alice"}[i]) {
					t.Errorf("%s row %d = %q", consumer.name, i, text)
				}
				if consumer.name == "table" && !strings.Contains(text, "<caption>People</caption>") {
					t.Errorf("caption lost in row chunk: %s", text)
				}
				if _, ok := chunks[i]["_pdf_positions"]; !ok {
					t.Errorf("%s PDF positions lost", consumer.name)
				}
			}
		}
	}
}

func TestCSVHeaderlessRowsThroughTableChunker(t *testing.T) {
	parsed := parser.NewCSVParser().ParseWithResult(t.Context(), "data.csv", []byte("Alice,18\nBob,20\n"))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	c, _ := NewTableChunker(nil)
	out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "data.csv", "file_type": "csv", "output_format": "json", "json": parsed.JSON})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want both data rows", len(chunks))
	}
	for i, name := range []string{"Alice", "Bob"} {
		text := chunks[i]["text"].(string)
		if strings.Contains(text, "<th") || !strings.Contains(text, name) {
			t.Errorf("row %d = %s", i, text)
		}
		other := []string{"Bob", "Alice"}[i]
		if strings.Contains(text, other) {
			t.Errorf("data row repeated as header: %s", text)
		}
	}
}

func TestQAUsesConfirmedHeaderRows(t *testing.T) {
	for _, headers := range []int{0, 1} {
		td := &entity.TableData{HeaderRows: headers, Grid: [][]entity.TableCell{
			{{Text: "Question"}, {Text: "Answer"}}, {{Text: "Why?"}, {Text: "Because."}},
		}}
		pairs := extractQAJSON([]schema.ChunkDoc{{TableData: td}}, "xlsx")
		if len(pairs) != 2-headers || pairs[len(pairs)-1].Question != "Why?" {
			t.Fatalf("header rows %d: %+v", headers, pairs)
		}
	}
}

func TestGeneralHeaderlessSubtablesKeepEveryRow(t *testing.T) {
	td := &entity.TableData{Grid: [][]entity.TableCell{
		{{Text: "Alice"}, {Text: "18"}}, {{Text: "Bob"}, {Text: "20"}}, {{Text: "Carol"}, {Text: "22"}},
	}}
	count := func(s string) int { return len(s) }
	parts, _, _ := splitLargeTable(td, 70, count, "")
	if len(parts) < 2 {
		t.Fatalf("expected split: %v", parts)
	}
	var rows int
	for _, part := range parts {
		rows += len(part.Grid)
		if part.HeaderRows != 0 || strings.Contains(tableutil.RenderTableHTML(&part), "<th") {
			t.Fatalf("invented header: %+v", part)
		}
	}
	if rows != 3 {
		t.Fatalf("got %d rows, want 3", rows)
	}
}

func TestGeneralTableHeaderRowspanSurvivesWire(t *testing.T) {
	td, err := tableutil.ParseTableHTML(`<table><tr><th>Name</th><th rowspan="2">Group</th></tr><tr><td>Alice</td></tr><tr><td>Bob</td><td>B</td></tr><tr><td>Carol</td><td>C</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewGeneralChunker(map[string]any{"chunk_token_size": 1})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "merged.xlsx", "file_type": "xlsx", "output_format": "json", "json": []map[string]any{{"doc_type_kwd": "table", "table": td}}})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 data rows", len(chunks))
	}
	if text := chunks[0]["text"].(string); !strings.Contains(text, `rowspan="2"`) {
		t.Fatalf("header/data merge lost across parser wire: %s", text)
	}
}
