package chunker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/ingestion/task/indexdoc"
	"ragflow/internal/parser/parser"
)

func TestMistralOCRPageNumbersThroughPipeline(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	if models.GetProviderManager() == nil {
		if err := models.InitProviderManager("../../../../conf/models"); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile("../../../parser/parser/testdata/paddleocr_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/ocr" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want selected instance key", got)
		}
		var body struct {
			Model string `json:"model"`
			Pages []int  `json:"pages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode OCR request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Model != "mistral-ocr-latest" || !reflect.DeepEqual(body.Pages, []int{0, 2}) {
			t.Errorf("request model=%q pages=%v, want mistral-ocr-latest and [0 2]", body.Model, body.Pages)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"pages":[
			{"index":0,"blocks":[
				{"type":"header","content":"Running header"},
				{"type":"title","content":"Introduction"},
				{"type":"text","content":"First page text."}
			]},
			{"index":2,"markdown":"Third page text."}
		]}`)
	}))
	defer server.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.UserTenant{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.UserTenant{ID: "owner", TenantID: "tenant", UserID: "tenant", Role: "owner"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"Mistral", "Mistral OCR"} {
		key := "test-key"
		if provider == "Mistral OCR" {
			key = `{"MISTRAL_OCR_API_KEY":"test-key"}`
		}
		for _, row := range []any{
			&entity.TenantModelProvider{ID: provider, TenantID: "tenant", UserID: "tenant", ProviderName: provider},
			&entity.TenantModelInstance{
				ID: "instance-" + provider, ProviderID: provider, InstanceName: "default",
				Status: "active", APIKey: key,
				Extra: fmt.Sprintf(`{"base_url":%q}`, server.URL+"/v1"),
			},
			&entity.TenantModel{
				ID: "model-" + provider, ProviderID: provider, InstanceID: "instance-" + provider,
				ModelName: "mistral-ocr-latest", ModelType: int(entity.ModelTypeOCR), Status: "active",
			},
		} {
			if err := db.Create(row).Error; err != nil {
				t.Fatalf("seed %T: %v", row, err)
			}
		}
	}

	for _, tc := range []struct {
		name, field, selector string
	}{
		{"tenant model ID", "layout_recognizer", "model-Mistral"},
		{"Mistral composite", "parse_method", "mistral-ocr-latest@default@Mistral"},
		{"stored Mistral OCR composite", "layout_recognizer", "mistral-ocr-latest@default@Mistral OCR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := component.NewParserComponent(map[string]any{
				"pdf": map[string]any{tc.field: tc.selector, "pages": []any{[]any{1, 1}, []any{3, 3}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			const filename = "sample.pdf"
			parsed, err := p.Invoke(t.Context(), db, map[string]any{
				"binary": data, "name": filename, "tenant_id": "tenant",
			})
			if err != nil {
				t.Fatal(err)
			}
			if meta := parsed["file"].(map[string]any); meta["page_count"] != 3 {
				t.Fatalf("unexpected file metadata: %v", meta)
			}
			items := parser.ItemsToMaps(parsed["json"].([]parser.DocItem))
			if len(items) != 3 || items[0]["layout"] != "heading" ||
				items[1]["text"] != "First page text." || items[2]["text"] != "Third page text." {
				t.Fatalf("unexpected parser items: %v", items)
			}
			for i, page := range []int{1, 1, 3} {
				want := [][]any{{page, 0.0, 0.0, 0.0, 0.0}}
				if got := parser.PDFPositionRows(items[i]["positions"]); !reflect.DeepEqual(got, want) {
					t.Errorf("item %d positions = %v, want %v", i, got, want)
				}
			}

			chunker, err := NewGeneralChunker(nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := chunker.Invoke(t.Context(), db, parsed)
			if err != nil {
				t.Fatal(err)
			}
			chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
			if !ok || len(chunks) == 0 {
				t.Fatalf("expected chunks, got %v", out["chunks"])
			}
			if _, err := indexdoc.ProcessChunksForPipeline(chunks, "doc", filename, time.Unix(1, 0)); err != nil {
				t.Fatal(err)
			}
			pages := map[int]bool{}
			for _, chunk := range chunks {
				if strings.Contains(fmt.Sprint(chunk), "Running header") {
					t.Errorf("header reached the index: %v", chunk)
				}
				numbers, ok := chunk["page_num_int"].([]int)
				if !ok || len(numbers) == 0 {
					t.Fatalf("chunk is missing indexed page numbers: %v", chunk)
				}
				for _, page := range numbers {
					pages[page] = true
				}
			}
			if !reflect.DeepEqual(pages, map[int]bool{1: true, 3: true}) {
				t.Errorf("indexed pages = %v, want pages 1 and 3", pages)
			}
		})
	}
}
