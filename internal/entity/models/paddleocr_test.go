package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestParseOCRResultBodyArray(t *testing.T) {
	p := &PaddleOCRModel{}
	var md strings.Builder
	arrayParsed, scanned, skipped, emptyRes, content, errored, err := p.parseOCRResultBody([]byte(sampleArrayResult), &md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !arrayParsed {
		t.Fatalf("expected array parse, got jsonl fallback")
	}
	if scanned != 2 {
		t.Errorf("scannedLines = %d, want 2", scanned)
	}
	if skipped != 0 {
		t.Errorf("skippedLines = %d, want 0", skipped)
	}
	if emptyRes != 0 {
		t.Errorf("emptyResultLines = %d, want 0", emptyRes)
	}
	if content != 2 {
		t.Errorf("contentLines = %d, want 2", content)
	}
	if errored != 0 {
		t.Errorf("erroredLines = %d, want 0", errored)
	}
	out := md.String()
	if !strings.Contains(out, "道可道，非常道") {
		t.Errorf("markdown missing first page text: %q", out)
	}
	if !strings.Contains(out, "天下皆知美之为美") {
		t.Errorf("markdown missing second page text: %q", out)
	}
}

func TestParseOCRResultBodyJSONL(t *testing.T) {
	// Older payloads are one JSON object per line; the fallback path must
	// handle them too.
	jsonl := `{"logId":"a","result":{"layoutParsingResults":[{"markdown":{"text":"line one"}}]},"errorCode":0}
{"logId":"b","result":{"layoutParsingResults":[{"markdown":{"text":"line two"}}]},"errorCode":0}`
	p := &PaddleOCRModel{}
	var md strings.Builder
	arrayParsed, scanned, skipped, emptyRes, content, errored, err := p.parseOCRResultBody([]byte(jsonl), &md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if arrayParsed {
		t.Fatalf("expected jsonl fallback, got array parse")
	}
	if scanned != 2 {
		t.Errorf("scannedLines = %d, want 2", scanned)
	}
	if skipped != 0 {
		t.Errorf("skippedLines = %d, want 0", skipped)
	}
	if emptyRes != 0 {
		t.Errorf("emptyResultLines = %d, want 0", emptyRes)
	}
	if content != 2 {
		t.Errorf("contentLines = %d, want 2", content)
	}
	if errored != 0 {
		t.Errorf("erroredLines = %d, want 0", errored)
	}
	out := md.String()
	if !strings.Contains(out, "line one") || !strings.Contains(out, "line two") {
		t.Errorf("markdown missing jsonl text: %q", out)
	}
}

func TestParseOCRResultBodyEmpty(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantArr   bool
		wantEmpty int
	}{
		{"empty array", `[]`, true, 0},
		{"entries without text", `[{"logId":"a","result":{"layoutParsingResults":[]},"errorCode":0}]`, true, 1},
		{"garbage line", `not json`, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PaddleOCRModel{}
			var md strings.Builder
			arrayParsed, _, _, emptyRes, content, errored, err := p.parseOCRResultBody([]byte(tt.body), &md)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if errored != 0 {
				t.Errorf("erroredLines = %d, want 0", errored)
			}
			if arrayParsed != tt.wantArr {
				t.Errorf("arrayParsed = %v, want %v", arrayParsed, tt.wantArr)
			}
			if content != 0 {
				t.Errorf("contentLines = %d, want 0", content)
			}
			// Structurally valid entries that decode but carry no text are
			// counted as empty; unparseable content must never crash.
			if emptyRes != tt.wantEmpty {
				t.Errorf("emptyResultLines = %d, want %d", emptyRes, tt.wantEmpty)
			}
			if strings.TrimSpace(md.String()) != "" {
				t.Errorf("expected empty markdown, got %q", md.String())
			}
		})
	}
}

// TestParseOCRResultBodyErrored verifies that entries with a non-zero
// errorCode are counted as errored (not silently counted as empty), produce no
// text, and are handled identically on the array and jsonl paths.
func TestParseOCRResultBodyErrored(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"array path", `[{"logId":"l1","result":{"layoutParsingResults":[{"markdown":{"text":"should not surface"}}]},"errorCode":1001,"errorMsg":"page failed"}]`},
		{"jsonl path", `{"logId":"l1","result":{"layoutParsingResults":[{"markdown":{"text":"should not surface"}}]},"errorCode":1001,"errorMsg":"page failed"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PaddleOCRModel{}
			var md strings.Builder
			_, scanned, skipped, emptyRes, content, errored, err := p.parseOCRResultBody([]byte(tt.body), &md)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if scanned != 1 {
				t.Errorf("scannedLines = %d, want 1", scanned)
			}
			if skipped != 0 {
				t.Errorf("skippedLines = %d, want 0", skipped)
			}
			if emptyRes != 0 {
				t.Errorf("emptyResultLines = %d, want 0 (errored entry must not masquerade as empty)", emptyRes)
			}
			if content != 0 {
				t.Errorf("contentLines = %d, want 0", content)
			}
			if errored != 1 {
				t.Errorf("erroredLines = %d, want 1", errored)
			}
			if strings.TrimSpace(md.String()) != "" {
				t.Errorf("errored entry must not contribute text, got %q", md.String())
			}
		})
	}
}
