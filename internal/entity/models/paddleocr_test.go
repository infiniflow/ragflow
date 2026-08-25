package models

import (
	"strings"
	"testing"
)

// sampleArrayResult mirrors the actual response shape observed from the
// paddle_ocr.net online service: a JSON array of page objects where each
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
