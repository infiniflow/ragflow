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

package ingestion

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenPackage describes one package whose process-wide singleton
// accessors orchestration-layer production code must not reach for directly.
// The singletons are resolved once at the server assembly points (cmd, server)
// and passed down explicitly (see ingestion.Deps).
type forbiddenPackage struct {
	// importPath is the package path. A file only counts as accessing the
	// global when it also imports this package, which keeps unrelated
	// identifiers named "db" or "engine" from false-positiving.
	importPath string
	// pkgName is the default package identifier (last element of importPath).
	pkgName string
	// symbols are the package-level singleton accessors that are forbidden
	// outside the assembly points.
	symbols []string
}

var forbiddenPackages = []forbiddenPackage{
	{importPath: "ragflow/internal/dao", pkgName: "dao", symbols: []string{"DB"}},
	{importPath: "ragflow/internal/engine", pkgName: "engine", symbols: []string{"Get*"}},
	{importPath: "ragflow/internal/engine/kvrocks", pkgName: "kvrocks", symbols: []string{"Get"}},
}

// globalAccessWhitelist lists the few production files that legitimately
// resolve a process-wide singleton. Each entry carries the justification;
// new entries require reviewer sign-off.
var globalAccessWhitelist = map[string]string{
	// Client() is the sanctioned kvrocks resolver for the chunkcache.Store
	// seam: it wraps kvrocks.Get() with typed-nil protection so callers can
	// pass the result into Store parameters. Components reach it through the
	// canvas runtime, which does not carry a kvrocks handle.
	"internal/ingestion/chunkcache/chunkcache.go": "Client() is the sanctioned kvrocks resolver for the chunkcache.Store seam",
	// MarkWikiDocumentDirty is a package-level API consumed by out-of-scope
	// service packages (internal/service/document, internal/service/chunk)
	// that run in the API server process, where Provision has not installed
	// kcDB. The explicit kcDB (installed by Provision with an injected db) is
	// preferred; dao.DB is the API-process fallback.
	"internal/ingestion/knowledge_compile/wiki_dirty.go": "package-level API for out-of-scope callers; kcDB is the explicit Provision-installed dependency",
	// testutil installs in-memory test doubles by swapping the process-wide
	// singletons (ReplaceDBForTest). It is the sanctioned test seam, the only
	// place besides the server assembly points that may touch the globals.
	"internal/ingestion/testutil/helpers.go": "test-seam installer: swaps the process-wide singletons for in-memory doubles",
}

// scanRoots are the directory trees this guard covers: the ingestion
// orchestration layers plus the Go memory service files. The wider
// internal/service package is deliberately out of scope for this guard; it
// converges separately.
var scanRoots = []string{
	"internal/ingestion",
	"internal/service",
}

// memoryFilePrefix scopes the internal/service scan to the memory module
// files only.
//
// Blind spots — read before trusting a green run:
//   - internal/service is only scanned for files whose base name starts with
//     "memory". Production files that feed the memory (or other DI-converged)
//     modules but are not memory*-named — e.g. task_publisher.go (the memory
//     module's MQ publisher) or document/document.go (which injects the
//     knowledge_compile publisher) — are invisible to this guard and may still
//     read process-wide singletons. Their global reads are only safe because a
//     composition root reads the singleton to inject it; the guard cannot see
//     that intent, so reviewers must check such sites manually.
//   - Detection is AST-based and only matches literal pkg.Symbol selectors.
//     A helper that wraps a singleton read (e.g. chunkcache.Client() wrapping
//     kvrocks.Get()) is invisible even within a scanned file; it relies on the
//     globalAccessWhitelist to be declared as an explicit, approved access.
const memoryFilePrefix = "memory"

// findModuleRoot walks up from the working directory until it finds go.mod.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// importedName maps an import spec to the identifier the file uses for it,
// honouring explicit aliases (import kv "ragflow/internal/engine/kvrocks").
// A blank or dot import cannot produce pkg.Symbol selectors and is skipped.
func importedName(spec *ast.ImportSpec, fallback string) (string, bool) {
	if spec.Name != nil {
		switch spec.Name.Name {
		case "_", ".":
			return "", false
		}
		return spec.Name.Name, true
	}
	return fallback, true
}

// fileForbiddenHits returns the forbidden global accesses in one parsed file.
// Detection is AST-based: only real selector expressions count, so comments
// and string literals that merely mention e.g. "engine.Get()" are ignored.
func fileForbiddenHits(fset *token.FileSet, file *ast.File) []string {
	// Map each locally-used package identifier to the forbidden package it
	// belongs to. Keying by the local name (alias or default) keeps aliased
	// imports detectable while unrelated identifiers named "db" or "engine"
	// in files that do not import the package are ignored.
	imported := map[string]forbiddenPackage{}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, fp := range forbiddenPackages {
			if path != fp.importPath {
				continue
			}
			name, ok := importedName(imp, fp.pkgName)
			if !ok {
				continue
			}
			imported[name] = fp
		}
	}
	if len(imported) == 0 {
		return nil
	}

	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		fp, ok := imported[ident.Name]
		if !ok {
			return true
		}
		for _, sym := range fp.symbols {
			switch {
			case strings.HasSuffix(sym, "*"):
				// A trailing "*" matches every selector beginning with the
				// prefix, so "engine.Get*" forbids engine.Get / GetMessageQueueEngine /
				// GetEngineType — any process-wide engine singleton accessor.
				if strings.HasPrefix(sel.Sel.Name, strings.TrimSuffix(sym, "*")) {
					hits = append(hits, fp.pkgName+"."+sel.Sel.Name)
				}
			case sel.Sel.Name == sym:
				hits = append(hits, fp.pkgName+"."+sym)
			}
		}
		return true
	})
	return hits
}

// parseSource detects forbidden accesses in an in-memory source snippet. It
// backs the detector self-tests.
func parseSource(t *testing.T, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "snippet.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse snippet: %v", err)
	}
	return fileForbiddenHits(fset, file)
}

func TestForbiddenGlobalDetectorIgnoresCommentsAndStrings(t *testing.T) {
	src := `package snippet

import (
	"ragflow/internal/dao"
	"ragflow/internal/engine"
)

// A comment mentioning dao.DB and engine.Get() must not count.
const note = "dao.DB and kvrocks.Get() in a string literal must not count either"

func f() {
	_ = note
	_ = dao.NewDocumentDAO
	_ = engine.EngineInfinity
}
`
	if hits := parseSource(t, src); len(hits) != 0 {
		t.Fatalf("comments/strings/unrelated selectors flagged as violations: %v", hits)
	}
}

func TestForbiddenGlobalDetectorFlagsRealAccesses(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "dao.DB passed as an argument",
			src: `package snippet

import "ragflow/internal/dao"

func f() { _ = dao.NewTaskDAO().GetByID(nil, dao.DB, "id") }
`,
			want: "dao.DB",
		},
		{
			name: "engine.Get assigned",
			src: `package snippet

import "ragflow/internal/engine"

func f() { eng := engine.Get(); _ = eng }
`,
			want: "engine.Get",
		},
		{
			name: "kvrocks.Get nil check",
			src: `package snippet

import "ragflow/internal/engine/kvrocks"

func f() { if kvrocks.Get() != nil { _ = 1 } }
`,
			want: "kvrocks.Get",
		},
		{
			name: "aliased import still detected",
			src: `package snippet

import eng "ragflow/internal/engine"

func f() { _ = eng.GetMessageQueueEngine() }
`,
			want: "engine.GetMessageQueueEngine",
		},
		{
			name: "engine.Get* prefix catches GetEngineType",
			src: `package snippet

import "ragflow/internal/engine"

func f() { _ = engine.GetEngineType() }
`,
			want: "engine.GetEngineType",
		},
		{
			name: "unrelated identifier named engine is not a violation",
			src: `package snippet

type engine struct{ Get int }

func f() { _ = engine{}.Get }
`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := parseSource(t, tc.src)
			if tc.want == "" {
				if len(hits) != 0 {
					t.Fatalf("unexpected violations: %v", hits)
				}
				return
			}
			if len(hits) != 1 || hits[0] != tc.want {
				t.Fatalf("hits = %v, want exactly [%s]", hits, tc.want)
			}
		})
	}
}

func TestForbiddenGlobalDetectorSkipsTestFilesAndWhitelist(t *testing.T) {
	// The detector is fed by the walker, so verify the walker's own rules:
	// _test.go files are skipped, whitelisted files are skipped, and files
	// under internal/service that are not memory files are skipped.
	root := findModuleRoot(t)

	scanned, skipped := 0, 0
	for _, scanRoot := range scanRoots {
		err := filepath.Walk(filepath.Join(root, scanRoot), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !strings.HasSuffix(rel, ".go") {
				return nil
			}
			switch {
			case strings.HasSuffix(rel, "_test.go"):
				skipped++
				return nil
			case globalAccessWhitelist[rel] != "":
				skipped++
				return nil
			case scanRoot == "internal/service" && !strings.HasPrefix(filepath.Base(rel), memoryFilePrefix):
				skipped++
				return nil
			}
			scanned++
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", scanRoot, err)
		}
	}
	if scanned == 0 {
		t.Fatal("walker selected no files to scan; the guard would pass vacuously")
	}
	if skipped == 0 {
		t.Fatal("walker skipped nothing; test-file/whitelist rules are not exercised")
	}
}

// TestProductionCodeDoesNotAccessGlobalSingletons is the architecture guard:
// ingestion orchestration layers and the memory module must resolve their
// dependencies through the injected ingestion.Deps bundle (and the memory
// service's constructor parameters), never through process-wide singletons.
// The singletons are captured once at the server assembly points.
func TestProductionCodeDoesNotAccessGlobalSingletons(t *testing.T) {
	root := findModuleRoot(t)

	var violations []string
	scanned := 0
	for _, scanRoot := range scanRoots {
		err := filepath.Walk(filepath.Join(root, scanRoot), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
				return nil
			}
			if reason := globalAccessWhitelist[rel]; reason != "" {
				return nil
			}
			if scanRoot == "internal/service" && !strings.HasPrefix(filepath.Base(rel), memoryFilePrefix) {
				return nil
			}
			scanned++

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			for _, hit := range fileForbiddenHits(fset, file) {
				violations = append(violations, rel+": "+hit)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", scanRoot, err)
		}
	}

	if scanned == 0 {
		t.Fatal("guard scanned no files; the guard would pass vacuously")
	}
	if len(violations) > 0 {
		t.Fatalf("%d file(s) in the ingestion/memory orchestration layers still resolve process-wide singletons directly (%d files scanned):\n%s\n\n"+
			"Resolve these through the injected ingestion.Deps bundle at the server assembly points instead.",
			len(violations), scanned, strings.Join(violations, "\n"))
	}
}
