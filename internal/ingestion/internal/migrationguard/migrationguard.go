// Package migrationguard provides a static hygiene check that flags comments
// still anchoring the Go implementation on the deleted earlier codebase.
//
// The ingestion tree was ported from an implementation that is being removed
// (Go-only target). Any surviving reference — the word "python", a .py file
// name, or a rag.(flow|nlp|app|deepdoc) module path — is, by definition, a
// stale migration anchor. A genuine WHY (invariant, payload shape, algorithm)
// must be rephrased to describe the Go behavior and must not contain any of
// these.
package migrationguard

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// migrationRef matches comments that still anchor the Go implementation on the
// deleted earlier codebase. It is intentionally high-precision: it only flags
// the unambiguous migration tell-tales:
//   - the word "python", a .py file,
//   - the rag.(flow|nlp|app|deepdoc) dotted module path,
//   - the rag/(flow|nlp|app|deepdoc) slashed module path,
//   - a stray smart-quote “ left by an automated rephrase.
//
// It deliberately does NOT flag ordinary English ("the chunker's crop",
// "data:image/png"), Go code snippets in comments ("matrix[:headerRows]"), Go
// file::func cross-references ("raptor.go::watershed"), or Go-to-Go "mirrors"
// references, which would produce false positives. Those residual references
// are removed by hand during the comment rephrase rather than by this
// heuristic guard.
var migrationRef = regexp.MustCompile(`(?i)(\bpython\b|\.py\b|legacy implementation|other runtime|two runtimes|rag\.(flow|nlp|app|deepdoc)\.[a-zA-Z_]+|rag/(flow|nlp|app|deepdoc)|“)`)

// scan returns every comment line under dir that still anchors on the deleted
// earlier codebase.
func scan(dir string) ([]string, error) {
	fset := token.NewFileSet()
	var hits []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		file, perr := parser.ParseFile(fset, path, data, parser.ParseComments)
		if perr != nil {
			return perr
		}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if migrationRef.MatchString(c.Text) {
					pos := fset.Position(c.Pos())
					hits = append(hits, filepath.ToSlash(path)+":"+strconv.Itoa(pos.Line)+": "+strings.TrimSpace(c.Text))
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// Check fails the test if any non-test .go file under dir still anchors a
// comment on the deleted earlier codebase.
func Check(t *testing.T, dir string) {
	t.Helper()
	hits, err := scan(dir)
	if err != nil {
		t.Fatalf("migrationguard walk failed: %v", err)
	}
	if len(hits) > 0 {
		t.Errorf("found %d migration-anchor references in comments; rephrase to describe the Go behavior (see AGENTS.md):\n%s",
			len(hits), strings.Join(hits, "\n"))
	}
}
