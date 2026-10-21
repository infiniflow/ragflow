package document

import "testing"

// TestIsPresentationFile pins the name-based presentation guard used by the
// parser_id / chunk_method switch in document_dataset_update.go. A WPS
// presentation (dps/dpt) must be treated as a presentation on a par with ppt/
// pptx/pages, so the API allows switching its chunk_method to "presentation".
func TestIsPresentationFile(t *testing.T) {
	p := func(s string) *string { return &s }

	cases := []struct {
		name string
		in   *string
		want bool
	}{
		{"nil name is not presentation", nil, false},
		{"ppt is presentation", p("deck.ppt"), true},
		{"pptx is presentation", p("deck.pptx"), true},
		{"pages is presentation", p("deck.pages"), true},
		{"wps presentation dps is presentation", p("deck.dps"), true},
		{"wps presentation dpt is presentation", p("deck.dpt"), true},
		{"wps presentation dps is case-insensitive", p("DECK.DPS"), true},
		{"pdf is not presentation", p("report.pdf"), false},
		{"wps doc is not presentation", p("note.wps"), false},
		{"wps sheet et is not presentation", p("sheet.et"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPresentationFile(tc.in); got != tc.want {
				t.Fatalf("isPresentationFile(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
