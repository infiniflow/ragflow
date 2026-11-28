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

package tool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"ragflow/internal/utility"
)

// legacyTruncateRunes mirrors the former per-tool two-argument helper
// template. It is kept only as an equivalence oracle for the shared
// implementation on positive limits.
func legacyTruncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func TestTruncateRunesEquivalence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		limit int
		want  string
	}{
		{"empty value", "", 10000, ""},
		{"ascii shorter than limit", "hello", 10000, "hello"},
		{"ascii exactly at limit", "hello", 5, "hello"},
		{"cjk exactly at limit", "你好世界", 4, "你好世界"},
		{"emoji exactly at limit", "😀😃😄😁", 4, "😀😃😄😁"},
		{"multibyte byte length exceeds limit", "你好", 2, "你好"},
		{"ascii over limit", "hello world", 5, "hello"},
		{"cjk over limit", "你好世界", 2, "你好"},
		{"emoji over limit", "😀😃😄😁", 2, "😀😃"},
		{"mixed scripts over limit", "ab中😀cd", 3, "ab中"},
		{"single multibyte rune", "你好", 1, "你"},
		{"zero limit", "abc", 0, ""},
		{"negative limit", "abc", -1, ""},
		{"zero limit empty value", "", 0, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := utility.TruncateRunes(tc.value, tc.limit)
			if got != tc.want {
				t.Fatalf("utility.TruncateRunes(%q, %d) = %q, want %q", tc.value, tc.limit, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("utility.TruncateRunes(%q, %d) produced invalid UTF-8: %q", tc.value, tc.limit, got)
			}
			if tc.limit > 0 {
				if legacy := legacyTruncateRunes(tc.value, tc.limit); legacy != got {
					t.Fatalf("shared and legacy truncation disagree for (%q, %d): legacy=%q shared=%q", tc.value, tc.limit, legacy, got)
				}
			}
		})
	}
}

// legacyTruncateRunesHelpers is the exact set of two-argument per-tool helpers
// converged by this change. truncateAnySearchRunes is intentionally excluded:
// it takes three arguments and implements separate ellipsis semantics.
var legacyTruncateRunesHelpers = map[string]bool{
	"truncateGoogleRunes":        true,
	"truncateBGPTRunes":          true,
	"truncateDuckDuckGoRunes":    true,
	"truncatePerplexityRunes":    true,
	"truncateGitHubRunes":        true,
	"truncateQueritRunes":        true,
	"truncateWikipediaRunes":     true,
	"truncatePubMedRunes":        true,
	"truncateSearch1APIRunes":    true,
	"truncateYouComRunes":        true,
	"truncateSofyaRunes":         true,
	"truncateKeenableRunes":      true,
	"truncateArxivRunes":         true,
	"truncateGoogleScholarRunes": true,
	"truncateTavilyRunes":        true,
}

func TestAgentToolRunesTruncationConverged(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	var leftovers []string
	sharedCalls := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			switch expr := node.(type) {
			case *ast.FuncDecl:
				if legacyTruncateRunesHelpers[expr.Name.Name] {
					leftovers = append(leftovers, fmt.Sprintf("%s: helper %s still defined", name, expr.Name.Name))
				}
			case *ast.CallExpr:
				if ident, ok := expr.Fun.(*ast.Ident); ok && legacyTruncateRunesHelpers[ident.Name] {
					leftovers = append(leftovers, fmt.Sprintf("%s: call %s still present", name, ident.Name))
				}
				if sel, ok := expr.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "TruncateRunes" {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "utility" && len(expr.Args) == 2 {
						sharedCalls++
						assertPositiveRunesLimit(t, fset, expr.Args[1])
					}
				}
			}
			return true
		})
	}

	if len(leftovers) > 0 {
		t.Errorf("legacy per-tool rune truncation is not fully converged:\n  %s", strings.Join(leftovers, "\n  "))
	}
	if sharedCalls < 16 {
		t.Errorf("expected at least 16 call sites migrated to utility.TruncateRunes, found %d", sharedCalls)
	}
}

func assertPositiveRunesLimit(t *testing.T, fset *token.FileSet, arg ast.Expr) {
	t.Helper()

	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		t.Errorf("%s: utility.TruncateRunes limit must be an integer literal, got %T", fset.Position(arg.Pos()), arg)
		return
	}
	limit, err := strconv.Atoi(lit.Value)
	if err != nil || limit <= 0 {
		t.Errorf("%s: utility.TruncateRunes limit must be a positive integer, got %s", fset.Position(arg.Pos()), lit.Value)
	}
}
