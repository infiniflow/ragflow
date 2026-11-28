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

package cli

import (
	"strings"
	"testing"
)

func TestParseLoginRejectsUnterminatedQuotes(t *testing.T) {
	for _, input := range []string{
		"login user 'unfinished",
		`login user "unfinished`,
		"login user 'user@example.com' password 'unfinished",
	} {
		command, err := NewParser(input).parseAPILoginUser()
		if command != nil || err == nil || !strings.Contains(err.Error(), "unterminated quoted string") {
			t.Fatal("expected an unterminated quote error and no login command")
		}
		if strings.Contains(err.Error(), "unfinished") {
			t.Fatal("error includes the unfinished argument")
		}
	}
}

func TestLexerPreservesClosedQuotedStrings(t *testing.T) {
	for _, input := range []string{"'中文 with spaces'", `"中文 with spaces"`} {
		lexer := NewLexer(input)
		if token := lexer.NextToken(); token.Type != TokenQuotedString || token.Value != "中文 with spaces" {
			t.Fatalf("NextToken() = %#v, want a quoted string with the original value", token)
		}
		if token := lexer.NextToken(); token.Type != TokenEOF {
			t.Fatalf("next token = %#v, want EOF", token)
		}
	}
}
