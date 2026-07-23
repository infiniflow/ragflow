//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestParserComponent_Check covers the construction-time business
// validation that mirrors the applicable subset of Python
// ParserParam.check() (rag/flow/parser/parser.py:251-321).
//
// Go does NOT validate audio/video vlm.llm_id because media_dispatch
// resolves tenant default models via resolveTenantModelByType, not
// setup["vlm"]["llm_id"]. See plan: quantum-forging-curie-sZ_7zRZb.
func TestParserComponent_Check(t *testing.T) {
	cases := []struct {
		name    string
		setups  map[string]schema.ParserSetup
		wantErr string // non-empty substring expected in error; empty means no error
	}{
		// --- PDF family (parser.py:252-261) ---
		{
			name:    "pdf: parse_method empty → error",
			setups:  map[string]schema.ParserSetup{"pdf": {"parse_method": ""}},
			wantErr: "Parse method abnormal",
		},
		{
			name:    "pdf: parse_method missing → error",
			setups:  map[string]schema.ParserSetup{"pdf": {}},
			wantErr: "Parse method abnormal",
		},
		{
			name:   "pdf: deepdoc (whitelist) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "deepdoc"}},
		},
		{
			name:   "pdf: plain_text (whitelist, case-insensitive) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "PLAIN_TEXT"}},
		},
		{
			name:   "pdf: tcadp parser (whitelist with space) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "tcadp parser"}},
		},
		{
			name:    "pdf: unknown VLM method without lang → error",
			setups:  map[string]schema.ParserSetup{"pdf": {"parse_method": "some_vlm", "lang": ""}},
			wantErr: "PDF VLM language",
		},
		{
			name:   "pdf: unknown VLM method with lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "some_vlm", "lang": "English"}},
		},
		{
			name:   "pdf: paddleocr (whitelist) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "paddleocr"}},
		},

		// --- image family (parser.py:283-287) ---
		{
			name:   "image: ocr without lang → pass (no lang check for OCR)",
			setups: map[string]schema.ParserSetup{"image": {"parse_method": "ocr"}},
		},
		{
			name:   "image: ocr with empty lang → pass (OCR skips lang)",
			setups: map[string]schema.ParserSetup{"image": {"parse_method": "ocr", "lang": ""}},
		},
		{
			name:    "image: non-ocr without lang → error",
			setups:  map[string]schema.ParserSetup{"image": {"parse_method": "vlm_xyz", "lang": ""}},
			wantErr: "Image VLM language",
		},
		{
			name:   "image: non-ocr with lang → pass",
			setups: map[string]schema.ParserSetup{"image": {"parse_method": "vlm_xyz", "lang": "English"}},
		},
		{
			name:   "image: missing parse_method → pass (treated as non-ocr, but lang defaults empty in DSL)",
			setups: map[string]schema.ParserSetup{"image": {"lang": "English"}},
		},

		// --- audio/video: vlm.llm_id NOT validated in Go ---
		{
			name:   "audio: no vlm field → pass (Go uses tenant default ASR)",
			setups: map[string]schema.ParserSetup{"audio": {"output_format": "text"}},
		},
		{
			name:   "video: no vlm field → pass (Go uses tenant default VISION)",
			setups: map[string]schema.ParserSetup{"video": {"output_format": "text"}},
		},
		{
			name:   "audio: vlm.llm_id empty → pass (Go ignores vlm.llm_id)",
			setups: map[string]schema.ParserSetup{"audio": {"vlm": map[string]any{"llm_id": ""}}},
		},

		// --- empty / default configurations ---
		{
			name:   "empty setups → pass",
			setups: map[string]schema.ParserSetup{},
		},
		{
			name:   "nil setups → pass",
			setups: nil,
		},
		{
			name:   "only unrelated family → pass",
			setups: map[string]schema.ParserSetup{"markdown": {"output_format": "json"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &ParserComponent{Setups: tc.setups, Param: schema.ParserParam{}.Defaults()}
			err := c.Check()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil error, got: %v", err)
			}
		})
	}
}

// TestParserComponent_New_RunsCheck asserts NewParserComponent
// invokes Check() at construction time so a malformed DSL surfaces
// as a canvas compile failure rather than a mid-run error.
func TestParserComponent_New_RunsCheck(t *testing.T) {
	// Default config must pass (audio/video vlm not required in Go).
	c, err := NewParserComponent(nil)
	if err != nil {
		t.Fatalf("NewParserComponent(nil): %v", err)
	}
	if c == nil {
		t.Fatal("NewParserComponent(nil) returned nil component")
	}

	// Malformed PDF parse_method must fail at construction.
	c, err = NewParserComponent(map[string]any{
		"pdf": map[string]any{"parse_method": "", "lang": ""},
	})
	if err == nil {
		t.Fatal("NewParserComponent with empty pdf.parse_method: want error, got nil")
	}
	if !strings.Contains(err.Error(), "Parse method abnormal") {
		t.Errorf("error = %q, want substring %q", err.Error(), "Parse method abnormal")
	}
	if c != nil {
		t.Errorf("want nil component on error, got %T", c)
	}
}
