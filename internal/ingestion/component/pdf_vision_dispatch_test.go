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

import "testing"

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

// TestIsNamedPDFParseMethodLayoutSuffixes verifies the "@"-suffixed
// layout_recognizer spelling is recognized per backend. "@mineru" must be
// included to mirror the MinerU dispatch branch in maybeDispatchPDFVision
// (pdf_vision_dispatch.go:64-68); without it a "foo@mineru" parse_method
// would be misrouted to the generic vision path instead of MinerU.
func TestIsNamedPDFParseMethodLayoutSuffixes(t *testing.T) {
	suffixed := []string{
		"foo@mineru", "@mineru",
		"foo@paddleocr", "@paddleocr",
		"foo@somark", "@somark",
		"foo@opendataloader", "@opendataloader",
	}
	for _, v := range suffixed {
		if !isNamedPDFParseMethod(v) {
			t.Errorf("isNamedPDFParseMethod(%q) = false, want true (recognized layout suffix)", v)
		}
	}

	// A non-recognized suffix is a CustomVLM name, not a named method.
	if isNamedPDFParseMethod("foo@unknown") {
		t.Errorf("isNamedPDFParseMethod(%q) = true, want false", "foo@unknown")
	}
}
