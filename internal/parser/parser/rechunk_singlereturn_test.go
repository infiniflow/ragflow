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
	"strings"
	"testing"
)

func TestRechunkEmailAttachmentsItems(t *testing.T) {
	p := NewEmailParser()
	ctx := t.Context()

	// depth>0 must stop recursion immediately.
	if got, err := p.rechunkEmailAttachments(ctx, map[string]any{}, 1); got != nil || err != nil {
		t.Fatalf("depth>0 should return nil, got %d items, err=%v", len(got), err)
	}

	// No attachments yield no searchable items.
	if got, err := p.rechunkEmailAttachments(ctx, map[string]any{}, 0); len(got) != 0 || err != nil {
		t.Fatalf("no attachments should yield 0 items, got %d, err=%v", len(got), err)
	}

	// A non-text (image) attachment is skipped, not re-parsed.
	image := map[string]any{
		"attachments": []map[string]any{
			{"filename": "pic.png", "payload": "binary", "raw": "binary"},
		},
	}
	if got, err := p.rechunkEmailAttachments(ctx, image, 0); len(got) != 0 || err != nil {
		t.Fatalf("image attachment should be skipped, got %d items, err=%v", len(got), err)
	}

	// A .txt attachment re-parses into a searchable text item.
	txt := map[string]any{
		"attachments": []map[string]any{
			{"filename": "note.txt", "payload": "hello attachment"},
		},
	}
	got, err := p.rechunkEmailAttachments(ctx, txt, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("txt attachment should yield 1 item, got %d, err=%v", len(got), err)
	}
	if got[0]["doc_type_kwd"] != "text" {
		t.Errorf("item doc_type_kwd = %v, want text", got[0]["doc_type_kwd"])
	}
	if text, ok := got[0]["text"].(string); !ok || !strings.Contains(text, "hello attachment") {
		t.Errorf("item text = %v, want it to contain %q", got[0]["text"], "hello attachment")
	}
}
