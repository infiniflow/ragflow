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

package infinity

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	infinity "github.com/infiniflow/infinity-go-sdk"
)

// TestMetadataStringPredicatesRoundTrip checks the value that the pinned SDK
// actually sends, rather than merely inspecting the generated SQL text.
func TestMetadataStringPredicatesRoundTrip(t *testing.T) {
	values := []string{
		`plain`, `a"b`, `c:\temp`, "line\nbreak", "tab\tvalue", "nul\x00value",
		"中文🙂", `o'brien`, `a\"b'c`, `\u0061`, `a\\b`, `trailing\`, `a, (b) "c'd"`, `O'Reilly`,
	}
	for _, op := range []string{"=", "≠", "contains", "not contains", "in", "not in"} {
		for _, value := range values {
			t.Run(op+"/"+value, func(t *testing.T) {
				var input interface{} = value
				if op == "in" || op == "not in" {
					input = []interface{}{value}
				}
				sql, err := NewMetaFilterTranslator().Translate(map[string]interface{}{"op": op, "key": "label", "value": input})
				if err != nil {
					t.Fatal(err)
				}
				expr, err := infinity.ParseFilter(sql)
				if err != nil {
					t.Fatalf("SDK cannot parse %q: %v", sql, err)
				}
				// IN wraps its predicate in a balanced OR group; negative
				// operators add NOT. The leaf is the JSON_CONTAINS call.
				for expr.Type.FunctionExpr != nil && expr.Type.FunctionExpr.FunctionName != "json_contains" && expr.Type.FunctionExpr.FunctionName != "JSON_CONTAINS" {
					args := expr.Type.FunctionExpr.Arguments
					if len(args) != 1 {
						t.Fatalf("unexpected expression for %q: %#v", sql, expr.Type)
					}
					expr = args[0]
				}
				if expr.Type.FunctionExpr == nil {
					t.Fatalf("expected JSON_CONTAINS for %q", sql)
				}
				args := expr.Type.FunctionExpr.Arguments
				if len(args) != 3 || args[2].Type.ConstantExpr == nil || args[2].Type.ConstantExpr.StrValue == nil {
					t.Fatalf("expected JSON string argument for %q", sql)
				}
				payload := *args[2].Type.ConstantExpr.StrValue
				var decoded string
				if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
					t.Fatalf("invalid JSON sent by SDK %q (SQL %q): %v", payload, sql, err)
				}
				want := value
				if op == "in" || op == "not in" {
					want = strings.ToLower(value)
				}
				if decoded != want {
					t.Fatalf("SDK sent %q, want %q (JSON %q, SQL %q)", decoded, want, payload, sql)
				}
			})
		}
	}
}

// TestMetadataStringPredicatesCompose checks the complete predicate builder
// across multiple conditions and mixed string/numeric membership lists.
func TestMetadataStringPredicatesCompose(t *testing.T) {
	for _, logic := range []string{"and", "or"} {
		for _, op := range []string{"in", "not in"} {
			t.Run(logic+"/"+op, func(t *testing.T) {
				values := []interface{}{`a, (b) "c'd"`, `trailing\`, 2026}
				sql, err := BuildInfinityFilter([]map[string]interface{}{
					{"key": "label", "op": op, "value": values},
					{"key": "author", "op": "=", "value": `O'Reilly`},
				}, logic)
				if err != nil {
					t.Fatal(err)
				}
				expr, err := infinity.ParseFilter(sql)
				if err != nil {
					t.Fatalf("SDK cannot parse %q: %v", sql, err)
				}
				if expr.Type.FunctionExpr == nil {
					t.Fatalf("expected compound predicate: %q", sql)
				}
				// Derive the stack type from the SDK API without depending on its
				// internal generated Thrift package.
				stack := append(expr.Type.FunctionExpr.Arguments[:0:0], expr)
				var got []string
				numbers := 0
				for len(stack) > 0 {
					node := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					fn := node.Type.FunctionExpr
					if fn == nil {
						continue
					}
					if strings.EqualFold(fn.FunctionName, "json_contains") {
						if len(fn.Arguments) != 3 || fn.Arguments[2].Type.ConstantExpr == nil {
							t.Fatalf("bad JSON_CONTAINS: %q", sql)
						}
						constant := fn.Arguments[2].Type.ConstantExpr
						if constant.StrValue != nil {
							var value string
							if err := json.Unmarshal([]byte(*constant.StrValue), &value); err != nil {
								t.Fatalf("bad JSON in %q: %v", sql, err)
							}
							got = append(got, value)
						} else if constant.I64Value != nil && *constant.I64Value == 2026 {
							numbers++
						} else {
							t.Fatalf("unexpected numeric literal in %q: %#v", sql, constant)
						}
					} else {
						stack = append(stack, fn.Arguments...)
					}
				}
				slices.Sort(got)
				want := []string{`a, (b) "c'd"`, `trailing\`, `O'Reilly`}
				slices.Sort(want)
				if !slices.Equal(got, want) || numbers != 1 {
					t.Fatalf("SDK sent strings %q, numeric count %d; want %q and 1", got, numbers, want)
				}
			})
		}
	}
}
