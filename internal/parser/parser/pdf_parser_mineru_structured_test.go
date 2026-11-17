package parser

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"ragflow/internal/entity"
	models "ragflow/internal/entity/models"
)

func TestMinerUMarkdownResultKeepsStructuredPagesAndCaptions(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	structured, err := zw.Create("structured_content.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = structured.Write([]byte(`{"pages":[
		{"page_idx":0,"blocks":[
			{"type":"text","content":"Alpha body","bbox":[0.1,0.2,0.5,0.4]},
			{"type":"header","content":"RUNNING HEADER"}
		]},
		{"page_idx":1,"blocks":[
			{"type":"table","content":"<table><tr><td>42</td></tr></table>","captions":[{"content":"Rates"}]}
		]}
	]}`))
	middle, err := zw.Create("middle.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = middle.Write([]byte(`{"pdf_info":[
		{"page_idx":0,"page_size":[1000,1000]},
		{"page_idx":1,"page_size":[1000,1000]}
	]}`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	res := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{
		Markdown: "Alpha body\n<table><tr><td>42</td></tr></table>\nRates\nRUNNING HEADER",
		Zip:      buf.Bytes(),
	})
	if res.Err != nil {
		t.Fatalf("structured result: %v", res.Err)
	}
	if got := int(numberValue(res.File["page_count"])); got != 2 {
		t.Fatalf("page_count = %v, want 2", res.File["page_count"])
	}

	items := ItemsToMaps(res.JSON)
	if len(items) != 2 {
		t.Fatalf("items = %d, want paragraph and table (header skipped, caption kept on the table)", len(items))
	}
	if items[0]["page_number"] != 1 || items[0]["text"] != "Alpha body" || items[0]["doc_type_kwd"] != "text" {
		t.Fatalf("paragraph = %#v", items[0])
	}
	positions, _ := items[0]["_pdf_positions"].([][]any)
	if len(positions) != 1 || int(numberValue(positions[0][0])) != 1 {
		t.Fatalf("paragraph positions = %#v", items[0]["_pdf_positions"])
	}
	if int(numberValue(positions[0][1])) != 100 || int(numberValue(positions[0][3])) != 200 {
		t.Fatalf("paragraph box = %#v, want left 100 top 200 from the 0-1 box on a 1000pt page", positions[0])
	}

	if items[1]["page_number"] != 2 || items[1]["doc_type_kwd"] != "table" {
		t.Fatalf("table = %#v", items[1])
	}
	table, _ := items[1]["table"].(*entity.TableData)
	if table == nil || table.Page != 2 {
		t.Fatalf("table data = %#v", items[1]["table"])
	}
	joined := strings.TrimSpace(strings.Join([]string{table.Caption, stringValue(items[1]["text"])}, "\n"))
	if !strings.Contains(joined, "Rates") {
		t.Fatalf("table caption = %q, text = %#v", table.Caption, items[1]["text"])
	}
	for _, item := range items {
		if text := stringValue(item["text"]); strings.Contains(text, "RUNNING HEADER") {
			t.Fatalf("header leaked into %#v", item)
		}
	}
}

func TestMinerUMarkdownResultKeepsContentListPageIdentity(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = list.Write([]byte(`[
		{"type":"text","text":"Second page","page_idx":1},
		{"type":"header","text":"RUNNING HEADER","page_idx":1}
	]`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	res := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{
		Markdown: "Second page\nRUNNING HEADER",
		Zip:      buf.Bytes(),
	})
	if res.Err != nil {
		t.Fatalf("content list result: %v", res.Err)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 1 || items[0]["text"] != "Second page" || items[0]["page_number"] != 2 {
		t.Fatalf("items = %#v", items)
	}
}
