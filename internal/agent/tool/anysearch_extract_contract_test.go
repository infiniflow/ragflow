// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tool

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Extraction format detection belongs to AnySearch. These controlled replies
// verify delivery and failure handling, not vendor format support.
func TestAnySearchExtractFormatContracts(t *testing.T) {
	for _, tc := range []struct {
		name, content string
	}{
		{"xhtml", "XHTML page text: 日本語 résumé"},
		{"text", "First line\nSecond line"},
		{"json", `{"count":9007199254740993,"nested":{"value":"保留"}}`},
		{"markdown", "# Heading\n\n[Source](https://example.com/)\n\n**Evidence**"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pageURL := "https://example.com/page." + tc.name
			helper, recorder := anySearchTestServer(t, anySearchExtractPage(pageURL, "Format contract", tc.content))
			extract := NewAnySearchExtractToolWith(helper, "")
			out, env := runAnySearchExtract(t, extract, anySearchExtractCall(pageURL))
			if env.Error != "" || env.Content != tc.content || len(recorder.all()) != 1 {
				t.Fatalf("extraction changed vendor text: %s", out)
			}
			var decoded map[string]any
			if err := json.Unmarshal([]byte(out), &decoded); err != nil {
				t.Fatal(err)
			}
			chunks, _ := extract.BuildReferences(t.Context(), decoded)
			if len(chunks) != 1 || chunks[0]["content"] != strings.Join(strings.Fields(tc.content), " ") {
				t.Fatal("format text lost its reference")
			}
		})
	}
	for _, name := range []string{"pdf", "docx", "xlsx", "png", "mp3", "mp4", "zip", "oversized-json", "oversized-markdown"} {
		t.Run("vendor-rejection/"+name, func(t *testing.T) {
			helper, recorder := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
				writeAnySearch(w, http.StatusUnprocessableEntity, `{"code":422,"message":"unsupported or oversized document","request_id":"rid-format"}`)
			})
			extract := NewAnySearchExtractToolWith(helper, "")
			out, env := runAnySearchExtract(t, extract, anySearchExtractCall("https://example.com/"+name))
			var decoded map[string]any
			if err := json.Unmarshal([]byte(out), &decoded); err != nil {
				t.Fatal(err)
			}
			chunks, docs := extract.BuildReferences(t.Context(), decoded)
			if len(recorder.all()) != 1 || !strings.Contains(env.Error, "HTTP 422") || env.RequestID != "rid-format" || env.Content != "" || len(chunks) != 0 || len(docs) != 0 {
				t.Fatalf("vendor failure became evidence: %s", out)
			}
		})
	}
}
