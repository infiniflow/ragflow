package pdf

import "testing"

// TestParityLockedGridExcludesUnfixed locks that known-unfixed content gaps are
// NOT in the locked grid-parity set. Locking a PDF whose gridSim is below 100%
// (13_crosspage_table open gap, 14_text_table_interleaved accepted_at_merge)
// makes the harness simultaneously accept and forbid the same gap. Only PDFs at
// gridSim=100% (HTML-format-only divergence: 06, 18) belong in the lock.
func TestParityLockedGridExcludesUnfixed(t *testing.T) {
	locked := parityLockedGridPDFs()

	// Known-unfixed content gaps must not be locked.
	if locked["13_crosspage_table.pdf"] {
		t.Error("13_crosspage_table is an open grid gap and must not be locked")
	}
	if locked["14_text_table_interleaved.pdf"] {
		t.Error("14_text_table_interleaved is accepted_at_merge and must not be locked")
	}

	// PDFs that reach gridSim=100% (HTML-format-only divergence) stay locked.
	if !locked["06_table_content.pdf"] {
		t.Error("06_table_content reaches grid parity and must stay locked")
	}
	if !locked["18_table_caption.pdf"] {
		t.Error("18_table_caption reaches grid parity and must stay locked")
	}
}
