package indexdoc

import (
	"testing"

	"ragflow/internal/common"
)

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

// TestMaterializeParentChunksOrdersParentsAfterChildren pins the ordering
// contract every chunk_order_int reader relies on (management chunk list,
// agent grep / deep read): a parent sorts after all source chunks of its
// document, its key is unique within that document so LIMIT/OFFSET and
// search_after pagination slice the same group the same way on every engine, and
// it sits in the reserved parent range so chunks added later through AddChunk
// (which keep appending into [0, ParentChunkOrderBase)) still list before it.
func TestMaterializeParentChunksOrdersParentsAfterChildren(t *testing.T) {
	chunks := []map[string]any{
		{"id": "child-1", "doc_id": "doc-1", "mom": "parent one", "chunk_order_int": float64(0)},
		{"id": "child-2", "doc_id": "doc-1", "mom": "parent one", "chunk_order_int": float64(1)},
		{"id": "child-3", "doc_id": "doc-1", "chunk_order_int": float64(2)},
		{"id": "child-4", "doc_id": "doc-1", "mom": "parent two", "chunk_order_int": float64(3)},
	}

	parents := MaterializeParentChunks("kb-1", chunks)

	if len(parents) != 2 {
		t.Fatalf("parent count = %d, want 2", len(parents))
	}
	for i, want := range []int{common.ParentChunkOrderBase, common.ParentChunkOrderBase + 1} {
		if got, ok := common.GetInt(parents[i]["chunk_order_int"]); !ok || got != want {
			t.Errorf("parent %d chunk_order_int = %v, want %d", i, parents[i]["chunk_order_int"], want)
		}
	}
	// Source chunks keep their own values; the parent values are what moves.
	for i, want := range []int{0, 1, 2, 3} {
		if got, ok := common.GetInt(chunks[i]["chunk_order_int"]); !ok || got != want {
			t.Errorf("child %d chunk_order_int = %v, want %d", i, chunks[i]["chunk_order_int"], want)
		}
	}

	seen := map[int]bool{}
	rows := append(append([]map[string]any{}, parents...), chunks...)
	for _, row := range rows {
		order, ok := common.GetInt(row["chunk_order_int"])
		if !ok {
			t.Fatalf("row %#v has no numeric chunk_order_int", row)
		}
		if seen[order] {
			t.Fatalf("chunk_order_int %d is not unique per document: %#v", order, rows)
		}
		seen[order] = true
	}

	// The document's real slice count must not be affected by the extra rows.
	if chunks[0]["mom_id"] != chunks[1]["mom_id"] {
		t.Fatalf("children of the same parent lost their shared mom_id: %#v", chunks)
	}
	if chunks[2]["mom_id"] != nil {
		t.Fatalf("child without a parent must not gain a mom_id: %#v", chunks[2])
	}
}

// TestMaterializeParentChunksLeavesUnnumberedDocumentsAlone keeps the
// Elasticsearch placement for pipelines that never numbered their chunks:
// inventing values there would move parents from "missing (last)" to the front.
func TestMaterializeParentChunksLeavesUnnumberedDocumentsAlone(t *testing.T) {
	chunks := []map[string]any{
		{"id": "child-1", "doc_id": "doc-1", "mom": "parent one"},
	}

	parents := MaterializeParentChunks("kb-1", chunks)

	if len(parents) != 1 {
		t.Fatalf("parent count = %d, want 1", len(parents))
	}
	if _, exists := parents[0]["chunk_order_int"]; exists {
		t.Fatalf("parent chunk_order_int = %v, want it absent", parents[0]["chunk_order_int"])
	}
}
