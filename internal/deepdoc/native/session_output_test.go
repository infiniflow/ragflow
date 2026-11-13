//go:build cgo

package native

import "testing"

// TestCheckOutputLength guards the output-length validation that the dynamic
// (per-Run allocated) session relies on. The previous pinned-output design got
// this check for free: ORT errored when the bound output tensor's shape
// mismatched the model. After switching to a nil output that ORT allocates, the
// shape is no longer checked by ORT, so postprocessing (dlaPostprocess indexes
// 300*6, tsrPostprocess indexes 11*8400, RunDet fills rh*rw) would panic or
// silently misread a truncated output. This test locks the expected lengths.
func TestCheckOutputLength(t *testing.T) {
	cases := []struct {
		model string
		got   int
		want  int
		ok    bool
	}{
		{"dla", dlaMaxBoxes * 6, dlaMaxBoxes * 6, true},
		{"dla", dlaMaxBoxes*6 - 1, dlaMaxBoxes * 6, false},
		{"dla", 0, dlaMaxBoxes * 6, false},
		{"tsr", 11 * tsrCandidates, 11 * tsrCandidates, true},
		{"tsr", 11*tsrCandidates - 7, 11 * tsrCandidates, false},
		{"det", 100, 100, true},
		{"det", 99, 100, false},
	}
	for _, c := range cases {
		err := checkOutputLength(c.model, c.got, c.want)
		if c.ok && err != nil {
			t.Errorf("checkOutputLength(%q,%d,%d) = %v, want nil", c.model, c.got, c.want, err)
		}
		if !c.ok && err == nil {
			t.Errorf("checkOutputLength(%q,%d,%d) = nil, want error", c.model, c.got, c.want)
		}
	}
}
