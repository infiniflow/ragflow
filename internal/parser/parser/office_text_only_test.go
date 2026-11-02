//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"testing"

	"ragflow/internal/utility"
)

func TestGetTextOnlyParserRoutesOfficeFamilies(t *testing.T) {
	tests := []struct {
		fileType utility.FileType
		isText   func(ParseResultProducer) bool
	}{
		{utility.FileTypeDOCX, func(p ParseResultProducer) bool {
			parser, ok := p.(*DOCXParser)
			return ok && parser.textOnly
		}},
		{utility.FileTypeDOC, func(p ParseResultProducer) bool {
			parser, ok := p.(*DOCParser)
			return ok && parser.textOnly
		}},
		{utility.FileTypePPTX, func(p ParseResultProducer) bool {
			parser, ok := p.(*PPTXParser)
			return ok && parser.textOnly
		}},
		{utility.FileTypePPT, func(p ParseResultProducer) bool {
			parser, ok := p.(*PPTParser)
			return ok && parser.pptx.textOnly
		}},
		{utility.FileTypeXLSX, func(p ParseResultProducer) bool {
			parser, ok := p.(*XLSXParser)
			return ok && parser.textOnly
		}},
		{utility.FileTypeXLS, func(p ParseResultProducer) bool {
			parser, ok := p.(*XLSParser)
			return ok && parser.textOnly
		}},
	}
	for _, tc := range tests {
		t.Run(string(tc.fileType), func(t *testing.T) {
			p, err := GetTextOnlyParser(tc.fileType)
			if err != nil {
				t.Fatalf("GetTextOnlyParser: %v", err)
			}
			if !tc.isText(p) {
				t.Fatalf("GetTextOnlyParser(%q) = %T, want the text-only parser", tc.fileType, p)
			}
		})
	}
}
