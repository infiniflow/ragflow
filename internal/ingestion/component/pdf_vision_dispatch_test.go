//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"context"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

// paddleOCRFakeDriver embeds the ModelDriver interface and only implements
// the OCRFile method needed by dispatchPaddleOCRPdf.
type paddleOCRFakeDriver struct {
	modelModule.ModelDriver
	text string
}

func (d *paddleOCRFakeDriver) Name() string { return "PaddleOCR" }

func (d *paddleOCRFakeDriver) OCRFile(_ context.Context, _ *string, _ []byte, _ *string, _ *modelModule.APIConfig, _ *modelModule.OCRConfig, _ *common.ModelUsage) (*modelModule.OCRFileResponse, error) {
	return &modelModule.OCRFileResponse{Text: &d.text}, nil
}

// TestDispatchPaddleOCRPdfLabelsPayloadAsMarkdown guards against the
// format-mismatch bug: PaddleOCR backends always return markdown text via
// OCRFile.Text, so the dispatch result MUST be labelled OutputFormat
// "markdown" regardless of what setup["output_format"] says (the pdf default
// is "json"). If the setup value leaked into OutputFormat, buildParserOutputs
// would emit a nil "json" payload and the downstream TokenChunker would
// consume an empty JSONResult -> "completed with 0 chunks".
func TestDispatchPaddleOCRPdfLabelsPayloadAsMarkdown(t *testing.T) {
	orig := resolvePaddleOCRModelForDispatch
	t.Cleanup(func() { resolvePaddleOCRModelForDispatch = orig })

	md := "## 《道德经》全文及翻译\n\n道可道，非常道。"
	resolvePaddleOCRModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		baseURL := "http://localhost:9380"
		return &paddleOCRFakeDriver{text: md}, "ocr-model", &modelModule.APIConfig{BaseURL: &baseURL}, nil
	}

	// The real run had output_format=json in the pdf setup; the payload must
	// still be labelled markdown because that is what the backend produced.
	res, err := dispatchPaddleOCRPdf(t.Context(), dao.DB, "test.pdf", []byte("%PDF-1.4"), "tenant", schema.ParserSetup{"output_format": "json"}, "some-uuid")
	if err != nil {
		t.Fatalf("dispatchPaddleOCRPdf: %v", err)
	}
	if res.OutputFormat != "markdown" {
		t.Errorf("OutputFormat = %q, want markdown", res.OutputFormat)
	}
	if res.Markdown != md {
		t.Errorf("Markdown = %q, want %q", res.Markdown, md)
	}

	// Default setup (no output_format key) must behave identically.
	res, err = dispatchPaddleOCRPdf(t.Context(), dao.DB, "test.pdf", []byte("%PDF-1.4"), "tenant", nil, "some-uuid")
	if err != nil {
		t.Fatalf("dispatchPaddleOCRPdf (default setup): %v", err)
	}
	if res.OutputFormat != "markdown" {
		t.Errorf("OutputFormat = %q, want markdown", res.OutputFormat)
	}
	if res.Markdown != md {
		t.Errorf("Markdown = %q, want %q", res.Markdown, md)
	}
}

// TestDispatchPaddleOCRPdfEmptyTextFails guards against the silent-empty
// result: OCRFileResponse.Text is a *string that stays non-nil even when the
// backend produced zero text, so the old nil-only guard let an empty payload
// through and the pipeline emitted a "completed with 0 chunks" document.
// Empty text must surface as an explicit error instead.
func TestDispatchPaddleOCRPdfEmptyTextFails(t *testing.T) {
	orig := resolvePaddleOCRModelForDispatch
	t.Cleanup(func() { resolvePaddleOCRModelForDispatch = orig })

	resolvePaddleOCRModelForDispatch = func(context.Context, *gorm.DB, string, string) (modelModule.ModelDriver, string, *modelModule.APIConfig, error) {
		baseURL := "http://localhost:9380"
		return &paddleOCRFakeDriver{text: ""}, "ocr-model", &modelModule.APIConfig{BaseURL: &baseURL}, nil
	}

	res, err := dispatchPaddleOCRPdf(t.Context(), dao.DB, "test.pdf", []byte("%PDF-1.4"), "tenant", nil, "some-uuid")
	if err == nil {
		t.Fatalf("dispatchPaddleOCRPdf with empty text: expected error, got result %+v", res)
	}
	if res.OutputFormat != "" || res.Markdown != "" {
		t.Errorf("expected zero-value result on error, got %+v", res)
	}
}

// TestIsNamedPDFParseMethodWhitelistAligned verifies that the runtime
// "named parse_method" classifier agrees with (*ParserComponent).Check()'s
// PDF whitelist (parser.go:200-203):
//
//	deepdoc, plain_text, mineru, docling,
//	opendataloader, tcadp parser, paddleocr, somark
//
// Diff 2.10: a parse_method that Check() rejects must NOT be treated as a
// recognized named method by isNamedPDFParseMethod — otherwise it silently
// falls through to the CustomVLM vision path instead of failing fast at
// construction (and Python would have rejected it outright).
func TestIsNamedPDFParseMethodWhitelistAligned(t *testing.T) {
	// Values that MUST be recognized (subset of the Check() whitelist,
	// case-insensitive).
	named := []string{
		"deepdoc", "plain_text", "mineru", "docling",
		"opendataloader", "tcadp parser", "paddleocr", "somark",
		"DeepDoc", "PLAIN_TEXT", "MinerU", "DocLing",
		"OpenDataLoader", "TCADP Parser", "PaddleOCR", "SoMark",
	}
	for _, v := range named {
		if !isNamedPDFParseMethod(v) {
			t.Errorf("isNamedPDFParseMethod(%q) = false, want true (in Check() whitelist)", v)
		}
	}

	// Values that MUST NOT be recognized. These either duplicate the
	// whitelist with non-canonical spelling ("plain text"/"plaintext")
	// or are bare-family abbreviations ("tcadp") that Check() does not
	// accept, so they should be funneled to the CustomVLM path (or fail
	// construction) rather than masquerading as a named method.
	notNamed := []string{
		"plain text", "plaintext", "tcadp",
		"CustomVLM", "some_vlm", "gpt-4o",
		"", "  ",
	}
	for _, v := range notNamed {
		if isNamedPDFParseMethod(v) {
			t.Errorf("isNamedPDFParseMethod(%q) = true, want false (not in Check() whitelist)", v)
		}
	}
}

// TestIsNamedPDFParseMethodLayoutSuffixes verifies that "@"-suffixed
// layout_recognizer spellings are NOT treated as named parse methods. They
// are layout_recognizer selectors (resolved separately at
// pdf_vision_dispatch.go:62-68), and Check() rejects them as parse_method,
// so they must fall through to the CustomVLM/VLM path — consistent with the
// (*ParserComponent).Check() whitelist (parser.go:200-203).
func TestIsNamedPDFParseMethodLayoutSuffixes(t *testing.T) {
	suffixed := []string{
		"foo@mineru", "@mineru",
		"foo@paddleocr", "@paddleocr",
		"foo@somark", "@somark",
		"foo@opendataloader", "@opendataloader",
	}
	for _, v := range suffixed {
		if isNamedPDFParseMethod(v) {
			t.Errorf("isNamedPDFParseMethod(%q) = true, want false (layout_recognizer selector, not a named parse_method)", v)
		}
	}

	// An unknown suffix is also not a named method.
	if isNamedPDFParseMethod("foo@unknown") {
		t.Errorf("isNamedPDFParseMethod(%q) = true, want false", "foo@unknown")
	}
}
