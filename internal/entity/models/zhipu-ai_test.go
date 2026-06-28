package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestZhipuAIOCRFileSendsLayoutParsingRequest(t *testing.T) {
	apiKey := "test-key"
	modelName := "glm-ocr"
	fileURL := "https://example.com/doc.png"
	expectedText := "# OCR result"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/layout_parsing" {
			t.Errorf("path = %s, want /layout_parsing", r.URL.Path)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Errorf("Authorization = %q", got)
			return
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
			return
		}

		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if req["model"] != modelName {
			t.Errorf("model = %q, want %q", req["model"], modelName)
			return
		}
		if req["file"] != fileURL {
			t.Errorf("file = %q, want %q", req["file"], fileURL)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"md_results": expectedText})
	}))
	defer server.Close()

	model := NewZhipuAIModel(map[string]string{"default": server.URL}, URLSuffix{OCR: "layout_parsing"})
	resp, err := model.OCRFile(&modelName, nil, &fileURL, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("OCRFile returned error: %v", err)
	}
	if resp == nil || resp.Text == nil || *resp.Text != expectedText {
		t.Fatalf("OCRFile text = %#v, want %q", resp, expectedText)
	}
}

func TestZhipuAIOCRFileEncodesContent(t *testing.T) {
	apiKey := "test-key"
	modelName := "glm-ocr"
	content := []byte("sample image bytes")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if !strings.HasPrefix(req["file"], "data:text/plain; charset=utf-8;base64,") {
			t.Errorf("file = %q, want base64 data URL", req["file"])
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"md_results": "ok"})
	}))
	defer server.Close()

	model := NewZhipuAIModel(map[string]string{"default": server.URL}, URLSuffix{OCR: "layout_parsing"})
	if _, err := model.OCRFile(&modelName, content, nil, &APIConfig{ApiKey: &apiKey}, nil); err != nil {
		t.Fatalf("OCRFile returned error: %v", err)
	}
}

func TestZhipuAIOCRFileDetectsPDFContent(t *testing.T) {
	apiKey := "test-key"
	modelName := "glm-ocr"
	content := []byte("%PDF-1.7 sample")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if !strings.HasPrefix(req["file"], "data:application/pdf;base64,") {
			t.Errorf("file = %q, want PDF data URL", req["file"])
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"md_results": "ok"})
	}))
	defer server.Close()

	model := NewZhipuAIModel(map[string]string{"default": server.URL}, URLSuffix{OCR: "layout_parsing"})
	if _, err := model.OCRFile(&modelName, content, nil, &APIConfig{ApiKey: &apiKey}, nil); err != nil {
		t.Fatalf("OCRFile returned error: %v", err)
	}
}

func TestZhipuAIOCRFileValidation(t *testing.T) {
	apiKey := "test-key"
	modelName := "glm-ocr"
	fileURL := "https://example.com/doc.png"
	model := NewZhipuAIModel(map[string]string{"default": "https://example.com"}, URLSuffix{OCR: "layout_parsing"})

	tests := []struct {
		name      string
		modelName *string
		fileURL   *string
		apiConfig *APIConfig
		want      string
	}{
		{
			name:      "missing api key",
			modelName: &modelName,
			fileURL:   &fileURL,
			apiConfig: &APIConfig{},
			want:      "api key is required",
		},
		{
			name:      "missing model name",
			modelName: nil,
			fileURL:   &fileURL,
			apiConfig: &APIConfig{ApiKey: &apiKey},
			want:      "model name is required",
		},
		{
			name:      "missing file",
			modelName: &modelName,
			fileURL:   nil,
			apiConfig: &APIConfig{ApiKey: &apiKey},
			want:      "file url or content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := model.OCRFile(tt.modelName, nil, tt.fileURL, tt.apiConfig, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
