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

package parser

import (
	"context"
	"strings"
	"testing"
)

// TestRechunkEmailAttachmentsSingleReturn guards the dead-code cleanup of
// rechunkEmailAttachments: it must return only the re-parsed attachment items
// ([]map[string]any) and must no longer build/return a discarded flat-text
// string. If the second return value is reintroduced, this test fails to
// compile.
func TestRechunkEmailAttachmentsSingleReturn(t *testing.T) {
	p := NewEmailParser()
	ctx := context.Background()

	// depth>0 must stop recursion immediately.
	if got := p.rechunkEmailAttachments(ctx, map[string]any{}, 1); got != nil {
		t.Fatalf("depth>0 should return nil, got %d items", len(got))
	}

	// No attachments -> empty, never a nil-panic on a removed return value.
	if got := p.rechunkEmailAttachments(ctx, map[string]any{}, 0); len(got) != 0 {
		t.Fatalf("no attachments should yield 0 items, got %d", len(got))
	}

	// A non-text (image) attachment is skipped, not re-parsed.
	image := map[string]any{
		"attachments": []map[string]any{
			{"filename": "pic.png", "payload": "binary", "raw": "binary"},
		},
	}
	if got := p.rechunkEmailAttachments(ctx, image, 0); len(got) != 0 {
		t.Fatalf("image attachment should be skipped, got %d items", len(got))
	}

	// A .txt attachment re-parses into a searchable text item.
	txt := map[string]any{
		"attachments": []map[string]any{
			{"filename": "note.txt", "payload": "hello attachment"},
		},
	}
	got := p.rechunkEmailAttachments(ctx, txt, 0)
	if len(got) != 1 {
		t.Fatalf("txt attachment should yield 1 item, got %d", len(got))
	}
	if got[0]["doc_type_kwd"] != "text" {
		t.Errorf("item doc_type_kwd = %v, want text", got[0]["doc_type_kwd"])
	}
	if text, ok := got[0]["text"].(string); !ok || !strings.Contains(text, "hello attachment") {
		t.Errorf("item text = %v, want it to contain %q", got[0]["text"], "hello attachment")
	}
}
