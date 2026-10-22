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
	"math"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestParserComponent_Check covers construction-time validation: the pdf
// parse_method whitelist, the image ocr_enabled type, and the image
// text-source requirement (OCR off + vision enhancement off is rejected).
//
// audio/video vlm.llm_id is not validated: Python's check() has no
// such branch, and audio resolves its model at dispatch time with a
// tenant-default fallback.
func TestParserComponent_Check(t *testing.T) {
	cases := []struct {
		name    string
		setups  map[string]schema.ParserSetup
		enhance bool
		wantErr string // non-empty substring expected in error; empty means no error
	}{
		// --- PDF family (parser.py:252-261) ---
		{
			name:    "pdf: parse_method empty → error",
			setups:  map[string]schema.ParserSetup{"pdf": {"parse_method": ""}},
			wantErr: "parse method abnormal",
		},
		{
			name:    "pdf: parse_method missing → error",
			setups:  map[string]schema.ParserSetup{"pdf": {}},
			wantErr: "parse method abnormal",
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
			name:   "pdf: Plain Text (UI spelling of the plain-text option) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "Plain Text"}},
		},
		{
			name:   "pdf: plaintext (UI spelling, no space) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "plaintext"}},
		},
		{
			name:   "pdf: tcadp parser (whitelist with space) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "tcadp parser"}},
		},
		{
			// A stored canvas that names a model in parse_method but carries no
			// language is valid: the engine and the caption path each resolve
			// their own default at run time, and Check has no run inputs yet.
			name:   "pdf: unknown VLM method without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "some_vlm", "lang": ""}},
		},
		{
			name:   "pdf: unknown VLM method with lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "some_vlm", "lang": "English"}},
		},
		{
			name:   "pdf: paddleocr (whitelist) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "paddleocr"}},
		},
		{
			name:   "pdf: monkeyocrv2 (whitelist) without lang → pass",
			setups: map[string]schema.ParserSetup{"pdf": {"parse_method": "monkeyocrv2"}},
		},

		// --- image family (parser.py:283-287) ---
		{
			name:   "image: legacy ocr shape → pass",
			setups: map[string]schema.ParserSetup{"image": {"parse_method": "ocr"}},
		},
		{
			// Language is not validated anywhere: it is no longer an input to
			// captions, and Check runs before run inputs exist so it cannot see
			// the knowledge base value. These two cases only differ in lang.
			name:    "image: model reference with enhancement → pass regardless of lang",
			setups:  map[string]schema.ParserSetup{"image": {"parse_method": "vlm_xyz", "lang": ""}},
			enhance: true,
		},
		{
			// A legacy model reference disables OCR by construction, so without
			// the enhancement there is no text source left.
			name:    "image: model reference without enhancement → error",
			setups:  map[string]schema.ParserSetup{"image": {"parse_method": "vlm_xyz", "lang": "English"}},
			wantErr: "no text source",
		},
		{
			// Absent parse_method matches the backend's legacy inference, which
			// reads it as "run local OCR".
			name:   "image: no parse_method → pass",
			setups: map[string]schema.ParserSetup{"image": {"lang": "English"}},
		},
		{
			// The image contract keys off the presence of ocr_enabled, so a
			// wrong-typed value must not read as "absent" and fall back to the
			// legacy parse_method inference.
			name:    "image: non-boolean ocr_enabled → error",
			setups:  map[string]schema.ParserSetup{"image": {"ocr_enabled": "false"}},
			wantErr: "ocr_enabled must be a boolean",
		},
		{
			name:    "image: ocr_enabled false without enhancement → error",
			setups:  map[string]schema.ParserSetup{"image": {"ocr_enabled": false}},
			wantErr: "no text source",
		},
		{
			name:    "image: ocr_enabled false with enhancement → pass",
			setups:  map[string]schema.ParserSetup{"image": {"ocr_enabled": false}},
			enhance: true,
		},
		{
			name:   "image: ocr_enabled alongside a stale parse_method → pass (switch wins)",
			setups: map[string]schema.ParserSetup{"image": {"ocr_enabled": true, "parse_method": "ocr"}},
		},

		// --- audio/video: vlm.llm_id not validated (matches Python check()) ---
		{
			name:   "audio: no vlm field → pass (model falls back to tenant default)",
			setups: map[string]schema.ParserSetup{"audio": {"output_format": "text"}},
		},
		{
			name:   "video: no vlm field → pass",
			setups: map[string]schema.ParserSetup{"video": {"output_format": "text"}},
		},
		{
			name:   "audio: vlm.llm_id empty → pass (empty falls back to tenant default)",
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
		// --- linked-content ingestion (#21032): switches live under the
		// families that own a collector, nowhere else ---
		{
			name:    "html carries fetch_links → rejected",
			setups:  map[string]schema.ParserSetup{"html": {"fetch_links": true}},
			wantErr: "html has no hyperlink parameter",
		},
		{
			name:    "pdf carries collect_markdown_links → rejected",
			setups:  map[string]schema.ParserSetup{"pdf": {"parse_method": "deepdoc", "collect_markdown_links": true}},
			wantErr: "pdf has no hyperlink parameter",
		},
		{
			name:    "pdf budget out of range → rejected",
			setups:  map[string]schema.ParserSetup{"pdf": {"parse_method": "deepdoc", "fetch_links": true, "max_urls": 0}},
			wantErr: "has invalid value",
		},
		{
			name:    "pdf budget wrong type → rejected",
			setups:  map[string]schema.ParserSetup{"pdf": {"parse_method": "deepdoc", "fetch_links": "yes"}},
			wantErr: "has invalid value",
		},
		{
			name:    "timeout_s true is not a number → rejected",
			setups:  map[string]schema.ParserSetup{"docx": {"timeout_s": true}},
			wantErr: "has invalid value",
		},
		{
			name:   "markdown budgets in range → pass",
			setups: map[string]schema.ParserSetup{"markdown": {"fetch_links": true, "collect_markdown_links": true, "max_urls": 5, "timeout_s": 10}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &ParserComponent{setups: tc.setups, enableVisionEnhancement: tc.enhance}
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
	if !strings.Contains(err.Error(), "parse method abnormal") {
		t.Errorf("error = %q, want substring %q", err.Error(), "parse method abnormal")
	}
	if c != nil {
		t.Errorf("want nil component on error, got %T", c)
	}
}

// A hyperlink switch left at the params root never reaches a family setup, so it
// must fail at compile time instead of doing nothing (#21032) — including the
// map shape, which would otherwise be absorbed as a file-format family named
// after the key.
func TestNewParserComponentRejectsTopLevelHyperlinkKey(t *testing.T) {
	for _, key := range []string{"fetch_links", "collect_markdown_links", "max_urls", "max_bytes_per_url", "max_total_bytes", "timeout_s"} {
		for _, value := range []any{1, true, "5", map[string]any{"a": 1}, map[string]any{}} {
			if _, err := NewParserComponent(map[string]any{key: value}); err == nil {
				t.Errorf("NewParserComponent with top-level %q = %T: want error, got nil", key, value)
			}
		}
	}
	// An unrelated non-map key keeps the legacy lenient behaviour.
	if _, err := NewParserComponent(map[string]any{"outputs": 1}); err != nil {
		t.Errorf("NewParserComponent with unrelated scalar: want no error, got %v", err)
	}
}

func TestNewParserComponentVisionEnhancementSetting(t *testing.T) {
	for _, test := range []struct {
		name    string
		params  map[string]any
		enabled bool
	}{
		{name: "absent", params: nil},
		{name: "explicit false", params: map[string]any{"enable_vision_enhancement": false}},
		{name: "explicit true", params: map[string]any{"enable_vision_enhancement": true}, enabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			component, err := NewParserComponent(test.params)
			if err != nil {
				t.Fatalf("NewParserComponent: %v", err)
			}
			if got := component.(*ParserComponent).enableVisionEnhancement; got != test.enabled {
				t.Fatalf("enableVisionEnhancement = %v, want %v", got, test.enabled)
			}
		})
	}
	if _, err := NewParserComponent(map[string]any{"enable_vision_enhancement": "true"}); err == nil || !strings.Contains(err.Error(), "must be a boolean") {
		t.Fatalf("string switch error = %v, want boolean validation", err)
	}
}

// The config path is as strict as the schema ranges: an integral JSON float
// decodes, while a fractional value, a bool, a string or an out-of-int32 number
// is rejected rather than truncated or counted as 1.
func TestHyperlinkFetchParamFromSetupBudgetTypes(t *testing.T) {
	p, err := hyperlinkParamFromSetup(schema.ParserSetup{
		"max_urls": float64(7), "timeout_s": 3,
	})
	if err != nil {
		t.Fatalf("integral values rejected: %v", err)
	}
	if p.MaxURLs != 7 || p.TimeoutSeconds != 3 {
		t.Fatalf("parsed = %+v, want max_urls 7 and timeout_s 3", p)
	}
	for _, raw := range []any{3.5, true, "5", nil, math.MaxInt32 + 1.0} {
		if _, err := hyperlinkParamFromSetup(schema.ParserSetup{"max_urls": raw}); err == nil {
			t.Errorf("max_urls %v (%T) accepted, want rejection", raw, raw)
		}
	}
	if _, err := hyperlinkParamFromSetup(schema.ParserSetup{"fetch_links": "yes"}); err == nil {
		t.Error("fetch_links as a string accepted, want rejection")
	}
}
