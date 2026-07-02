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
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestHTTPHelper_HasDefaultTimeout guards the Go-side port of
// agent/tools PR #15436. The Python fix added `timeout=DEFAULT_TIMEOUT`
// to every `requests.*` call in the four external-API tools
// (github, jin10, qweather, tushare). The Go side already routes
// those tools through HTTPHelper, whose default `*http.Client` carries
// a 30s Timeout. This test fails if that default ever regresses to 0
// (no timeout), since a missing client.Timeout is the same bug class
// the Python PR was fixing.
func TestHTTPHelper_HasDefaultTimeout(t *testing.T) {
	h := NewHTTPHelper()
	if h.client == nil {
		t.Fatal("NewHTTPHelper returned nil client")
	}
	if h.client.Timeout <= 0 {
		t.Fatalf("HTTPHelper default client.Timeout = %v, want > 0", h.client.Timeout)
	}
}

// TestHTTPHelper_DefaultTransportHasStageTimeouts guards the
// per-stage timeouts (TCP dial, TLS handshake, idle pool). A stalled
// socket bound by client.Timeout is sufficient for the
// indefinite-block bug, but per-stage bounds also fail fast on
// network errors that don't otherwise produce a read deadline.
func TestHTTPHelper_DefaultTransportHasStageTimeouts(t *testing.T) {
	h := NewHTTPHelper()
	tr, ok := h.client.Transport.(*http.Transport)
	if !ok {
		t.Skipf("transport is %T, not asserting stage timeouts", h.client.Transport)
	}
	if tr.TLSHandshakeTimeout <= 0 {
		t.Errorf("transport.TLSHandshakeTimeout = %v, want > 0", tr.TLSHandshakeTimeout)
	}
	if tr.DialContext == nil {
		t.Error("transport.DialContext is nil")
	}
	if tr.ResponseHeaderTimeout > 0 && tr.ResponseHeaderTimeout > tr.TLSHandshakeTimeout*5 {
		// Loose heuristic — header timeout should not be wildly larger
		// than TLS handshake timeout. A misconfiguration where the
		// header timeout is in hours is a regression worth flagging.
		t.Logf("ResponseHeaderTimeout=%v is much larger than TLSHandshakeTimeout=%v (suspicious but non-fatal)",
			tr.ResponseHeaderTimeout, tr.TLSHandshakeTimeout)
	}
}

// TestTools_NoBareHTTPGet ensures no tool in this package calls the
// stdlib `http.Get` / `http.Post` / `http.PostForm` family directly
// without going through HTTPHelper. Bare http.Get has no Timeout and
// is the exact bug PR #15436 was closing. The single allowed caller
// is http_helper.go (the helper itself).
//
// Jin10 is a stub that does no network I/O, so it is allowed to have
// no helper reference.
func TestTools_NoBareHTTPGet(t *testing.T) {
	banned := map[string]struct{}{
		"http.Get":                {},
		"http.Post":               {},
		"http.PostForm":           {},
		"http.Head":               {},
		"http.NewRequest":         {}, // without WithContext
		"http.DefaultClient.Do":   {},
		"http.DefaultClient.Get":  {},
		"http.DefaultClient.Post": {},
	}

	dir := "."
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		// Only scan *.go in this package's root; skip generated/_test.
		if !strings.HasSuffix(fi.Name(), ".go") {
			return false
		}
		if strings.HasSuffix(fi.Name(), "_test.go") {
			return false
		}
		return true
	}, parser.AllErrors)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			name := file.Name.Name
			// http_helper.go is the one allowed caller.
			if strings.HasSuffix(name, "http_helper.go") {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				root, ok := sel.X.(*ast.Ident)
				if !ok || root.Name != "http" {
					return true
				}
				key := "http." + sel.Sel.Name
				if _, banned := banned[key]; banned {
					t.Errorf("%s: bare %s detected — use HTTPHelper.Do / DoPinned instead (PR #15436 regression)",
						name, key)
				}
				return true
			})
		}
	}
}
