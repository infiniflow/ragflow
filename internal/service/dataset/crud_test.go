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
