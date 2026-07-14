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

	"ragflow/internal/entity"
)

func TestGetParserByID_AllKnownIDs(t *testing.T) {
	tests := []struct {
		parserID string
		wantNil  bool
	}{
		// PDF-based parsers
		{string(entity.ParserTypeNaive), false},
		{string(entity.ParserTypePaper), false},
		{string(entity.ParserTypeBook), false},
		{string(entity.ParserTypeManual), false},
		{string(entity.ParserTypeLaws), false},
		{string(entity.ParserTypeQA), false},
		{string(entity.ParserTypeResume), false},
		{string(entity.ParserTypePicture), false},
		{string(entity.ParserTypeOne), false},
		{string(entity.ParserTypeTag), false},
		// Office parsers
		{string(entity.ParserTypePresentation), false},
		{string(entity.ParserTypeTable), false},
		// Special parsers
		{string(entity.ParserTypeAudio), false},
		{string(entity.ParserTypeEmail), false},
	}

	for _, tt := range tests {
		t.Run(tt.parserID, func(t *testing.T) {
			got, err := GetParserByID(tt.parserID)
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetParserByID(%q) = non-nil, want nil", tt.parserID)
				}
			} else {
				if err != nil {
					t.Errorf("GetParserByID(%q): %v", tt.parserID, err)
				}
				if got == nil {
					t.Errorf("GetParserByID(%q) = nil, want non-nil", tt.parserID)
				}
			}
		})
	}
}

func TestGetParserByID_InvalidID(t *testing.T) {
	_, err := GetParserByID("nonexistent")
	if err == nil {
		t.Fatal("expected error for invalid parser_id")
	}
}

func TestGetParserByID_EmptyID(t *testing.T) {
	_, err := GetParserByID("")
	if err == nil {
		t.Fatal("expected error for empty parser_id")
	}
}
