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
	"testing"

	pdftype "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestSectionsToMaps(t *testing.T) {
	sections := []pdftype.Section{
		{
			Text:       "Hello World",
			LayoutType: "text",
			DocTypeKwd: "text",
			Positions: []pdftype.Position{
				{PageNumbers: []int{1}, Left: 10, Right: 100, Top: 50, Bottom: 80},
			},
		},
		{
			Text:       "Table Data",
			LayoutType: "table",
			DocTypeKwd: "table",
			Image:      "base64imgdata",
		},
		{
			Text:       "",
			LayoutType: "figure",
			DocTypeKwd: "image",
			Image:      "base64imgdata",
		},
	}

	result := sectionsToMaps(sections)

	if len(result) != 3 {
		t.Fatalf("got %d sections, want 3", len(result))
	}

	// section 0: text with positions
	s0 := result[0]
	if s0["text"] != "Hello World" {
		t.Errorf("text = %v, want Hello World", s0["text"])
	}
	if s0["doc_type_kwd"] != "text" {
		t.Errorf("doc_type_kwd = %v, want text", s0["doc_type_kwd"])
	}
	positions, ok := s0["positions"].([]pdftype.Position)
	if !ok || len(positions) != 1 {
		t.Fatal("missing positions")
	}
	if positions[0].PageNumbers[0] != 1 {
		t.Errorf("page number = %d, want 1", positions[0].PageNumbers[0])
	}
	// img_id should be empty (no image)
	if s0["img_id"] != "" {
		t.Errorf("img_id = %v, want empty string", s0["img_id"])
	}

	// section 1: table with image
	s1 := result[1]
	if s1["text"] != "Table Data" {
		t.Errorf("text = %v, want Table Data", s1["text"])
	}
	if s1["doc_type_kwd"] != "table" {
		t.Errorf("doc_type_kwd = %v, want table", s1["doc_type_kwd"])
	}
	// img_id should be set since Image is not empty
	if s1["img_id"] != "base64imgdata" {
		t.Errorf("img_id = %v, want base64imgdata", s1["img_id"])
	}

	// section 2: pure image, no text
	s2 := result[2]
	if s2["text"] != "" {
		t.Errorf("text = %v, want empty", s2["text"])
	}
	if s2["img_id"] != "base64imgdata" {
		t.Errorf("img_id = %v, want base64imgdata", s2["img_id"])
	}
}

func TestSectionsToMaps_EmptyInput(t *testing.T) {
	result := sectionsToMaps(nil)
	if len(result) != 0 {
		t.Errorf("got %d sections, want 0", len(result))
	}

	result = sectionsToMaps([]pdftype.Section{})
	if len(result) != 0 {
		t.Errorf("got %d sections, want 0", len(result))
	}
}
