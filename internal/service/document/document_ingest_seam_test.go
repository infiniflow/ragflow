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

package document

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestIngestUsesDocumentWriteSeam guards a contract that OSS behavior alone
// cannot observe: Ingest must authorize each document through the shared seam,
// not through an inline database checker. Other document-write paths already
// do this; Ingest must not be the lone exception.
func TestIngestUsesDocumentWriteSeam(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "document_ingest.go", nil, 0)
	if err != nil {
		t.Fatalf("parse document_ingest.go: %v", err)
	}
	var ingest *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "Ingest" {
			ingest = fn
			break
		}
	}
	if ingest == nil {
		t.Fatal("Ingest function not found")
	}

	seamCalled := false
	seamGetsDocID := false
	inlineAuth := false
	ast.Inspect(ingest.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "CheckDocumentWrite":
			seamCalled = true
			for _, arg := range call.Args {
				if ident, ok := arg.(*ast.Ident); ok && ident.Name == "docID" {
					seamGetsDocID = true
				}
			}
		case "CheckResource", "FilterResources":
			inlineAuth = true
		}
		return true
	})

	if !seamCalled {
		t.Fatal("Ingest must call service.CheckDocumentWrite for each document")
	}
	if !seamGetsDocID {
		t.Fatal("the seam must receive docID so per-document authorization can run")
	}
	if inlineAuth {
		t.Fatal("Ingest must not authorize inline through checker.CheckResource/FilterResources")
	}
}
