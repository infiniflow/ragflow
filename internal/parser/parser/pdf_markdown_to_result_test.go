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
	"context"
	"testing"
)

// TestPDFMarkdownToResultMatchesNormalizeMarkdown pins that the two PDF
// markdown entry points — pdfMarkdownToResult (remote-backed producers:
// mineru/docling/opendataloader) and (*PDFParser).NormalizeMarkdown — produce
// the same structured JSON for identical markdown. They must share one
// conversion + item-policy path so a markdown producer can never silently
// diverge from the local Markdown path. File metadata (page_count) is not
// compared because only the remote producers carry it.
func TestPDFMarkdownToResultMatchesNormalizeMarkdown(t *testing.T) {
	cases := []struct {
		name     string
		markdown string
	}{
		{"plain", "# Title\n\nSome body text.\n"},
		{"table", "# T\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\nbody\n"},
		{"remote_image_ignored", "![img](https://example.com/x.png)\n"},
		{"data_image", "![img](data:image/png;base64,abc)\n"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remote := pdfMarkdownToResult(context.Background(), "doc.md", tc.markdown, 7)
			local := NewPDFParser().NormalizeMarkdown(context.Background(), "doc.md", tc.markdown)
			if remote.Err != nil {
				t.Fatalf("pdfMarkdownToResult err: %v", remote.Err)
			}
			if local.Err != nil {
				t.Fatalf("NormalizeMarkdown err: %v", local.Err)
			}
			if len(remote.JSON) != len(local.JSON) {
				t.Fatalf("item count mismatch: remote %d vs local %d", len(remote.JSON), len(local.JSON))
			}
			for i := range remote.JSON {
				ra, la := remote.JSON[i].ToMap(), local.JSON[i].ToMap()
				if ra["doc_type_kwd"] != la["doc_type_kwd"] {
					t.Errorf("item %d doc_type_kwd: remote %v vs local %v", i, ra["doc_type_kwd"], la["doc_type_kwd"])
				}
				if ra["text"] != la["text"] {
					t.Errorf("item %d text: remote %q vs local %q", i, ra["text"], la["text"])
				}
				if (ra["table"] != nil) != (la["table"] != nil) {
					t.Errorf("item %d table presence mismatch: remote %v vs local %v", i, ra["table"] != nil, la["table"] != nil)
				}
			}
		})
	}
}
