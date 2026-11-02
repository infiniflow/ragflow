package chunker

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/ingestion/task/indexdoc"
	"ragflow/internal/parser/parser"
	"ragflow/internal/tokenizer"
)

func TestPaddleOCRHighlightsThroughPipeline(t *testing.T) {
	previous := common.AllowAnyHostForTest
	common.AllowAnyHostForTest = true
	t.Cleanup(func() { common.AllowAnyHostForTest = previous })
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	body, err := os.ReadFile("../../../entity/models/testdata/paddleocr_layout.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../parser/parser/testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()
	driver := models.NewPaddleOCRLocalModel(nil, models.URLSuffix{OCR: "layout-parsing"})
	model := "PaddleOCR-VL-1.6"
	response, err := driver.OCRFile(t.Context(), &model, data, nil, &models.APIConfig{BaseURL: &server.URL}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := parser.PaddleOCRPDFResult(t.Context(), "sample.pdf", data, response)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	for _, item := range result.JSON {
		if strings.Contains(fmt.Sprint(item.ToMap()), "IgnoredFurniture") {
			t.Fatalf("page furniture emitted as a parser item: %v", item)
		}
	}
	parsed := map[string]any{"name": "sample.pdf", "file_type": "pdf", "output_format": "json", "json": result.JSON, "file": result.File}
	general, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512, "table_context_size": 32})
	if err != nil {
		t.Fatal(err)
	}
	chunked, err := general.Invoke(t.Context(), nil, parsed)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := component.NewTokenizerComponent(map[string]any{"search_method": []any{"full_text"}})
	if err != nil {
		t.Fatal(err)
	}
	chunked["name"] = "sample.pdf"
	output, err := tok.Invoke(t.Context(), nil, chunked)
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(output["chunks"])
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	if _, err := indexdoc.ProcessChunksForPipeline(chunks, "doc-id", "sample.pdf", time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	tables := 0
	for _, ck := range chunks {
		if strings.Contains(fmt.Sprint(ck), "IgnoredFurniture") {
			t.Fatalf("page furniture leaked into indexed chunk: %v", ck)
		}
		if _, exists := ck["_preserve_order"]; exists {
			t.Fatalf("parser reading-order flag leaked into indexed chunk: %v", ck)
		}
		positions, ok := ck["position_int"].([][]int)
		if !ok || len(positions) == 0 {
			t.Fatalf("chunk missing indexed positions: %v", ck)
		}
		if !reflect.DeepEqual(ck["page_num_int"], []int{1}) && ck["doc_type_kwd"] == "table" {
			t.Fatalf("table pages=%v", ck["page_num_int"])
		}
		for _, box := range positions {
			if box[0] != 1 || box[2] <= box[1] || box[4] <= box[3] {
				t.Fatalf("bad position=%v", box)
			}
		}
		if ck["doc_type_kwd"] == "table" {
			tables++
		}
	}
	if tables != 2 {
		t.Fatalf("table chunks=%d; want two tables", tables)
	}
}

func TestPaddleOCRChunkersPreserveColumnReadingOrder(t *testing.T) {
	data, err := os.ReadFile("../../../parser/parser/testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Width: 1191, Height: 1684, Blocks: []models.OCRBlock{
		{Text: "LeftFirst", BBox: []float64{10, 10, 400, 60}},
		{Text: "LeftSecond", BBox: []float64{10, 200, 400, 260}},
		{Text: "RightFirst", BBox: []float64{600, 10, 1000, 60}},
		{Text: "RightSecond", BBox: []float64{600, 200, 1000, 260}},
	}}}}
	result := parser.PaddleOCRPDFResult(t.Context(), "columns.pdf", data, response)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	for _, name := range []string{"general", "manual"} {
		t.Run(name, func(t *testing.T) {
			var output map[string]any
			var err error
			input := map[string]any{"name": "columns.pdf", "file_type": "pdf", "output_format": "json", "json": result.JSON, "file": result.File}
			if name == "general" {
				c, e := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
				if e != nil {
					t.Fatal(e)
				}
				output, err = c.Invoke(t.Context(), nil, input)
			} else {
				c, e := NewManualChunker(map[string]any{"levels": [][]string{{`^# `}}})
				if e != nil {
					t.Fatal(e)
				}
				output, err = c.Invoke(t.Context(), nil, input)
			}
			if err != nil {
				t.Fatal(err)
			}
			var texts []string
			for _, chunk := range outputChunks(t, output) {
				texts = append(texts, chunk["text"].(string))
			}
			text := strings.Join(texts, "\n")
			previous := -1
			for _, word := range []string{"LeftFirst", "LeftSecond", "RightFirst", "RightSecond"} {
				index := strings.Index(text, word)
				if index <= previous {
					t.Fatalf("reading order lost: %q", text)
				}
				previous = index
			}
		})
	}
}

func TestPDFSortingPreservesMarkedBarriersWithinMixedInput(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "late before", PDFPositions: mustPosJSON(t, 1, 0, 10, 30, 35)},
		{Text: "early before", PDFPositions: mustPosJSON(t, 1, 0, 10, 10, 15)},
		{Text: "first marked", PreserveOrder: true, PDFPositions: mustPosJSON(t, 1, 0, 10, 90, 95)},
		{Text: "second marked", PreserveOrder: true, PDFPositions: mustPosJSON(t, 1, 0, 10, 5, 10)},
		{Text: "late after", PDFPositions: mustPosJSON(t, 1, 0, 10, 40, 45)},
		{Text: "early after", PDFPositions: mustPosJSON(t, 1, 0, 10, 20, 25)},
	}
	want := []string{"early before", "late before", "first marked", "second marked", "early after", "late after"}
	t.Run("general", func(t *testing.T) {
		got := generalChunkTexts(sortPDFUnits(units))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("mixed reading order=%v; want %v", got, want)
		}
	})
	t.Run("manual", func(t *testing.T) {
		records := recordsFromStructured(units)
		sortRecordsByPosition(records)
		var got []string
		for _, record := range records {
			got = append(got, record.text)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("mixed reading order=%v; want %v", got, want)
		}
	})
}

func TestPaddleOCRCaptionsRemainBodyInTitleChunking(t *testing.T) {
	response := &models.OCRFileResponse{Pages: []models.OCRPage{{Index: 0, Blocks: []models.OCRBlock{
		{Text: "Document title", Label: "doc_title"},
		{Text: "Figure caption", Label: "figure_title"},
		{Text: "Table caption", Label: "table_title"},
		{Text: "Ordinary body.", Label: "text"},
	}}}}
	result := parser.PaddleOCRPDFResult(t.Context(), "captions.pdf", nil, response)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	units, _, err := schema.ChunkDocsFromDocItem(result.JSON)
	if err != nil {
		t.Fatal(err)
	}
	records := recordsFromStructured(units)
	params := defaultsTitle()
	levels := resolveTitleLevels(records, &params)
	if len(levels) != 4 || levels[0] >= bodyLevel || levels[1] != bodyLevel || levels[2] != bodyLevel || levels[3] != bodyLevel {
		t.Fatalf("caption became a section heading: levels=%v records=%+v", levels, records)
	}
}
