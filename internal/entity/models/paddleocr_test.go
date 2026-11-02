package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestPaddleOCRUploadFilename checks content-derived image extensions and the
// unchanged PDF default, including both TIFF byte orders.
func TestPaddleOCRUploadFilename(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"pdf", []byte("%PDF-1.7"), "document.pdf"},
		{"png", []byte("\x89PNG\r\n\x1a\n"), "document.png"},
		{"jpeg", []byte("\xff\xd8\xff"), "document.jpg"},
		{"gif", []byte("GIF89a"), "document.gif"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "document.webp"},
		{"bmp", []byte("BM\x00\x00"), "document.bmp"},
		{"little endian tiff", []byte("II*\x00"), "document.tiff"},
		{"big endian tiff", []byte("MM\x00*"), "document.tiff"},
		{"unrecognized", []byte("unknown"), "document.pdf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := paddleOCRUploadFilename(tc.data); got != tc.want {
				t.Fatalf("filename=%q, want %q", got, tc.want)
			}
		})
	}
}

// TestPaddleOCRLocalListModels verifies that the local PaddleOCR driver lists
// the default models shipped in conf/models/paddleocr_local.json, so that the
// "add model by selection" flow can pick them up.
func TestPaddleOCRLocalListModels(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "paddleocr_local.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	driver := NewPaddleOCRLocalModel(map[string]string{"default": "http://127.0.0.1:9000"}, URLSuffix{OCR: "layout-parsing"})
	models, err := driver.ListModels(context.Background(), &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	want := []string{"PaddleOCR-VL-1.6", "PaddleOCR-VL-1.5", "PP-OCRv6", "PP-OCRv5", "PP-StructureV3"}
	if len(models) != len(want) {
		t.Fatalf("len(models) = %d, want %d", len(models), len(want))
	}
	for i, name := range want {
		if models[i].Name != name {
			t.Errorf("models[%d].Name = %q, want %q", i, models[i].Name, name)
		}
		if len(models[i].ModelTypes) != 1 || models[i].ModelTypes[0] != "ocr" {
			t.Errorf("models[%d].ModelTypes = %v, want [ocr]", i, models[i].ModelTypes)
		}
	}
}

// TestPaddleOCRConfigFromAPIKey pins the api_key JSON contract shared with
// Python's PaddleOCROcrModel: the cloud provider stores
// paddleocr_base_url / paddleocr_api_url, paddleocr_access_token and
// paddleocr_algorithm in the api_key payload — either with the lower-case UI
// keys or the upper-case env-style keys the env auto-provisioning writes —
// while a plain-text api_key (PaddleOCR.local bearer token) yields zero
// values.
func TestPaddleOCRConfigFromAPIKey(t *testing.T) {
	cases := []struct {
		name      string
		apiKey    string
		wantURL   string
		wantToken string
		wantAlgo  string
	}{
		{
			name:      "empty",
			apiKey:    "",
			wantURL:   "",
			wantToken: "",
			wantAlgo:  "",
		},
		{
			name:      "plain token (PaddleOCR.local)",
			apiKey:    "tok-123",
			wantURL:   "",
			wantToken: "",
			wantAlgo:  "",
		},
		{
			name:      "api_url payload",
			apiKey:    `{"paddleocr_api_url":"http://ocr.test/api","paddleocr_access_token":"tok-456","paddleocr_algorithm":"PaddleOCR-VL"}`,
			wantURL:   "http://ocr.test/api",
			wantToken: "tok-456",
			wantAlgo:  "PaddleOCR-VL",
		},
		{
			name:      "base_url wins over api_url",
			apiKey:    `{"paddleocr_base_url":"http://base.test","paddleocr_api_url":"http://api.test","paddleocr_access_token":"tok-789"}`,
			wantURL:   "http://base.test",
			wantToken: "tok-789",
			wantAlgo:  "",
		},
		{
			name:      "nested api_key key",
			apiKey:    `{"api_key":{"paddleocr_api_url":"http://nested.test","paddleocr_access_token":"tok-nested"}}`,
			wantURL:   "http://nested.test",
			wantToken: "tok-nested",
			wantAlgo:  "",
		},
		{
			name:      "unrelated json ignored",
			apiKey:    `{"openai_api_key":"sk-xxx"}`,
			wantURL:   "",
			wantToken: "",
			wantAlgo:  "",
		},
		{
			name:      "upper-case env-style payload",
			apiKey:    `{"PADDLEOCR_BASE_URL":"http://envbase.test","PADDLEOCR_ACCESS_TOKEN":"tok-env","PADDLEOCR_ALGORITHM":"PP-OCRv5"}`,
			wantURL:   "http://envbase.test",
			wantToken: "tok-env",
			wantAlgo:  "PP-OCRv5",
		},
		{
			name:      "upper-case api_url fallback",
			apiKey:    `{"PADDLEOCR_API_URL":"http://envapi.test","PADDLEOCR_ACCESS_TOKEN":"tok-env2"}`,
			wantURL:   "http://envapi.test",
			wantToken: "tok-env2",
			wantAlgo:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, token, algo := paddleOCRConfigFromAPIKey(tc.apiKey)
			if url != tc.wantURL {
				t.Errorf("baseURL = %q, want %q", url, tc.wantURL)
			}
			if token != tc.wantToken {
				t.Errorf("accessToken = %q, want %q", token, tc.wantToken)
			}
			if algo != tc.wantAlgo {
				t.Errorf("algorithm = %q, want %q", algo, tc.wantAlgo)
			}
		})
	}
}

// sampleArrayResult mirrors the actual response shape observed from the
// PaddleOCR online service: a JSON array of page objects where each
// element carries result.layoutParsingResults[].markdown.text plus
// logId/errorCode/errorMsg at the top level.
const sampleArrayResult = `[
  {
    "logId": "de52e6c7-225e-4b0e-9a6b-8b4f5c74a3c3",
    "result": {
      "layoutParsingResults": [
        {
          "prunedResult": {"page_count": null, "width": 998, "height": 1418},
          "markdown": {"text": "## 第一章\n\n道可道，非常道。", "images": {}},
          "outputImages": {"layout_det_res": "https://example.com/layout_0.jpg"},
          "inputImage": "https://example.com/input_0.jpg"
        }
      ],
      "dataInfo": {"numPages": 1, "type": "pdf"},
      "preprocessedImages": ["https://example.com/preprocessed_0.jpg"]
    },
    "errorCode": 0,
    "errorMsg": "Success"
  },
  {
    "logId": "de52e6c7-225e-4b0e-9a6b-8b4f5c74a3c3",
    "result": {
      "layoutParsingResults": [
        {
          "prunedResult": {"page_count": null, "width": 998, "height": 1418},
          "markdown": {"text": "第二章 天下皆知美之为美", "images": {}},
          "outputImages": {},
          "inputImage": "https://example.com/input_1.jpg"
        }
      ],
      "dataInfo": {"numPages": 1, "type": "pdf"}
    },
    "errorCode": 0,
    "errorMsg": "Success"
  }
]`

// TestPaddleOCRResolvedAPIConfig pins the wire-config resolution order shared
// by both PaddleOCR drivers: bearer token from the api_key payload, base url
// from the instance field, then the payload, then the PADDLEOCR_* env vars.
func TestPaddleOCRResolvedAPIConfig(t *testing.T) {
	t.Run("json payload supplies token and base url", func(t *testing.T) {
		apiKey := `{"paddleocr_api_url":"http://key.test/api","paddleocr_access_token":"tok-456"}`
		emptyBaseURL := ""
		resolved := paddleOCRResolvedAPIConfig(&APIConfig{ApiKey: &apiKey, BaseURL: &emptyBaseURL})
		if got := *resolved.ApiKey; got != "tok-456" {
			t.Errorf("ApiKey = %q, want tok-456", got)
		}
		if got := *resolved.BaseURL; got != "http://key.test/api" {
			t.Errorf("BaseURL = %q, want http://key.test/api", got)
		}
	})

	t.Run("instance base url wins over payload", func(t *testing.T) {
		apiKey := `{"paddleocr_base_url":"http://key.test","paddleocr_access_token":"tok-1"}`
		baseURL := "http://instance.test"
		resolved := paddleOCRResolvedAPIConfig(&APIConfig{ApiKey: &apiKey, BaseURL: &baseURL})
		if got := *resolved.BaseURL; got != "http://instance.test" {
			t.Errorf("BaseURL = %q, want http://instance.test", got)
		}
		if got := *resolved.ApiKey; got != "tok-1" {
			t.Errorf("ApiKey = %q, want tok-1", got)
		}
	})

	t.Run("plain token passes through", func(t *testing.T) {
		apiKey := "tok-plain"
		baseURL := "http://instance.test"
		resolved := paddleOCRResolvedAPIConfig(&APIConfig{ApiKey: &apiKey, BaseURL: &baseURL})
		if got := *resolved.ApiKey; got != "tok-plain" {
			t.Errorf("ApiKey = %q, want tok-plain", got)
		}
		if got := *resolved.BaseURL; got != "http://instance.test" {
			t.Errorf("BaseURL = %q, want http://instance.test", got)
		}
	})

	t.Run("env base url fallback", func(t *testing.T) {
		t.Setenv("PADDLEOCR_BASE_URL", "http://envbase.test")
		apiKey := `{"paddleocr_access_token":"tok-env"}`
		emptyBaseURL := ""
		resolved := paddleOCRResolvedAPIConfig(&APIConfig{ApiKey: &apiKey, BaseURL: &emptyBaseURL})
		if got := *resolved.BaseURL; got != "http://envbase.test" {
			t.Errorf("BaseURL = %q, want http://envbase.test", got)
		}
	})

	t.Run("env auto-provisioned payload resolves", func(t *testing.T) {
		apiKey := `{"PADDLEOCR_BASE_URL":"http://envauto.test","PADDLEOCR_ACCESS_TOKEN":"tok-auto","PADDLEOCR_ALGORITHM":"PaddleOCR-VL"}`
		emptyBaseURL := ""
		resolved := paddleOCRResolvedAPIConfig(&APIConfig{ApiKey: &apiKey, BaseURL: &emptyBaseURL})
		if got := *resolved.ApiKey; got != "tok-auto" {
			t.Errorf("ApiKey = %q, want tok-auto", got)
		}
		if got := *resolved.BaseURL; got != "http://envauto.test" {
			t.Errorf("BaseURL = %q, want http://envauto.test", got)
		}
	})

	t.Run("whitespace instance base url falls back to payload", func(t *testing.T) {
		apiKey := `{"paddleocr_base_url":"http://key.test","paddleocr_access_token":"tok-2"}`
		baseURL := "   "
		resolved := paddleOCRResolvedAPIConfig(&APIConfig{ApiKey: &apiKey, BaseURL: &baseURL})
		if got := *resolved.BaseURL; got != "http://key.test" {
			t.Errorf("BaseURL = %q, want http://key.test", got)
		}
		if got := *resolved.ApiKey; got != "tok-2" {
			t.Errorf("ApiKey = %q, want tok-2", got)
		}
	})

	t.Run("padded instance base url is trimmed", func(t *testing.T) {
		apiKey := `{"paddleocr_access_token":"tok-3"}`
		baseURL := "  http://instance.test  "
		resolved := paddleOCRResolvedAPIConfig(&APIConfig{ApiKey: &apiKey, BaseURL: &baseURL})
		if got := *resolved.BaseURL; got != "http://instance.test" {
			t.Errorf("BaseURL = %q, want http://instance.test", got)
		}
	})
}

// TestPaddleOCRModelOCRFileUnwrapsAPIKeyPayload pins the layering contract the
// cloud driver owns: OCRFile receives the tenant api_key JSON payload verbatim
// and resolves the bearer token and server endpoint itself, mirroring Python's
// PaddleOCROcrModel constructor.
func TestPaddleOCRModelOCRFileUnwrapsAPIKeyPayload(t *testing.T) {
	withSSRFBypass(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/ocr/jobs":
			if got, want := r.Header.Get("Authorization"), "Bearer tok-123"; got != want {
				t.Errorf("submit Authorization = %q, want %q", got, want)
			}
			_, _ = w.Write([]byte(`{"data":{"jobId":"job-1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/ocr/jobs/job-1":
			if got, want := r.Header.Get("Authorization"), "Bearer tok-123"; got != want {
				t.Errorf("poll Authorization = %q, want %q", got, want)
			}
			_, _ = w.Write([]byte(`{"data":{"state":"done","resultUrl":{"jsonUrl":"http://` + r.Host + `/result"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/result":
			_, _ = w.Write([]byte(`[{"logId":"l1","errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"# Unwrapped\n"}}]}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	driver := NewPaddleOCRModel(nil, URLSuffix{OCR: "v2/ocr/jobs"})
	apiKey := fmt.Sprintf(`{"paddleocr_api_url":%q,"paddleocr_access_token":"tok-123"}`, server.URL+"/api")
	emptyBaseURL := ""
	modelName := "PaddleOCR-VL-1.6"

	resp, err := driver.OCRFile(context.Background(), &modelName, []byte("%PDF-1.4"), nil, &APIConfig{ApiKey: &apiKey, BaseURL: &emptyBaseURL}, nil, nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if resp == nil || resp.Text == nil || !strings.Contains(*resp.Text, "Unwrapped") {
		t.Fatalf("OCRFile text = %v, want Unwrapped", resp)
	}
}

// TestPaddleOCRLocalModelOCRFileResolvesPayload pins the local driver's share
// of the same contract: bearer token and algorithm are resolved inside the
// driver from the tenant api_key payload.
func TestPaddleOCRLocalModelOCRFileResolvesPayload(t *testing.T) {
	withSSRFBypass(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/layout-parsing" {
			http.NotFound(w, r)
			return
		}
		if got, want := r.Header.Get("Authorization"), "Bearer tok-local"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if got, want := body["algorithm"], "PaddleOCR-VL"; got != want {
			t.Errorf("algorithm = %v, want %v", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":"# Local Title\n"}}]}}`))
	}))
	defer server.Close()

	driver := NewPaddleOCRLocalModel(nil, URLSuffix{OCR: "layout-parsing"})
	apiKey := fmt.Sprintf(`{"paddleocr_base_url":%q,"paddleocr_access_token":"tok-local","paddleocr_algorithm":"PaddleOCR-VL"}`, server.URL)
	emptyBaseURL := ""
	modelName := "PaddleOCR-VL"

	resp, err := driver.OCRFile(context.Background(), &modelName, []byte("%PDF-1.4"), nil, &APIConfig{ApiKey: &apiKey, BaseURL: &emptyBaseURL}, nil, nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if resp == nil || resp.Text == nil || !strings.Contains(*resp.Text, "Local Title") {
		t.Fatalf("OCRFile text = %v, want Local Title", resp)
	}
}

func TestParsePaddleOCRResultBody(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		pages      int
		want       string
	}{
		{"array", sampleArrayResult, 2, "天下皆知美之为美"},
		{"JSONL", `{"result":{"layoutParsingResults":[{"markdown":{"text":"first"}}]}}
{"result":{"layoutParsingResults":[{"markdown":{"text":"second"}}]}}`, 2, "first\n\nsecond"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := parsePaddleOCRResultBody([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Pages) != tc.pages || response.Text == nil || !strings.Contains(*response.Text, tc.want) {
				t.Fatalf("response=%+v", response)
			}
		})
	}
}

func TestParsePaddleOCRResultBodyRejectsErrors(t *testing.T) {
	for _, body := range []string{`[]`, `not json`, `{"result":{}}`, `[{"errorCode":1001,"errorMsg":"page failed"}]`, `{"errorCode":1001,"errorMsg":"page failed"}`} {
		if _, err := parsePaddleOCRResultBody([]byte(body)); err == nil {
			t.Fatalf("accepted failed result: %s", body)
		}
	}
}

// Missing structured pages must fail even when the OCR text is readable.
func TestPaddleOCRDriversPreserveLayout(t *testing.T) {
	withSSRFBypass(t)
	body, err := os.ReadFile("testdata/paddleocr_layout.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, local := range []bool{false, true} {
		t.Run(fmt.Sprintf("local=%v", local), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/layout-parsing":
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					for _, key := range []string{"useDocOrientationClassify", "useDocUnwarping"} {
						if request[key] != false {
							t.Errorf("%s=%v; want preprocessing disabled", key, request[key])
						}
					}
					w.Write(body)
				case r.URL.Path == "/result":
					var obj any
					json.Unmarshal(body, &obj)
					json.NewEncoder(w).Encode(obj)
				case r.Method == http.MethodPost:
					fmt.Fprint(w, `{"data":{"jobId":"layout"}}`)
				default:
					fmt.Fprintf(w, `{"data":{"state":"done","resultUrl":{"jsonUrl":%q}}}`, "http://"+r.Host+"/result")
				}
			}))
			defer server.Close()
			var driver ModelDriver = NewPaddleOCRModel(nil, URLSuffix{OCR: "api/v2/ocr/jobs"})
			if local {
				driver = NewPaddleOCRLocalModel(nil, URLSuffix{OCR: "layout-parsing"})
			}
			model, token := "PaddleOCR-VL-1.6", "test-token"
			response, err := driver.OCRFile(t.Context(), &model, []byte("%PDF-1.4"), nil, &APIConfig{BaseURL: &server.URL, ApiKey: &token}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Pages) != 1 {
				t.Fatalf("pages=%+v", response.Pages)
			}
			page := response.Pages[0]
			if page.Width != 1191 || page.Height != 1684 || len(page.Blocks) != 7 {
				t.Fatalf("page=%+v", page)
			}
			if page.Blocks[0].Text != "Paragraph 1." || fmt.Sprint(page.Blocks[0].BBox) != "[174 146 1015 240]" {
				t.Fatalf("block=%+v", page.Blocks[0])
			}
		})
	}
}

func TestPaddleOCRResultPageIdentity(t *testing.T) {
	cases := []struct {
		name, body string
		pages      int
		index      int
		text       string
		box        string
	}{
		{"block only", `{"result":{"layoutParsingResults":[{"prunedResult":{"width":1200,"height":1600,"parsing_res_list":[{"block_content":"body","block_bbox":[20,40,100,120]}]}}]}}`, 1, 0, "body", "[20 40 100 120]"},
		{"blank middle page", `{"result":{"ocrResults":[{"prunedResult":{"width":1200,"height":1600,"rec_texts":["first"],"rec_boxes":[[20,40,100,120]]}},{"prunedResult":{}},{"prunedResult":{"width":900,"height":1200,"rec_texts":["third","unlocated"],"rec_boxes":[[30,60,90,120]]}}]}}`, 3, 2, "third", "[30 60 90 120]"},
		{"explicit source index", `{"result":{"layoutParsingResults":[{"prunedResult":{"page_index":4,"parsing_res_list":[{"block_content":"fifth","block_bbox":[20,40,100,120]}]}}]}}`, 1, 4, "fifth", "[20 40 100 120]"},
		{"layout polygon becomes rectangle", `{"result":{"layoutParsingResults":[{"prunedResult":{"parsing_res_list":[{"block_content":"polygon","block_bbox":[300,160,30,140,50,40,280,60]}]}}]}}`, 1, 0, "polygon", "[30 40 300 160]"},
		{"recognition polygon becomes rectangle", `{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["polygon"],"rec_boxes":[[300,160,30,140,50,40,280,60]]}}]}}`, 1, 0, "polygon", "[30 40 300 160]"},
		{"unsupported box keeps text", `{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["retained"],"rec_boxes":[[1,2,3,4,5,6]]}}]}}`, 1, 0, "retained", "[]"},
		{"degenerate box keeps text", `{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["retained"],"rec_boxes":[[10,20,10,40]]}}]}}`, 1, 0, "retained", "[]"},
		{"null rectangle keeps text", `{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["retained"],"rec_boxes":[[10,null,30,40]]}}]}}`, 1, 0, "retained", "[]"},
		{"null polygon keeps text", `{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["retained"],"rec_boxes":[[30,40,null,160,100,160,100,40]]}}]}}`, 1, 0, "retained", "[]"},
		{"zero coordinate is valid", `{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["retained"],"rec_boxes":[[0,0,30,40]]}}]}}`, 1, 0, "retained", "[0 0 30 40]"},
		{"invalid box keeps text", `{"result":{"layoutParsingResults":[{"prunedResult":{"parsing_res_list":[{"block_content":"retained","block_bbox":[10,"bad",30,40]}]}}]}}`, 1, 0, "retained", "[]"},
		{"pretty object", "{\n\"result\": {\n\"ocrResults\": [{\"prunedResult\": {\"rec_texts\": [\"pretty\"]}}]\n}}", 1, 0, "pretty", "[]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, err := parsePaddleOCRResultBody([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			pages := response.Pages
			if len(pages) != tc.pages {
				t.Fatalf("pages=%+v", pages)
			}
			page := pages[len(pages)-1]
			if page.Index != tc.index || len(page.Blocks) == 0 || page.Blocks[0].Text != tc.text || fmt.Sprint(page.Blocks[0].BBox) != tc.box {
				t.Fatalf("page=%+v", page)
			}
		})
	}
}

func TestPaddleOCRPageMarkdownFallbackWhenBlocksHaveNoText(t *testing.T) {
	for _, tc := range []struct {
		name     string
		blocks   string
		wantText string
		wantBox  string
	}{
		{"no blocks", `[]`, "page fallback", "[]"},
		{"only header", `[{"block_label":"header","block_content":"ignored header","block_bbox":[10,20,30,40]}]`, "page fallback", "[]"},
		{"only page number", `[{"block_label":"number","block_content":"12"}]`, "page fallback", "[]"},
		{"empty blocks", `[{"block_content":"  "}]`, "page fallback", "[]"},
		{"markdown image", `[{"block_content":"![figure](https://example.invalid/a.png)","block_bbox":[10,20,30,40]}]`, "page fallback", "[]"},
		{"parenthesized image destination", `[{"block_content":"![figure](https://example.invalid/a(1).png)","block_bbox":[10,20,30,40]}]`, "page fallback", "[]"},
		{"html image", `[{"block_content":"<div><img src=\"a.png\"></div>"}]`, "page fallback", "[]"},
		{"text among images", `[{"block_content":"![figure](a.png)"},{"block_content":"positioned text","block_bbox":[10,20,30,40]}]`, "![figure](a.png)\npositioned text", "[10 20 30 40]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"result":{"layoutParsingResults":[{"markdown":{"text":"page fallback"},"prunedResult":{"page_index":2,"parsing_res_list":` + tc.blocks + `}}]}}`
			response, err := parsePaddleOCRResultBody([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			page := response.Pages[0]
			var texts []string
			var box []float64
			for _, block := range page.Blocks {
				if strings.TrimSpace(block.Text) != "" {
					texts = append(texts, block.Text)
					box = block.BBox
				}
			}
			if page.Index != 2 || strings.Join(texts, "\n") != tc.wantText || fmt.Sprint(box) != tc.wantBox {
				t.Fatalf("page=%+v", page)
			}
			if response.Text == nil || *response.Text != "page fallback" {
				t.Fatalf("API text changed: %+v", response.Text)
			}
		})
	}
}

func TestPaddleOCRPagesExcludeMarkdownIgnoredLabels(t *testing.T) {
	for _, label := range []string{"header", "header_image", "footer", "footer_image", "number", "aside_text", "footnote", " HEADER "} {
		t.Run(label, func(t *testing.T) {
			body := `{"result":{"layoutParsingResults":[{"markdown":{"text":"body"},"prunedResult":{"parsing_res_list":[{"block_label":"` + label + `","block_content":"ignored furniture"},{"block_label":"text","block_content":"body","block_bbox":[10,20,30,40]}]}}]}}`
			response, err := parsePaddleOCRResultBody([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			blocks := response.Pages[0].Blocks
			if len(blocks) != 1 || blocks[0].Text != "body" || fmt.Sprint(blocks[0].BBox) != "[10 20 30 40]" {
				t.Fatalf("blocks=%+v", blocks)
			}
		})
	}
}

func TestPaddleOCRRejectsMalformedJSONL(t *testing.T) {
	body := `{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["first"]}}]}}
{"result": corrupted
{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["third"]}}]}}`
	if _, err := parsePaddleOCRResultBody([]byte(body)); err == nil {
		t.Fatal("accepted malformed JSONL with ambiguous page identity")
	}
}

func TestPaddleOCRResultErrorIdentifiesEntry(t *testing.T) {
	body := `{"logId":"failed-page-log","errorCode":1001,"errorMsg":"page failed"}`
	_, err := parsePaddleOCRResultBody([]byte(body))
	if err == nil {
		t.Fatal("accepted failed page")
	}
	for _, part := range []string{"entry 1", "failed-page-log", "1001", "page failed"} {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("missing %q in error: %v", part, err)
		}
	}
}

func TestOCRFileResponseDoesNotExposePageBlocks(t *testing.T) {
	text := "recognized text"
	response := OCRFileResponse{Text: &text, Warnings: []string{"internal warning"}, Pages: []OCRPage{{Index: 0, Blocks: []OCRBlock{{Text: text, BBox: []float64{1, 2, 3, 4}}}}}}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"text":"recognized text"}` {
		t.Fatalf("public OCR response exposes internal data: %s", body)
	}
}

func TestPaddleOCRPartialFailuresPreservePageIdentity(t *testing.T) {
	first := `{"result":{"layoutParsingResults":[{"markdown":{"text":"first"}}]}}`
	failed := `{"logId":"bad-page","errorCode":1001,"errorMsg":"page failed"}`
	third := `{"result":{"layoutParsingResults":[{"prunedResult":{"page_index":2},"markdown":{"text":"third"}}]}}`
	for _, tc := range []struct {
		name, body string
		indexes    []int
		wantErr    bool
	}{
		{"explicit recovery", first + "\n" + failed + "\n" + third, []int{0, 2}, false},
		{"missing result recovery", first + "\n" + `{"result":{}}` + "\n" + third, []int{0, 2}, false},
		{"trailing failure", first + "\n" + failed, []int{0}, false},
		{"unknown page identity", first + "\n" + failed + "\n" + first, nil, true},
		{"known failed range", first + "\n" + `{"errorCode":1001,"result":{"layoutParsingResults":[{"prunedResult":{"page_index":1}},{"prunedResult":{"page_index":2}}]}}` + "\n" + first, []int{0, 3}, false},
		{"all failed", failed, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parsePaddleOCRResultBody([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatal("accepted result without trustworthy page identity or content")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Warnings) != 1 {
				t.Fatalf("partial failure warnings=%v", result.Warnings)
			}
			if len(result.Pages) != len(tc.indexes) {
				t.Fatalf("pages=%v", result.Pages)
			}
			for i, index := range tc.indexes {
				if result.Pages[i].Index != index {
					t.Fatalf("page %d index=%d; want %d", i, result.Pages[i].Index, index)
				}
			}
		})
	}
}

func TestPaddleOCRPageSizeFallbackRequiresAlignedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, fields  string
		width, height float64
	}{
		{"ordinal", ``, 1200, 1600},
		{"aligned index", `"page_index":0,`, 1200, 1600},
		{"ambiguous index", `"page_index":1,`, 0, 0},
		{"source dimensions", `"page_index":1,"width":900,"height":1100,`, 900, 1100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"result":{"layoutParsingResults":[{"prunedResult":{%s"parsing_res_list":[{"block_content":"body","block_bbox":[10,20,100,200]}]}}],"dataInfo":{"pages":[{"width":1200,"height":1600},{"width":900,"height":1100}]}}}`, tc.fields)
			result, err := parsePaddleOCRResultBody([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			page := result.Pages[0]
			if page.Width != tc.width || page.Height != tc.height {
				t.Fatalf("dimensions=(%v,%v); want (%v,%v)", page.Width, page.Height, tc.width, tc.height)
			}
			if len(page.Blocks) != 1 || page.Blocks[0].Text != "body" {
				t.Fatalf("text lost: %v", page)
			}
		})
	}
}

func TestPaddleOCRRejectsRepeatedOrDecreasingSourcePages(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"repeated across entries", `{"logId":"first","result":{"layoutParsingResults":[{"prunedResult":{"page_index":0},"markdown":{"text":"first"}}]}}
{"logId":"second","result":{"layoutParsingResults":[{"prunedResult":{"page_index":0},"markdown":{"text":"second"}}]}}`},
		{"decreasing across entries", `{"result":{"layoutParsingResults":[{"prunedResult":{"page_index":2},"markdown":{"text":"first"}}]}}
{"result":{"layoutParsingResults":[{"prunedResult":{"page_index":1},"markdown":{"text":"second"}}]}}`},
		{"repeated within entry", `{"result":{"layoutParsingResults":[{"prunedResult":{"page_index":0},"markdown":{"text":"first"}},{"prunedResult":{"page_index":0},"markdown":{"text":"second"}}]}}`},
		{"decreasing after inferred page", `{"result":{"layoutParsingResults":[{"prunedResult":{"page_index":0},"markdown":{"text":"first"}},{"markdown":{"text":"second"}},{"prunedResult":{"page_index":1},"markdown":{"text":"third"}}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePaddleOCRResultBody([]byte(tc.body))
			if err == nil || !strings.Contains(err.Error(), "source page index") || !strings.Contains(err.Error(), "entry") {
				t.Fatalf("expected identifiable source-page error, got %v", err)
			}
		})
	}
	body := `{"result":{"layoutParsingResults":[{"prunedResult":{"page_index":2},"markdown":{"text":"third"}},{"prunedResult":{"page_index":4},"markdown":{"text":"fifth"}}]}}`
	response, err := parsePaddleOCRResultBody([]byte(body))
	if err != nil || len(response.Pages) != 2 || response.Pages[0].Index != 2 || response.Pages[1].Index != 4 {
		t.Fatalf("valid source-page gaps rejected: response=%+v err=%v", response, err)
	}
}
