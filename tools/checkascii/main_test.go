package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		before  string
		after   string
		removed bool
		lines   []string
	}{
		{name: "ASCII edit beside existing Unicode", file: "code.go", before: "// \u4e2d\u6587\n// old\n", after: "// \u4e2d\u6587\n// new\n"},
		{name: "removed Unicode line", file: "code.go", before: "// \u4e2d\u6587\n// ASCII\n", after: "// ASCII\n"},
		{name: "removed file", file: "removed.go", before: "// \u4e2d\u6587\n", removed: true},
		{name: "ASCII string escape", file: "code.go", after: `var text = "\u4e2d\u6587"`},
		{name: "new code", file: "code.go", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "multiple hunks", file: "code.go", before: "one\ntwo\nthree\nfour\nfive\n", after: "one\n\u00e9\nthree\nfour\n\u2014\n", lines: []string{"2", "5"}},
		{name: "no final newline", file: "code.go", after: "// \u4e2d\u6587", lines: []string{"1"}},
		{name: "content resembles diff header", file: "code.go", after: "++ \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "long line", file: "long.txt", after: strings.Repeat("a", 128*1024) + "\u00e9\n", lines: []string{"1"}},
		{name: "binary file", file: "image.bin", after: "\x00\xff\xfe\n"},
		{name: "translated locale", file: "web/src/locales/zh.ts", after: "// \u4e2d\u6587\n"},
		{name: "English locale", file: "web/src/locales/en.ts", after: "// \u2019\n", lines: []string{"1"}},
		{name: "nested locale code", file: "web/src/locales/nested/helper.ts", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "translated root README", file: "README_zh.md", after: "\u4e2d\u6587\n"},
		{name: "English README", file: "README.md", after: "\u2014\n", lines: []string{"1"}},
		{name: "nested translated README", file: "docs/README_zh.md", after: "\u4e2d\u6587\n", lines: []string{"1"}},
		{name: "localized agent template", file: "internal/agent/templates/example.json", after: "{\"zh\": \"\u4e2d\u6587\"}\n"},
		{name: "entity dictionary", file: "internal/rag/res/ner.json", after: "{\"\u4e2d\u6587\": \"entity\"}\n"},
		{name: "synonym dictionary", file: "internal/rag/res/synonym.json", after: "{\"\u4e2d\u6587\": \"synonym\"}\n"},
		{name: "other resource", file: "internal/rag/res/config.json", after: "{\"value\": \"\u4e2d\u6587\"}\n", lines: []string{"1"}},
		{name: "parser input", file: "internal/parser/testdata/sample.zh.html", after: "<p>\u4e2d\u6587</p>\n"},
		{name: "fixture data", file: "test/fixtures/example.json", after: "{\"value\": \"\u4e2d\u6587\"}\n"},
		{name: "golden data", file: "internal/example/golden/result.txt", after: "\u4e2d\u6587\n"},
		{name: "Go test code", file: "internal/example/example_test.go", after: "// \u4e2d\u6587\n"},
		{name: "Go test in fixture directory", file: "internal/parser/testdata/example_test.go", after: "// \u4e2d\u6587\n"},
		{name: "frontend TypeScript test", file: "web/src/utils/example.test.ts", after: "// \u4e2d\u6587\n"},
		{name: "frontend React test", file: "web/src/components/example.test.tsx", after: "// \u4e2d\u6587\n"},
		{name: "frontend JavaScript test", file: "web/src/utils/example.test.js", after: "// \u4e2d\u6587\n"},
		{name: "frontend JSX spec", file: "web/src/components/example.spec.jsx", after: "// \u4e2d\u6587\n"},
		{name: "frontend module spec", file: "web/src/utils/example.spec.mjs", after: "// \u4e2d\u6587\n"},
		{name: "frontend grouped test", file: "web/src/utils/__tests__/example.ts", after: "// \u4e2d\u6587\n"},
		{name: "frontend mock", file: "web/__mocks__/example.js", after: "// \u4e2d\u6587\n"},
		{name: "frontend test setup", file: "web/jest-setup.ts", after: "// \u4e2d\u6587\n"},
		{name: "frontend application code", file: "web/src/utils/example.ts", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "frontend application test helper", file: "web/src/utils/test-helper.ts", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "test-like directory", file: "web/src/utils/__tests__extra/example.ts", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "test-like filename", file: "web/src/utils/example.test.ts.backup", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "fixture generator", file: "internal/parser/testdata/generate.go", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "filename with spaces and quotes", file: "code with \"quotes\".go", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "literal pathspec", file: ":(exclude)file.go", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
		{name: "annotation escaping", file: "code%,\n.go", after: "// \u4e2d\u6587\n", lines: []string{"1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := initRepo(t)
			if tt.before != "" {
				writeFile(t, dir, tt.file, tt.before)
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "--allow-empty", "-qm", "base")
			base := git(t, dir, "rev-parse", "HEAD")
			if tt.removed {
				if err := os.Remove(filepath.Join(dir, tt.file)); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFile(t, dir, tt.file, tt.after)
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-qm", "PR")
			var out bytes.Buffer
			err := check(t.Context(), dir, base, "HEAD", &out)
			if (err != nil) != (len(tt.lines) > 0) {
				t.Fatalf("check returned %v, output: %s", err, &out)
			}
			if got := strings.Count(out.String(), "::error file="); got != len(tt.lines) {
				t.Fatalf("got %d annotations, want %d: %s", got, len(tt.lines), &out)
			}
			for _, line := range tt.lines {
				if !strings.Contains(out.String(), ",line="+line+"::") {
					t.Errorf("missing annotation at line %s: %s", line, &out)
				}
			}
			if tt.name == "annotation escaping" && !strings.Contains(out.String(), "file=code%25%2C%0A.go,line=1::") {
				t.Errorf("unescaped annotation path: %s", &out)
			}
		})
	}
}

func TestCheckUsesMergeBaseAndCommittedHead(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "code.go", "// ASCII\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "common ancestor")
	ancestor := git(t, dir, "rev-parse", "HEAD")
	writeFile(t, dir, "upstream.go", "// \u4e2d\u6587\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "target branch change")
	base := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-qb", "pr", ancestor)
	writeFile(t, dir, "code.go", "// changed ASCII\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "PR change")
	head := git(t, dir, "rev-parse", "HEAD")
	writeFile(t, dir, "code.go", "// uncommitted \u4e2d\u6587\n")
	var out bytes.Buffer
	if err := check(t.Context(), dir, base, head, &out); err != nil {
		t.Fatalf("check included changes outside the committed PR diff: %v: %s", err, &out)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "PR Unicode change")
	head = git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "--detach", base)
	if err := check(t.Context(), dir, base, head, &out); err == nil {
		t.Fatal("check ignored committed PR content absent from the working tree")
	}
}

func TestCheckRenameOutOfExclusion(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "web/src/locales/zh.ts", "// \u4e2d\u6587\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "mv", "web/src/locales/zh.ts", "web/src/locales/en.ts")
	git(t, dir, "commit", "-qm", "rename")
	var out bytes.Buffer
	if err := check(t.Context(), dir, base, "HEAD", &out); err == nil {
		t.Fatal("check allowed Unicode to be renamed into en.ts")
	}
	if !strings.Contains(out.String(), "file=web/src/locales/en.ts,line=1::") {
		t.Fatalf("missing rename annotation: %s", &out)
	}
}

func TestCheckInvalidRefs(t *testing.T) {
	dir := initRepo(t)
	git(t, dir, "commit", "--allow-empty", "-qm", "base")
	for _, refs := range [][2]string{{"missing-base", "HEAD"}, {"HEAD", "missing-head"}} {
		var out bytes.Buffer
		if err := check(t.Context(), dir, refs[0], refs[1], &out); err == nil {
			t.Errorf("check accepted invalid refs %v", refs)
		}
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.name", "ASCII check test")
	git(t, dir, "config", "user.email", "ascii-test@example.com")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"--literal-pathspecs", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir, file, text string) {
	t.Helper()
	fullPath := filepath.Join(dir, file)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
