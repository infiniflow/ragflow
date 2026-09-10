package pdf

import "testing"

// TestParityLockedGridExcludesUnfixed locks that known-unfixed content gaps are
// NOT in the locked grid-parity set, and that every PDF at full grid+structure
// parity (06/14/18) IS locked. Locking a PDF whose gridSim is below 100%
// (e.g. table_rotation_test, INTENTIONAL structSim=35.7%) makes the harness
// simultaneously accept and forbid the same gap; only PDFs at gridSim=100% AND
// structureSim=100% (the remaining divergence is non-cell-text) belong in the
// lock. 14/18 were added once their grid+structure parity was reached (cell-fill
// and caption fixes); keep this in lock-step with parity_lock.go.
func TestParityLockedGridExcludesUnfixed(t *testing.T) {
	locked := parityLockedGridPDFs()

	// Known-unfixed structural gap must not be locked.
	if locked["table_rotation_test.pdf"] {
		t.Error("table_rotation_test has structSim<100 (go_intentional segmentation) and must not be locked")
	}

	// 13_crosspage is no longer locked: the production R/C-assembly path
	// renders its first row at 3 columns vs Python's 5 (structSim 98.8%, a
	// documented follow-up), so locking it would forbid a known gap.
	if locked["13_crosspage_table.pdf"] {
		t.Error("13_crosspage_table has structSim<100 under the production R/C-assembly path and must not be locked")
	}

	// PDFs that reach grid+structure parity (only non-cell-text divergence
	// left) stay locked.
	for _, name := range []string{
		"06_table_content.pdf",
		"14_text_table_interleaved.pdf",
		"18_table_caption.pdf",
	} {
		if !locked[name] {
			t.Errorf("%s reaches grid+structure parity and must stay locked", name)
		}
	}
}
