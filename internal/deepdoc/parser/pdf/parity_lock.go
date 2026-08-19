package pdf

// parityLockedGridPDFs returns the set of table PDFs that MUST hold full grid
// (cell-content) parity with Python. The pipeline-parity harness reads this to
// error if a locked PDF's gridSim drops below 100%.
//
// Only PDFs that actually reach gridSim=100% (HTML-format-only divergence)
// belong here. Known-unfixed content gaps (e.g. 13_crosspage_table,
// 14_text_table_interleaved) must NOT be locked, or the harness both accepts
// and forbids the same gap — a contradiction. Keep this set in lock-step with
// known_diffs.json: a PDF listed here must have gridSim=100% (or be exempted
// as go_intentional), never an open/accepted_at_merge failure.
func parityLockedGridPDFs() map[string]bool {
	return map[string]bool{
		"06_table_content.pdf": true,
		"18_table_caption.pdf": true,
	}
}
