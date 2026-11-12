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

package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAgentHandlersClearWriteDeadline guards the fix for issue #21649.
//
// http.Server carries a hard-coded WriteTimeout (180s), and WriteTimeout is a
// deadline for the whole response, not an idle timeout. Two kinds of handler
// are therefore cut off by it and both must call clearResponseWriteDeadline:
//
//   - a streaming handler, which keeps writing past the deadline;
//   - a handler that runs an agent turn and computes the whole answer before
//     its first — and only — write.
//
// The check is structural, so it sees both: a handler is in scope when it sets
// a text/event-stream Content-Type *or* when it drives an agent turn through a
// chat runner. The second condition matters because a handler can run a full
// agent turn and never set the SSE header — AgentChatCompletions with
// `stream: false` and the non-stream path of handleOpenAICompat did exactly
// that, and were missed for that reason.
//
// This is a source-level guard rather than a behavioural test because reaching
// these branches needs databases, Redis and a chat runner; the failure mode is
// simply "a handler that can run long forgot the call", which is what an AST
// walk can see.
//
// What this guard does NOT prove: clearsWriteDeadline asks whether the function
// body calls the helper somewhere, not whether every long path reaches a call.
// A function that clears the deadline on one branch and not another still
// passes, because the one call satisfies the whole body. Pinning per-branch
// reachability — including the order of the clear relative to the call it
// protects — needs statement-level analysis or a behavioural test (a server
// with a short WriteTimeout plus a blocked chat runner), which is more than
// this cheap structural check is worth.
//
// runsAgentTurn deliberately over-approximates: it matches any `.RunAgent(`
// call, so a handler that runs an agent turn under some other condition is
// still covered. The cost of over-matching is only a redundant
// clearResponseWriteDeadline call, which is harmless.
func TestAgentHandlersClearWriteDeadline(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if !needsClearedWriteDeadline(fn.Body) || clearsWriteDeadline(fn.Body) {
				continue
			}
			pos := fset.Position(fn.Pos())
			offenders = append(offenders, pos.String()+" "+fn.Name.Name)
		}
	}

	if len(offenders) > 0 {
		t.Errorf("handlers that can outlive http.Server.WriteTimeout must call clearResponseWriteDeadline (issue #21649):\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// needsClearedWriteDeadline reports whether a handler can run longer than the
// server write deadline: it streams SSE, or it drives an agent turn through a
// chat runner and only writes once the run completes.
func needsClearedWriteDeadline(body *ast.BlockStmt) bool {
	return streamsSSE(body) || runsAgentTurn(body)
}

// runsAgentTurn reports whether the body calls into the agent chat runner.
func runsAgentTurn(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "RunAgent" {
			found = true
		}
		return true
	})
	return found
}

// streamsSSE reports whether the body sets a response Content-Type to
// text/event-stream. Request headers are deliberately not matched: some
// handlers send "Accept: ... text/event-stream" to an upstream MCP server.
func streamsSSE(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		isHeaderSet := sel.Sel.Name == "Set" || sel.Sel.Name == "Header"
		if !isHeaderSet {
			return true
		}
		key, okKey := stringLit(call.Args[0])
		value, okValue := stringLit(call.Args[1])
		if okKey && okValue && key == "Content-Type" && strings.Contains(value, "text/event-stream") {
			found = true
		}
		return true
	})
	return found
}

// clearsWriteDeadline reports whether the body calls clearResponseWriteDeadline.
func clearsWriteDeadline(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "clearResponseWriteDeadline" {
			found = true
		}
		return true
	})
	return found
}

func stringLit(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}
