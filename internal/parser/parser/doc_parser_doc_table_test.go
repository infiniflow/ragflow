//go:build cgo

package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDOCParser_RecoversTableFromLegacyDoc verifies that the legacy .doc path
// recovers table structure through office_oxide's structured IR. The fixture
// (testdata/table.doc) is a real Word binary containing a multi-column table.
//
// The " | " separator is emitted ONLY by flattenDocIR when office_oxide's
// ToIRJSON returns a table element (cells joined with " | " per row). Before
// office_oxide/go v0.1.9 (#116) the .doc reader ignored the table stream, so
// no table element was produced and the separator never appeared — this test
// is therefore a direct regression gate for the v0.1.9 dependency bump.
func TestDOCParser_RecoversTableFromLegacyDoc(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "table.doc"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	res := NewDOCParser().ParseWithResult(context.Background(), "table.doc", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	if strings.TrimSpace(res.Text) == "" {
		t.Fatalf("parsed text is empty")
	}

	// Table cells must be separated by " | " — the signature of IR-driven
	// table recovery. PlainText / ToMarkdown never emit this separator.
	if !strings.Contains(res.Text, " | ") {
		t.Errorf("table structure not recovered from legacy .doc\n got=%q", res.Text)
	}
}
