package migrationguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGo(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScanCatchesMigrationAnchors verifies the guard detects the anchored
// comment forms and ignores comments that describe the current Go behavior.
func TestScanCatchesMigrationAnchors(t *testing.T) {
	dir := t.TempDir()

	// Anchored forms that must be caught.
	writeGo(t, dir, "python_word.go", "// mirrors the python implementation\npackage x\n")
	writeGo(t, dir, "py_file.go", "// see tokenizer.py for details\npackage x\n")
	writeGo(t, dir, "ragslash.go", "// mirrors rag/nlp.not_bullet behavior\npackage x\n")
	writeGo(t, dir, "smartquote.go", "// port of “\npackage x\n")

	// Comments that describe Go behavior and must be ignored.
	writeGo(t, dir, "good_invariants.go", "// bulletsCategory buckets lines by bullet shape\npackage x\n")
	writeGo(t, dir, "good_mirrors.go", "// chunkerOutputs mirrors schema.ExtractorOutputs\npackage x\n")
	writeGo(t, dir, "good_possessive.go", "// the chunker's sliding-window crop keeps boxes stable\npackage x\n")
	writeGo(t, dir, "good_colon.go", "// the payload uses data:image/png and a key: value pair\npackage x\n")

	hits, err := scan(dir)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	got := strings.Join(hits, "\n")
	want := []string{
		"python_word.go",
		"py_file.go",
		"ragslash.go",
		"smartquote.go",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("expected anchor in %s to be caught, but it was not.\nGot:\n%s", w, got)
		}
	}
	if strings.Contains(got, "good_invariants.go") {
		t.Errorf("false positive: Go-behavior comment was flagged.\nGot:\n%s", got)
	}
	if strings.Contains(got, "good_mirrors.go") {
		t.Errorf("false positive: Go-to-Go mirrors comment was flagged.\nGot:\n%s", got)
	}
	if strings.Contains(got, "good_possessive.go") {
		t.Errorf("false positive: English possessive was flagged.\nGot:\n%s", got)
	}
	if strings.Contains(got, "good_colon.go") {
		t.Errorf("false positive: data:image / key: value was flagged.\nGot:\n%s", got)
	}
}
