package common

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// This is an output-invariant refactor: the shared helpers in this package
// (VectorColumnName / IsVectorColumn / VectorColumnDim) produce bytes identical
// to the inline spellings they replace, so a behavioral test cannot tell the
// old inline code from the helper. The migration's only real risk is leaving a
// call site behind on the old spelling — and that risk is exactly what the
// original PR's scope ("every call site") commits to eliminating. We therefore
// assert structurally, across the whole `internal` tree, that no production
// file still constructs or mentions the q_<dim>_vec vocabulary inline.
//
// The check is deliberately spelling-agnostic: it forbids any `"q_" + … + "_vec"`
// concatenation and any `Sprintf("q_%d_vec", …)` outside the canonical helper,
// so a variant written with a different variable name, `strconv.Itoa` instead of
// `fmt.Sprintf`, or different whitespace still trips the guard.
func TestNoInlineVectorColumnVocabulary(t *testing.T) {
	root := repoRoot(t)

	concatRe := regexp.MustCompile(`"q_"\s*\+\s*[^;{}]*?\+\s*"_vec"`)
	sprintfRe := regexp.MustCompile(`Sprintf\(\s*"q_%d_vec"`)

	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip vendored / generated / non-Go trees.
			if d.Name() == "vendor" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// The canonical helper is the single allowed definition site.
		if strings.HasSuffix(path, "/common/vector_column.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(data)
		if loc := concatRe.FindString(s); loc != "" {
			t.Errorf("%s: inline q_<dim>_vec concatenation still present: %q", rel(root, path), loc)
		}
		if loc := sprintfRe.FindString(s); loc != "" {
			t.Errorf("%s: inline Sprintf(\"q_%%d_vec\") still present: %q", rel(root, path), loc)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}

// rel returns path relative to root for readable failure messages.
func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return r
}

// repoRoot walks up from this test file until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
