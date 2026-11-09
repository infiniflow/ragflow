// Copyright 2026. Apache 2.0 License.

// Package tree_test hosts static hygiene checks for the tree sub-package. These
// are not behavioral tests: they guard against comments that anchor the Go
// implementation on the deleted Python codebase (see AGENTS.md — comments must
// describe the current runtime, not migration history).
package tree_test

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
// deleted Python codebase. The Python code is being removed (Go-only target),
// so any surviving reference — the word "python", a .py file name, or a
// rephrased anchor such as "legacy implementation" / "other runtime" / "two
// runtimes" — is, by definition, a stale migration anchor. A genuine WHY
// (invariant, payload shape, algorithm) must be rephrased to describe the Go
// behavior and must not contain any of these.
var migrationRef = regexp.MustCompile(`(?i)(python|\.py\b|legacy implementation|other runtime|two runtimes)`)

// TestNoMigrationHistoryComments fails while any non-test .go file under the
// tree sub-package still anchors a comment on the deleted Python codebase (the
// word "python", a .py file, or a rephrased legacy/runtime anchor).
func TestNoMigrationHistoryComments(t *testing.T) {
	fset := token.NewFileSet()
	var hits []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
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
		t.Fatalf("walk failed: %v", err)
	}
	if len(hits) > 0 {
		t.Errorf("found %d migration-anchor references in tree comments; rephrase to describe the Go behavior (see AGENTS.md):\n%s",
			len(hits), strings.Join(hits, "\n"))
	}
}
