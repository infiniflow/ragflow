package dataset

import (
	"testing"

	"ragflow/internal/entity"
)

func TestExtractUniqueFileIDs(t *testing.T) {
	f1 := "f1"
	f2 := "f2"
	empty := ""

	tests := []struct {
		name     string
		mappings []entity.File2Document
		want     []string
	}{
		{
			name:     "empty",
			mappings: nil,
			want:     nil,
		},
		{
			name: "single file",
			mappings: []entity.File2Document{
				{FileID: &f1},
			},
			want: []string{"f1"},
		},
		{
			name: "deduplicates duplicate file IDs",
			mappings: []entity.File2Document{
				{FileID: &f1},
				{FileID: &f1},
				{FileID: &f2},
			},
			want: []string{"f1", "f2"},
		},
		{
			name: "skips nil FileID",
			mappings: []entity.File2Document{
				{FileID: nil},
				{FileID: &f1},
			},
			want: []string{"f1"},
		},
		{
			name: "skips empty FileID",
			mappings: []entity.File2Document{
				{FileID: &empty},
				{FileID: &f1},
			},
			want: []string{"f1"},
		},
		{
			name: "all nil or empty returns empty",
			mappings: []entity.File2Document{
				{FileID: nil},
				{FileID: &empty},
			},
			want: nil,
		},
		{
			// Adversarial: duplicates interleaved across the slice must be
			// collapsed while preserving the order of first occurrence, not
			// sorted or last-wins.
			name: "preserves first-occurrence order on interleaved duplicates",
			mappings: []entity.File2Document{
				{FileID: &f2},
				{FileID: &f1},
				{FileID: &f2},
				{FileID: &f1},
			},
			want: []string{"f2", "f1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUniqueFileIDs(tt.mappings)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got %v, want %v)", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestExtractUniqueFileIDs_AllSkippedNonNil guards the contract that an all-skip
// (or nil) input yields a non-nil empty slice, not nil. Downstream callers key
// off len(), but the non-nil guarantee documents intent and prevents a future
// regression to returning nil when the dedup path is delegated to a canonical
// helper that may return nil for a nil argument.
func TestExtractUniqueFileIDs_AllSkippedNonNil(t *testing.T) {
	empty := ""
	cases := [][]entity.File2Document{
		nil,
		{{FileID: nil}},
		{{FileID: &empty}},
	}
	for i, m := range cases {
		got := extractUniqueFileIDs(m)
		if got == nil {
			t.Errorf("case %d: extractUniqueFileIDs returned nil, want non-nil empty slice", i)
		}
	}
}

func TestExtractDocIDs(t *testing.T) {
	docs := []entity.Document{
		{ID: "d1"},
		{ID: "d2"},
		{ID: "d3"},
	}
	got := extractDocIDs(docs)
	want := []string{"d1", "d2", "d3"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if got := extractDocIDs(nil); len(got) != 0 {
		t.Fatalf("nil input: got %v, want empty", got)
	}
}
