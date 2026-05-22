package models

import (
	"strings"
	"testing"
)

func TestBaiduOCRFileValidation(t *testing.T) {
	model := NewBaiduModel(
		map[string]string{"default": "http://unused"},
		URLSuffix{OCR: "ocr/paddleocr"},
	)
	modelName := "paddleocr-vl-0.9b"
	apiKey := "test-key"
	content := []byte("image bytes")

	tests := []struct {
		name      string
		modelName *string
		content   []byte
		fileURL   *string
		apiConfig *APIConfig
		wantErr   string
	}{
		{
			name:      "missing api config",
			modelName: &modelName,
			content:   content,
			apiConfig: nil,
			wantErr:   "api key",
		},
		{
			name:      "missing api key",
			modelName: &modelName,
			content:   content,
			apiConfig: &APIConfig{},
			wantErr:   "api key",
		},
		{
			name:      "missing model name",
			modelName: nil,
			content:   content,
			apiConfig: &APIConfig{ApiKey: &apiKey},
			wantErr:   "model name",
		},
		{
			name:      "missing file input",
			modelName: &modelName,
			content:   nil,
			apiConfig: &APIConfig{ApiKey: &apiKey},
			wantErr:   "image url or content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("OCRFile panicked: %v", recovered)
				}
			}()

			_, err := model.OCRFile(tt.modelName, tt.content, tt.fileURL, tt.apiConfig, nil)
			if err == nil {
				t.Fatalf("OCRFile error = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("OCRFile error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
