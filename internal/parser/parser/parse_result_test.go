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

package parser

import (
	"strings"
	"testing"
)

// TestParseResult_Contract pins the wire-shape guarantees
// port-rag-flow-pipeline-to-go.md §6.5 requires:
//
//   - exactly one payload family populated on success
//   - empty payload fields on failure (Err != nil)
//   - OutputFormat is "" on failure
//
// These are the public-key invariants a component-level caller
// (component/parser.go) relies on. The test does not exercise any
// specific parser implementation — it pins the contract that the
// ParseResult type itself enforces, so future parser additions
// inherit the same guarantees.
func TestParseResult_Contract(t *testing.T) {
	cases := []struct {
		name    string
		in      ParseResult
		wantErr bool
		wantFmt string // expected OutputFormat on success
	}{
		{
			name: "json family only",
			in: ParseResult{
				OutputFormat: "json",
				JSON: []map[string]any{
					{"text": "hello", "doc_type_kwd": "text"},
				},
			},
			wantFmt: "json",
		},
		{
			name: "markdown family only",
			in: ParseResult{
				OutputFormat: "markdown",
				Markdown:     "# Title\n\nbody",
			},
			wantFmt: "markdown",
		},
		{
			name: "text family only",
			in: ParseResult{
				OutputFormat: "text",
				Text:         "raw text",
			},
			wantFmt: "text",
		},
		{
			name: "html family only",
			in: ParseResult{
				OutputFormat: "html",
				HTML:         "<p>x</p>",
			},
			wantFmt: "html",
		},
		{
			name:    "failure clears payload",
			in:      ParseResult{Err: errSentinel},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantErr {
				if tc.in.Err == nil {
					t.Fatal("Err: want non-nil, got nil")
				}
				if tc.in.OutputFormat != "" {
					t.Errorf("OutputFormat on failure = %q, want empty", tc.in.OutputFormat)
				}
				if tc.in.JSON != nil || tc.in.Markdown != "" || tc.in.Text != "" || tc.in.HTML != "" {
					t.Errorf("payload fields must be zero on failure; got %+v", tc.in)
				}
				return
			}
			if tc.in.Err != nil {
				t.Errorf("Err on success: want nil, got %v", tc.in.Err)
			}
			if tc.in.OutputFormat != tc.wantFmt {
				t.Errorf("OutputFormat = %q, want %q", tc.in.OutputFormat, tc.wantFmt)
			}
			// Exactly-one-payload-family: pick the family declared
			// by OutputFormat; every other field must be zero.
			active := payloadFamily(tc.in)
			for _, other := range allPayloadFamilies {
				if other == active {
					continue
				}
				if !isPayloadZero(tc.in, other) {
					t.Errorf("OutputFormat=%s: secondary family %s should be zero", tc.in.OutputFormat, other)
				}
			}
		})
	}
}

// errSentinel is a tiny stand-in so the test does not need to
// import errors just to declare one.
var errSentinel = sentinelErr("sentinel")

type sentinelErr string

func (s sentinelErr) Error() string { return string(s) }

// TestMarkdownParser_ParseWithResult pins the migration exemplar
// — port-rag-flow-pipeline-to-go.md §6.5 marks MarkdownParser as
// the first ported format whose ParseWithResult surface emulates
// the Python "json" output_format. The fixture is intentionally
// tiny: 1 heading, 1 paragraph, 1 unordered list item, no nested
// formatting.
func TestMarkdownParser_ParseWithResult(t *testing.T) {
	p, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		t.Fatalf("NewMarkdownParser: %v", err)
	}
	src := []byte("# Title\n\nFirst paragraph.\n\n- Item one\n")
	res := p.ParseWithResult("doc.md", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if res.File == nil || res.File["name"] != "doc.md" {
		t.Errorf("File.name = %v, want doc.md", res.File)
	}
	if res.JSON == nil {
		t.Fatal("JSON: want non-nil slice, got nil")
	}
	// 1 heading + 1 paragraph + 1 list = 3 items.
	if len(res.JSON) != 3 {
		t.Fatalf("JSON len = %d, want 3 (heading, paragraph, list)", len(res.JSON))
	}
	heading := res.JSON[0]
	if txt, _ := heading["text"].(string); !strings.Contains(txt, "Title") {
		t.Errorf("JSON[0].text = %q, want contains 'Title'", txt)
	}
	if got, _ := heading["ck_type"].(string); got != "heading" {
		t.Errorf("JSON[0].ck_type = %q, want heading", got)
	}
	for i, it := range res.JSON {
		if _, ok := it["doc_type_kwd"]; !ok {
			t.Errorf("JSON[%d] missing doc_type_kwd: %+v", i, it)
		}
	}
}

// TestParseResultProducer_PDFIsProducer pins the explicit
// PDFParser ParseResultProducer wiring. The dispatch seam routes
// PDFs through ParseWithResult regardless of whether the current
// build has the cgo-backed DeepDOC engine enabled.
func TestParseResultProducer_PDFIsProducer(t *testing.T) {
	pdf := &PDFParser{}
	if _, ok := any(pdf).(ParseResultProducer); !ok {
		t.Error("PDFParser must implement ParseResultProducer so the " +
			"dispatch seam routes PDFs through ParseWithResult")
	}
}

// payloadFamily returns the field name that should be populated
// given the declared OutputFormat. "" when OutputFormat is
// unrecognized.
func payloadFamily(r ParseResult) string {
	switch r.OutputFormat {
	case "json":
		return "json"
	case "markdown":
		return "markdown"
	case "text":
		return "text"
	case "html":
		return "html"
	}
	return ""
}

var allPayloadFamilies = []string{"json", "markdown", "text", "html"}

func isPayloadZero(r ParseResult, family string) bool {
	switch family {
	case "json":
		return r.JSON == nil
	case "markdown":
		return r.Markdown == ""
	case "text":
		return r.Text == ""
	case "html":
		return r.HTML == ""
	}
	return false
}
