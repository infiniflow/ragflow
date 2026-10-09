package runtime

import (
	"fmt"
	"sync"
	"testing"
)

// TestWebSearchEvidenceIdentityAcrossCalls keeps distinct results independently citable.
func TestWebSearchEvidenceIdentityAcrossCalls(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	ids := map[string]string{}
	for _, passage := range []string{"first passage", "second passage", "first passage"} {
		deps.WebSearch = stubWebSearch{results: []string{passage}}
		out, err := WebSearchTool(t.Context(), deps, map[string]any{"query": []any{"question"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Payload) != 1 || len(out.EvidenceIDs) != 1 {
			t.Fatalf("missing evidence: %+v", out)
		}
		id := out.EvidenceIDs[0]
		if previous, ok := ids[passage]; ok {
			if id != previous || out.Status != StatusRedundant {
				t.Fatalf("repeat changed identity/status: %+v", out)
			}
		} else {
			for _, previous := range ids {
				if id == previous {
					t.Fatalf("distinct passages share %q", id)
				}
			}
			ids[passage] = id
			if out.Status != StatusOK {
				t.Fatalf("distinct passage rejected: %+v", out)
			}
		}
		if out.Payload[0].(map[string]any)["id"] != id {
			t.Fatal("payload and citation IDs differ")
		}
	}
	if len(kb.Chunks) != 2 {
		t.Fatalf("pool rows=%d, want 2", len(kb.Chunks))
	}
	for _, row := range kb.Chunks {
		if ids[row["content"].(string)] != row["chunk_id"] {
			t.Fatal("pool citation points to a different passage")
		}
	}
}

// TestWebSearchEvidenceIdentityIgnoresResultOrder reuses IDs when the provider reorders hits.
func TestWebSearchEvidenceIdentityIgnoresResultOrder(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.WebSearch = stubWebSearch{results: []string{"alpha", "beta"}}
	first, err := WebSearchTool(t.Context(), deps, map[string]any{"query": []any{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	deps.WebSearch = stubWebSearch{results: []string{"beta", "alpha", "beta"}}
	second, err := WebSearchTool(t.Context(), deps, map[string]any{"query": []any{"two"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.EvidenceIDs) != 2 || len(first.EvidenceIDs) != 2 || second.EvidenceIDs[0] != first.EvidenceIDs[1] || second.EvidenceIDs[1] != first.EvidenceIDs[0] || second.Status != StatusRedundant || len(kb.Chunks) != 2 {
		t.Fatalf("reordered evidence changed: first=%+v second=%+v rows=%d", first, second, len(kb.Chunks))
	}
}

// TestWebSearchEvidenceIdentityConcurrent checks the shared pool without depending on completion order.
func TestWebSearchEvidenceIdentityConcurrent(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			local := deps
			local.WebSearch = stubWebSearch{results: []string{fmt.Sprintf("passage %d", i%4)}}
			out, err := WebSearchTool(t.Context(), local, map[string]any{"query": []any{"query"}})
			if err != nil {
				failures <- err
				return
			}
			if len(out.EvidenceIDs) != 1 {
				failures <- fmt.Errorf("missing citation: %+v", out)
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if len(kb.Chunks) != 4 {
		t.Fatalf("pool rows=%d, want 4 distinct passages", len(kb.Chunks))
	}
	seen := map[string]bool{}
	for _, row := range kb.Chunks {
		id := row["chunk_id"].(string)
		if seen[id] {
			t.Fatalf("duplicate evidence ID %q", id)
		}
		seen[id] = true
	}
}
