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

//go:build cgo

package office

import "testing"

func TestJoinElements_MultiParagraphCell(t *testing.T) {
	// When a table cell contains multiple paragraphs, joinElements must
	// insert a newline between them to match python-docx _Cell.text behavior.
	els := []irElement{
		{Type: "paragraph", Content: []irRun{{Type: "text", Text: "first line"}}},
		{Type: "paragraph", Content: []irRun{{Type: "text", Text: "second line"}}},
	}
	got := joinElements(els)
	want := "first line\nsecond line"
	if got != want {
		t.Errorf("joinElements:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestJoinElements_SingleElement(t *testing.T) {
	// Single paragraph cell — no separator expected.
	els := []irElement{
		{Type: "paragraph", Content: []irRun{{Type: "text", Text: "single paragraph"}}},
	}
	got := joinElements(els)
	want := "single paragraph"
	if got != want {
		t.Errorf("joinElements:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestJoinElements_Empty(t *testing.T) {
	got := joinElements(nil)
	if got != "" {
		t.Errorf("joinElements(nil): got %q, want empty", got)
	}
}
