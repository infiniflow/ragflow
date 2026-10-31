package nlp

import (
	"reflect"
	"testing"
)

// Retrieval projects engine q_<dim>_vec fields under "vector". Parent
// expansion must retain that embedding's dimension and source identity even
// though the content and citation identity now belong to the hidden parent.
func TestParentExpansionPreservesChildVectorProvenance(t *testing.T) {
	for _, hydrated := range []bool{false, true} {
		name := "placeholder"
		if hydrated {
			name = "hydrated"
		}
		t.Run(name, func(t *testing.T) {
			vec := make([]float64, 384)
			if hydrated {
				vec[0], vec[1] = 0.8, 0.6
			}
			child := map[string]interface{}{
				"chunk_id": "child-a", "mom_id": "parent-a", "doc_id": "doc-a", "kb_id": "kb-a",
				"content_with_weight": "The warranty lasts twelve months.",
				"similarity":          0.9, "vector": vec,
			}
			engine := &parentChunkScopeEngine{parentChunk: map[string]interface{}{
				"id": "parent-a", "doc_id": "doc-a", "kb_id": "kb-a",
				"content_with_weight": "The warranty lasts twelve months. Contact support for repairs.",
			}}
			got := RetrievalByChildren([]map[string]interface{}{child}, []string{"tenant-a"}, engine, t.Context())
			if len(got) != 1 || got[0]["chunk_id"] != "parent-a" {
				t.Fatalf("expected parent content identity: %#v", got)
			}
			if !reflect.DeepEqual(got[0]["vector"], vec) {
				t.Errorf("parent expansion discarded the child's %d-dimensional vector", len(vec))
			}
			if got[0]["vector_chunk_id"] != "child-a" {
				t.Errorf("deferred hydration source = %#v, want child-a", got[0]["vector_chunk_id"])
			}
		})
	}
}
