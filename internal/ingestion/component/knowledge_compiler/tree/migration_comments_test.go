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
	"strings"
	"testing"
)

// pythonRef matches any mention of "python" in a comment. The Python codebase
// is being removed (Go-only target), so a surviving "python" reference in a
// comment is, by definition, a stale migration anchor. A genuine WHY (invariant,
// payload shape, algorithm) must be rephrased to describe the Go behavior and
// must not contain the word.
var pythonRef = regexp.MustCompile(`(?i)python`)

// TestNoMigrationHistoryComments fails while any non-test .go file under the
// tree sub-package still references Python in its comments.
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
				if pythonRef.MatchString(c.Text) {
					pos := fset.Position(c.Pos())
					hits = append(hits, filepath.ToSlash(path)+":"+itoa(pos.Line)+": "+strings.TrimSpace(c.Text))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}
	if len(hits) > 0 {
		t.Errorf("found %d Python references in tree comments; rephrase to describe the Go behavior (see AGENTS.md):\n%s",
			len(hits), strings.Join(hits, "\n"))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
