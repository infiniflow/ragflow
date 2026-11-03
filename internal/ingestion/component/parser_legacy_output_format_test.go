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
	"testing"

	"ragflow/internal/parser/parser"
)

// TestLegacyOutputFormatKeyIsIgnored pins the one contract that survives the
// key's removal: a persisted canvas may still carry output_format in a Parser
// family setup, and no reader consumes it — the component builds, parses, and
// emits the single JSON protocol regardless of the baked value.
func TestLegacyOutputFormatKeyIsIgnored(t *testing.T) {
	const eml = "From: a@example.com\r\n" +
		"To: b@example.com\r\n" +
		"Subject: Hi\r\n" +
		"Date: Mon, 01 Jan 2024 00:00:00 +0000\r\n" +
		"\r\n" +
		"Hello body.\r\n"

	cases := []struct {
		name     string
		params   map[string]any
		filename string
		binary   []byte
	}{
		{
			name:     "markdown legacy text",
			params:   map[string]any{"markdown": map[string]any{"output_format": "text"}},
			filename: "note.md",
			binary:   []byte("# Heading\n\nBody paragraph.\n"),
		},
		{
			name:     "email legacy text",
			params:   map[string]any{"email": map[string]any{"output_format": "text"}},
			filename: "mail.eml",
			binary:   []byte(eml),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp, err := NewParserComponent(tc.params)
			if err != nil {
				t.Fatalf("NewParserComponent: %v", err)
			}
			out, err := comp.Invoke(t.Context(), nil, map[string]any{"binary": tc.binary, "name": tc.filename})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if got := out["output_format"]; got != "json" {
				t.Fatalf("output_format = %v, want json", got)
			}
			items, ok := out["json"].([]parser.DocItem)
			if !ok || len(items) == 0 {
				t.Fatalf("json items = %#v, want non-empty", out["json"])
			}
		})
	}
}
