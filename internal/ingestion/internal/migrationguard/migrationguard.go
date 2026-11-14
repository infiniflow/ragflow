// Package migrationguard provides a build-time hygiene check that flags
// comments still anchoring the Go implementation on the removed earlier
// codebase. It matches unambiguous migration tell-tales — a source file name
// from that codebase, a dotted module path under the old rag tree, or a stray
// smart-quote — so genuine WHY comments stay and stale anchors are caught in
// CI.
//
// The guard is intentionally high-precision: ordinary English, Go code
// snippets, Go file::func cross-references, and Go-to-Go "mirrors" references
// are left alone; those residual references are rephrased by hand rather than
// by the heuristic.
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

// emptyRef matches a degenerate parenthesis group left behind by a half-finished
// rephrase: a "(" with no content before its matching ")" (optionally with a
// comma) is a placeholder the rephrase should have filled. It anchors
// the opening "(" so a "(" that is part of a Go symbol reference such as
// "splitSentencesLossless()" (the "(" is preceded by a word character) is NOT
// flagged — only a "(" that follows whitespace or line start is, which is how a
// true empty/placeholder group reads in a comment.
//
// This catches the bare-empty-parenthesis / empty-reference class of syntactic danglers
// that migrationRef (a high-precision keyword match) cannot see. It deliberately
// does NOT flag an unbalanced "(" that continues onto the next line (e.g.
// "the correction call (") or a ")" that closes a multi-line parenthetical on
// its own line: those are legitimate Go-doc prose and would produce false
// positives. Those residual references are removed by hand during the rephrase.
var emptyRef = regexp.MustCompile(`(^|[^[:alnum:]_])\(\s*(?:,\s*)?\)`)

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
				if migrationRef.MatchString(c.Text) || emptyRef.MatchString(c.Text) {
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
		t.Errorf("found %d migration-anchor / empty-parenthesis references in comments; rephrase to describe the Go behavior (see AGENTS.md):\n%s",
			len(hits), strings.Join(hits, "\n"))
	}
}
