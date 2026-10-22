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
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestParseResultHasNoTextField is the primary RED test for removing the dead
// ParseResult.Text field. The DSL hardcodes output_format to "json"
// (normalizeParserOutputFormats), so the only writers of Text — the DOC and
// Email "text" output branches — are unreachable in production. The field is
// dead; this test fails while it still exists and passes once it is deleted.
func TestParseResultHasNoTextField(t *testing.T) {
	typ := reflect.TypeOf(ParseResult{})
	if _, ok := typ.FieldByName("Text"); ok {
		t.Fatal("ParseResult.Text must be removed: it is dead (only populated on the DSL-removed text output path)")
	}
}

// TestEmailParserTextOutputRemoved verifies the Email parser no longer exposes a
// "text" output path. A setup requesting output_format "text" must be ignored
// and the result must be JSON, never a Text payload.
func TestEmailParserTextOutputRemoved(t *testing.T) {
	ctx := context.Background()
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: recipient@test.com",
		"Subject: Hello",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello, world!",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "text",
		"fields":        []string{"from", "to", "subject", "body"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.OutputFormat != "json" {
		t.Fatalf("email must ignore text output_format and emit json, got OutputFormat=%q", result.OutputFormat)
	}
	if len(result.JSON) == 0 {
		t.Fatalf("email json output must contain items, got empty")
	}
}
