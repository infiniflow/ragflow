package pdf

// parityLockedGridPDFs returns the set of table PDFs that MUST hold full grid
// (cell-content AND structure) parity with Python. The pipeline-parity harness
// reads this to error if a locked PDF's gridSim OR structureSim drops below
// 100% (both must hold — gridSim alone is shape-blind and would miss a
// segmentation divergence with identical cell text).
//
// Only PDFs that actually reach gridSim=100% AND structureSim=100% (the only
// divergence left being non-cell-text: caption/body/format outside table cells)
// belong here. Known-unfixed content/structure gaps (e.g. table_rotation_test)
// must NOT be locked, or the harness both accepts and forbids the same gap — a
// contradiction. Keep this set in lock-step with known_diffs.json: a PDF listed
// here must have gridSim=100% and structureSim=100% (or be exempted as
// go_intentional), never an open/accepted_at_merge failure.
//
// 13_crosspage_table.pdf is NOT locked: the production R/C-assembly path
// (AnnotateBoxesWithGrid + GroupBoxesByRC) renders its first row at 3 columns
// vs Python's 5 (a cross-page merged table whose first-page header boxes get
// re-assigned C indices after dedup/vertical-merge) — structSim 98.8%, a
// documented follow-up, so locking it would forbid a known gap.
func parityLockedGridPDFs() map[string]bool {
	return map[string]bool{
		"06_table_content.pdf":          true,
		"14_text_table_interleaved.pdf": true,
		"18_table_caption.pdf":          true,
		"1.pdf":                         true,
	}
}
