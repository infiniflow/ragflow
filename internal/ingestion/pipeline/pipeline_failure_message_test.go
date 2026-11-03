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

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestFormatComponentFailure covers the operator-facing failure formatting:
// per-component timeout guidance, prefix stripping, and the bare fallbacks.
// These messages are what an operator reads when a document fails, so the
// component identity in them must keep matching the registered names.
func TestFormatComponentFailure(t *testing.T) {
	t.Run("nil error reports the component only", func(t *testing.T) {
		if got := formatComponentFailure("Extractor", nil); got != "Extractor Error" {
			t.Fatalf("formatComponentFailure(nil) = %q, want %q", got, "Extractor Error")
		}
	})

	t.Run("extractor timeout gets extraction guidance", func(t *testing.T) {
		got := formatComponentFailure("Extractor", context.DeadlineExceeded)
		want := "Extractor: Document extraction timed out while waiting for the configured LLM. Check that the model service is reachable, then retry."
		if got != want {
			t.Fatalf("formatComponentFailure = %q, want %q", got, want)
		}
	})

	t.Run("compiler timeout gets compilation guidance", func(t *testing.T) {
		got := formatComponentFailure("Compiler", context.DeadlineExceeded)
		want := "Compiler: Document compilation timed out. Check that the configured model service is reachable and responsive, then retry."
		if got != want {
			t.Fatalf("formatComponentFailure = %q, want %q", got, want)
		}
	})

	t.Run("other components fall through to the generic form", func(t *testing.T) {
		got := formatComponentFailure("Tokenizer", context.DeadlineExceeded)
		want := "Tokenizer: context deadline exceeded"
		if got != want {
			t.Fatalf("formatComponentFailure = %q, want %q", got, want)
		}
	})

	t.Run("wrapped deadline is detected", func(t *testing.T) {
		err := fmt.Errorf("component failed: %w", context.DeadlineExceeded)
		got := formatComponentFailure("Extractor", err)
		if !strings.Contains(got, "timed out") {
			t.Fatalf("formatComponentFailure(wrapped) = %q, want the timeout guidance", got)
		}
	})

	t.Run("timeout wording inside the message is detected", func(t *testing.T) {
		err := errors.New("LLM call failed: CONTEXT DEADLINE EXCEEDED")
		got := formatComponentFailure("Extractor", err)
		if !strings.Contains(got, "timed out") {
			t.Fatalf("formatComponentFailure(mixed case) = %q, want the timeout guidance", got)
		}
	})

	t.Run("component prefix is stripped before reformatting", func(t *testing.T) {
		got := formatComponentFailure("Extractor", errors.New("Extractor: model quota exceeded"))
		if got != "Extractor: model quota exceeded" {
			t.Fatalf("formatComponentFailure = %q, want the prefix not doubled", got)
		}
	})

	t.Run("mixed case prefix is stripped", func(t *testing.T) {
		got := formatComponentFailure("Extractor", errors.New("extractor: model quota exceeded"))
		if got != "Extractor: model quota exceeded" {
			t.Fatalf("formatComponentFailure = %q, want the case-insensitive prefix stripped", got)
		}
	})

	t.Run("empty component name returns the bare message", func(t *testing.T) {
		got := formatComponentFailure("", errors.New("boom"))
		if got != "boom" {
			t.Fatalf("formatComponentFailure(\"\") = %q, want %q", got, "boom")
		}
	})

	t.Run("empty component name and nil error", func(t *testing.T) {
		if got := formatComponentFailure("", nil); got != " Error" {
			t.Fatalf("formatComponentFailure(\"\", nil) = %q, want %q", got, " Error")
		}
	})
}

// TestPipelineHasNoBareComponentNameLiterals guards the constant consolidation:
// component identity strings in the pipeline package must come from the shared
// schema constants, not inline literals, so a rename touches one place instead
// of a grep. The scan is AST-based so comments (which may mention a name in
// prose) are not flagged.
func TestPipelineHasNoBareComponentNameLiterals(t *testing.T) {
	banned := []string{
		`"Extractor"`, `"extractor"`,
		`"Compiler"`, `"compiler"`,
		`"Parser"`, `"parser"`,
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read pipeline package dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			for _, want := range banned {
				if lit.Value == want {
					t.Errorf("%s contains the bare component-name literal %s; use the shared schema constant", name, want)
				}
			}
			return true
		})
	}
}
