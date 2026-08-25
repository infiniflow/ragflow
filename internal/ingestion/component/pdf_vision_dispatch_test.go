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

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

func TestMaybeDispatchPDFVisionEnhancementForwardsDatasetLanguage(t *testing.T) {
	origResolver := resolveTenantModelByType
	origInvoker := visionChatInvoker
	origPrompt := figureVisionPromptBuilder
	t.Cleanup(func() {
		resolveTenantModelByType = origResolver
		visionChatInvoker = origInvoker
		figureVisionPromptBuilder = origPrompt
	})

	resolveTenantModelByType = func(context.Context, *gorm.DB, string, entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		return &docxVisionFakeDriver{}, "pdf-vision-model", &modelModule.APIConfig{}, 0, nil
	}
	invoker := &docxVisionCaptureInvoker{}
	visionChatInvoker = invoker.invoke
	capturedLanguage := ""
	figureVisionPromptBuilder = func(_, _, language string) (string, error) {
		capturedLanguage = language
		return "describe the figure", nil
	}

	dispatched := parserDispatchResult{
		OutputFormat: "json",
		DocType:      "pdf",
		JSON: []map[string]any{
			{"text": "caption", "image": "data:image/png;base64,aW1hZ2U=", "doc_type_kwd": "image"},
		},
	}

	res, modified, err := maybeDispatchPDFVisionEnhancement(
		t.Context(),
		dao.DB,
		utility.FileTypePDF,
		dispatched,
		map[string]any{"tenant_id": "t1", "lang": "Dutch"},
	)
	if err != nil {
		t.Fatalf("maybeDispatchPDFVisionEnhancement: %v", err)
	}
	if !modified {
		t.Fatal("modified = false, want true")
	}
	if capturedLanguage != "Dutch" {
		t.Fatalf("figure prompt language = %q, want Dutch", capturedLanguage)
	}
	if got := res.JSON[0]["text"]; got != "caption\na diagram of a pipeline" {
		t.Fatalf("enhanced text = %q", got)
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
