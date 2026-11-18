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
	if got := int(numberValue(res.File["page_count"])); got != 2 {
		t.Fatalf("page_count = %v, want 2 from the highest retained page", res.File["page_count"])
	}
}

func TestMinerUMarkdownResultCountsSkippedContentListPages(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = list.Write([]byte(`[
		{"type":"text","text":"Only body","page_idx":0},
		{"type":"header","text":"Running head","page_idx":4},
		{"type":"page_number","text":"5","page_idx":4}
	]`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	res := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{Zip: buf.Bytes()})
	if res.Err != nil {
		t.Fatalf("content list result: %v", res.Err)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 1 || items[0]["text"] != "Only body" || items[0]["page_number"] != 1 {
		t.Fatalf("items = %#v", items)
	}
	if got := int(numberValue(res.File["page_count"])); got != 5 {
		t.Fatalf("page_count = %v, want 5 from the skipped header on page_idx 4", res.File["page_count"])
	}
}

func TestMinerUMarkdownResultUsesDocVortexPageSize(t *testing.T) {
	for _, middleName := range []string{"middle.json", "result/middle_json.json"} {
		t.Run(middleName, func(t *testing.T) {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			structured, err := zw.Create("structured_content.json")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = structured.Write([]byte(`{"pages":[{"page_idx":0,"blocks":[
		{"type":"text","content":"Alpha body","bbox":[0.1,0.2,0.4,0.3]},
		{"type":"image","bbox":[0.2,0.5,0.6,0.8]}
	]}]}`))
			middle, err := zw.Create(middleName)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = middle.Write([]byte(`{"extensions":{"docvortex_layout":{"version":1,"pages":[
		{"page_idx":0,"width_pt":1000,"height_pt":1000}
	]}}}`))
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}

			res := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{
				Markdown: "Alpha body",
				Zip:      buf.Bytes(),
			})
			if res.Err != nil {
				t.Fatalf("docvortex result: %v", res.Err)
			}
			items := ItemsToMaps(res.JSON)
			if len(items) != 2 {
				t.Fatalf("items = %d, want paragraph and uncaptioned image: %#v", len(items), items)
			}
			if items[0]["text"] != "Alpha body" || items[0]["doc_type_kwd"] != "text" {
				t.Fatalf("paragraph = %#v", items[0])
			}
			if items[1]["doc_type_kwd"] != "image" {
				t.Fatalf("image = %#v", items[1])
			}
			positions, _ := items[1]["_pdf_positions"].([][]any)
			if len(positions) != 1 || int(numberValue(positions[0][1])) != 200 || int(numberValue(positions[0][3])) != 500 {
				t.Fatalf("image positions = %#v, want the 0-1 box scaled by width_pt/height_pt", items[1]["_pdf_positions"])
			}
		})
	}
}

func TestMinerUMarkdownResultKeepsCodeNotesAndTextLevel(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	list, err := zw.Create("content_list.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = list.Write([]byte(`[
		{"type":"text","text":"Introduction","text_level":1,"page_idx":0},
		{"type":"text","text":"Body","text_level":0,"page_idx":0},
		{"type":"code","code_body":"print(1)","code_caption":["Listing"],"code_footnote":["Runs locally"],"page_idx":0}
	]`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	res := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{Zip: buf.Bytes()})
	if res.Err != nil {
		t.Fatalf("content list result: %v", res.Err)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 3 {
		t.Fatalf("items = %#v", items)
	}
	if items[0]["layout"] != "title" || items[0]["heading_level"] != 1 || items[0]["text"] != "Introduction" {
		t.Fatalf("heading = %#v", items[0])
	}
	if items[1]["layout"] != "text" || items[1]["heading_level"] != nil || items[1]["text"] != "Body" {
		t.Fatalf("body = %#v", items[1])
	}
	code := stringValue(items[2]["text"])
	if items[2]["layout"] != "code" || !strings.Contains(code, "print(1)") || !strings.Contains(code, "Listing") || !strings.Contains(code, "Runs locally") {
		t.Fatalf("code = %#v", items[2])
	}

	var structuredZip bytes.Buffer
	zw = zip.NewWriter(&structuredZip)
	structured, err := zw.Create("structured_content.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = structured.Write([]byte(`{"pages":[{"page_idx":0,"blocks":[
		{"type":"code","content":"print(1)","captions":[{"content":"Listing"}],"footnotes":[{"content":"Runs locally"}]}
	]}]}`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	structuredRes := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{Zip: structuredZip.Bytes()})
	if structuredRes.Err != nil {
		t.Fatalf("structured code: %v", structuredRes.Err)
	}
	structuredItems := ItemsToMaps(structuredRes.JSON)
	if len(structuredItems) != 1 {
		t.Fatalf("structured items = %#v", structuredItems)
	}
	structuredCode := stringValue(structuredItems[0]["text"])
	if !strings.Contains(structuredCode, "print(1)") || !strings.Contains(structuredCode, "Listing") || !strings.Contains(structuredCode, "Runs locally") {
		t.Fatalf("structured code = %#v", structuredItems[0])
	}
}

func TestMinerUMarkdownResultDropsTextlessEquations(t *testing.T) {
	for _, tc := range []struct{ name, member, content string }{
		{"structured", "structured_content.json", `{"pages":[{"page_idx":0,"blocks":[
            {"type":"text","content":"Alpha body"},
            {"type":"equation","content":"","bbox":[0.1,0.2,0.5,0.4]},
            {"type":"equation","content":""}
        ]}]}`},
		{"content list", "content_list.json", `[
            {"type":"text","text":"Alpha body","page_idx":0},
            {"type":"equation","text":"","page_idx":0,"bbox":[100,200,500,400]},
            {"type":"equation","text":"","page_idx":0}
        ]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := minerUZipResult(t, tc.member, tc.content, "Alpha body")
			items := ItemsToMaps(result.JSON)
			if len(items) != 1 || items[0]["text"] != "Alpha body" || items[0]["layout"] == "equation" {
				t.Fatalf("items = %#v, want the paragraph only", items)
			}
		})
	}

	t.Run("equation text is kept", func(t *testing.T) {
		result := minerUZipResult(t, "content_list.json", `[
			{"type":"equation","text":"E = mc^2","page_idx":0,"bbox":[100,200,500,400]}
		]`, "")
		items := ItemsToMaps(result.JSON)
		if len(items) != 1 || items[0]["layout"] != "equation" || items[0]["text"] != "E = mc^2" {
			t.Fatalf("equation = %#v", items)
		}
	})

	t.Run("equation only falls back to markdown", func(t *testing.T) {
		result := minerUZipResult(t, "content_list.json", `[
			{"type":"equation","text":"","page_idx":0,"bbox":[100,200,500,400]}
		]`, "E = mc^2")
		items := ItemsToMaps(result.JSON)
		if len(items) == 0 || !strings.Contains(stringValue(items[0]["text"]), "E = mc^2") || items[0]["layout"] == "equation" {
			t.Fatalf("items = %#v, want the markdown formula", items)
		}
	})
}

func minerUZipResult(t *testing.T, memberName, content, markdown string) ParseResult {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	member, err := zw.Create(memberName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	middle, err := zw.Create("middle.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := middle.Write([]byte(`{"pdf_info":[{"page_idx":0,"page_size":[1000,1000]}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	result := minerUMarkdownResult(t.Context(), "sample.pdf", &models.MinerUV1Result{Markdown: markdown, Zip: buf.Bytes()})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	return result
}
