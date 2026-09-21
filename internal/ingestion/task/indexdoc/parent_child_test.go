package indexdoc

import "testing"

func TestParentChunkIDScopesDatasetAndDocument(t *testing.T) {
	if got, again := parentChunkID("kb-a", "doc-a", "same parent text"), parentChunkID("kb-a", "doc-a", "same parent text"); got != again {
		t.Fatalf("parentChunkID is not deterministic: %q != %q", got, again)
	}
	if got, otherDataset := parentChunkID("kb-a", "doc-a", "same parent text"), parentChunkID("kb-b", "doc-a", "same parent text"); got == otherDataset {
		t.Fatalf("parentChunkID collides across datasets: %q", got)
	}
}

func TestMaterializeParentChunksScopesSameParentTextToDocument(t *testing.T) {
	chunks := []map[string]any{
		{"id": "child-a", "doc_id": "doc-a", "mom": "shared parent"},
		{"id": "child-b", "doc_id": "doc-b", "mom": "shared parent"},
	}

	parents := MaterializeParentChunks("kb-a", chunks)

	if len(parents) != 2 {
		t.Fatalf("parent count = %d, want 2", len(parents))
	}
	if chunks[0]["mom_id"] == chunks[1]["mom_id"] {
		t.Fatalf("mom_id collision: %q", chunks[0]["mom_id"])
	}
	if parents[0]["doc_id"] == parents[1]["doc_id"] {
		t.Fatalf("parent docs = %#v, want one parent per document", parents)
	}
}
