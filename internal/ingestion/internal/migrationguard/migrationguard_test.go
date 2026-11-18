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
	writeGo(t, dir, "empty_paren.go", "// bulletsCategory mirrors bullets_category ()\npackage x\n")
	writeGo(t, dir, "empty_paren_comma.go", "// marker (, ) and persisted as the DSL\npackage x\n")

	// Comments that describe Go behavior and must be ignored.
	writeGo(t, dir, "good_invariants.go", "// bulletsCategory buckets lines by bullet shape\npackage x\n")
	writeGo(t, dir, "good_mirrors.go", "// chunkerOutputs mirrors schema.ExtractorOutputs\npackage x\n")
	writeGo(t, dir, "good_possessive.go", "// the chunker's sliding-window crop keeps boxes stable\npackage x\n")
	writeGo(t, dir, "good_colon.go", "// the payload uses data:image/png and a key: value pair\npackage x\n")
	writeGo(t, dir, "good_func_call.go", "// splitSentencesLossless() returns one chunk per sentence\npackage x\n")
	writeGo(t, dir, "good_nested_paren.go", "// the payload (a JSON list of chunks) is emitted as-is\npackage x\n")

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
		"empty_paren.go",
		"empty_paren_comma.go",
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
	if strings.Contains(got, "good_func_call.go") {
		t.Errorf("false positive: Go func-call reference was flagged.\nGot:\n%s", got)
	}
	if strings.Contains(got, "good_nested_paren.go") {
		t.Errorf("false positive: real parenthetical was flagged.\nGot:\n%s", got)
	}
}
