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

package oceanbase

import "testing"

func TestMarkTextEnglishBoundariesAndCase(t *testing.T) {
	text := "Apple, cat and dog; concatenate is different."
	got := newHighlightMarker([]string{"apple", "cat", "dog"}).markText(text, "")
	want := "<em>Apple</em>, <em>cat</em> and <em>dog</em>; concatenate is different."
	if got != want {
		t.Fatalf("markText() = %q, want %q", got, want)
	}
}

func TestMarkTextUsesTokenizedNonEnglishText(t *testing.T) {
	text := "这是数据库系统"
	if got := newHighlightMarker([]string{"数据库"}).markText(text, "这是 数据库 系统"); got != "这是<em>数据库</em>系统" {
		t.Fatalf("markText() = %q", got)
	}
	if got := newHighlightMarker([]string{"数据"}).markText(text, "这是 数据库 系统"); got != "" {
		t.Fatalf("partial token match = %q, want no highlight", got)
	}
}

func TestMarkTextReturnsEmptyWithoutMatch(t *testing.T) {
	if got := newHighlightMarker([]string{"missing"}).markText("no relevant text", ""); got != "" {
		t.Fatalf("markText() = %q, want empty", got)
	}
}
