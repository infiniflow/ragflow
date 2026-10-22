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

package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDOCParserTextOutputRemoved verifies the DOC parser no longer exposes a
// "text" output path. The parser only emits JSON regardless of any
// output_format request.
func TestDOCParserTextOutputRemoved(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "table.doc"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewDOCParser()
	res := p.ParseWithResult(context.Background(), "table.doc", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Fatalf("DOC must ignore text output_format and emit json, got OutputFormat=%q", res.OutputFormat)
	}
	if len(res.JSON) == 0 {
		t.Fatalf("DOC json output must contain items, got empty")
	}
}
