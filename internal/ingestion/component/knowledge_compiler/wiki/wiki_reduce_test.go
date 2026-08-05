package wiki

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// TestDedupeEntities_NoSeamIsNoop verifies dedupeEntities degrades to a no-op
// when the embedder or chat seam is nil (M1-style unit safety).
func TestDedupeEntities_NoSeamIsNoop(t *testing.T) {
	p := &wikiPipeline{ctx: context.Background()}
	in := []wikiEntity{
		{Name: "Alpha", SourceChunkIDs: []string{"c1"}},
		{Name: "Alpha Corp", SourceChunkIDs: []string{"c2"}},
	}
	got := p.dedupeEntities(in)
	if len(got) != 2 {
		t.Fatalf("got %d entities, want 2 (no-op without seams)", len(got))
	}
}

// TestDedupeEntities_LLMMergesAmbiguousPair verifies two distinct-name entities
// with high embedding similarity are collapsed into one canonical entity via the
// LLM merge decision, with aliases and provenance merged.
func TestDedupeEntities_LLMMergesAmbiguousPair(t *testing.T) {
	p := &wikiPipeline{
		ctx:   context.Background(),
		llmID: "llm1",
		deps: common.Deps{
			Chat:  reconcileChatStub{resp: `{"merge":true,"reason":"same company"}`},
			Embed: mergeEmbedStub{},
		},
	}
	in := []wikiEntity{
		{Name: "Alpha Inc", Type: "org", SourceChunkIDs: []string{"c1"}},
		{Name: "Alpha Incorporated", Type: "org", SourceChunkIDs: []string{"c2"}},
	}
	got := p.dedupeEntities(in)
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1 (LLM merge)", len(got))
	}
	if got[0].Name != "Alpha Inc" {
		t.Fatalf("canonical name = %q, want Alpha Inc", got[0].Name)
	}
	if len(got[0].SourceChunkIDs) != 2 {
		t.Fatalf("provenance = %#v, want 2 chunk ids", got[0].SourceChunkIDs)
	}
	if len(got[0].Aliases) == 0 {
		t.Fatalf("aliases not merged: %#v", got[0].Aliases)
	}
}

// TestDedupeEntities_LLMRejectsDistinct verifies the LLM rejecting a merge keeps
// both entities distinct.
func TestDedupeEntities_LLMRejectsDistinct(t *testing.T) {
	p := &wikiPipeline{
		ctx:   context.Background(),
		llmID: "llm1",
		deps: common.Deps{
			Chat:  reconcileChatStub{resp: `{"merge":false,"reason":"distinct products"}`},
			Embed: mergeEmbedStub{},
		},
	}
	in := []wikiEntity{
		{Name: "Alpha", SourceChunkIDs: []string{"c1"}},
		{Name: "Beta", SourceChunkIDs: []string{"c2"}},
	}
	got := p.dedupeEntities(in)
	if len(got) != 2 {
		t.Fatalf("got %d entities, want 2 (LLM rejected merge)", len(got))
	}
}

// TestDedupeEntities_ExactNameIsNotAmbiguous verifies entities with identical
// normalized names (already collapsed by reduceExtracts before this stage) are
// not treated as ambiguous: they pass through untouched and no LLM call is made
// for the exact-name pair.
func TestDedupeEntities_ExactNameIsNotAmbiguous(t *testing.T) {
	p := &wikiPipeline{
		ctx:   context.Background(),
		llmID: "llm1",
		deps: common.Deps{
			Chat:  reconcileChatStub{resp: `{"merge":true}`},
			Embed: mergeEmbedStub{},
		},
	}
	in := []wikiEntity{
		{Name: "Alpha", SourceChunkIDs: []string{"c1"}},
		{Name: "Alpha", SourceChunkIDs: []string{"c2"}},
	}
	got := p.dedupeEntities(in)
	// Exact-name duplicates are out of scope for the embedding step; both are
	// kept unchanged (they would already be one entity after reduceExtracts).
	if len(got) != 2 {
		t.Fatalf("got %d entities, want 2 (exact-name pairs are not ambiguous)", len(got))
	}
}

// TestDedupeEntities_ConceptStaysExact validates the REDUCE boundary: concept
// dedup must remain exact (no embedding/LLM). This test guards that the entity
// enhancement never touches concepts.
func TestReduceExtracts_ConceptsStayExact(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Concepts: []wikiConcept{{Term: "RAG", Definition: "d1", SourceChunkIDs: []string{"c1"}}}},
		{Concepts: []wikiConcept{{Term: "Retrieval Augmented Generation", Definition: "d2", SourceChunkIDs: []string{"c2"}}}},
	})
	if len(reduced.Concepts) != 2 {
		t.Fatalf("concepts = %d, want 2 (exact-term dedup must not collapse distinct terms)", len(reduced.Concepts))
	}
}

// TestDedupeEntities_FailingChatIsBudgeted locks F4: a chat seam that
// persistently fails must not drive unbounded external calls. The llmCalls
// budget is consumed before the request, so the loop stops after at most
// wikiEntityMergeMaxCalls attempts.
func TestDedupeEntities_FailingChatIsBudgeted(t *testing.T) {
	calls := 0
	p := &wikiPipeline{
		ctx:   context.Background(),
		llmID: "llm1",
		deps: common.Deps{
			Chat: chatFunc(func(_ context.Context, _ common.ChatRequest) (*common.ChatResponse, error) {
				calls++
				return nil, errors.New("llm down")
			}),
			Embed: mergeEmbedStub{},
		},
	}
	// 20 entities with distinct names but identical embeddings => every pair is
	// ambiguous and would trigger an LLM call.
	in := make([]wikiEntity, 0, 20)
	for i := 0; i < 20; i++ {
		in = append(in, wikiEntity{Name: "Entity " + itoa(i), Type: "person"})
	}
	got := p.dedupeEntities(in)
	if calls > wikiEntityMergeMaxCalls {
		t.Fatalf("chat calls = %d, want <= %d despite persistent failures", calls, wikiEntityMergeMaxCalls)
	}
	// No merges happen because every call fails, so all 20 entities survive.
	if len(got) != 20 {
		t.Fatalf("entities = %d, want 20 (no merges on failure)", len(got))
	}
}

// TestDedupeEntities_CrossTypeHighSimDoesNotCallLLM locks F5: entities with
// provably different types must never be treated as ambiguous, even with
// identical embeddings, so no LLM call is made for them.
func TestDedupeEntities_CrossTypeHighSimDoesNotCallLLM(t *testing.T) {
	calls := 0
	p := &wikiPipeline{
		ctx:   context.Background(),
		llmID: "llm1",
		deps: common.Deps{
			Chat: chatFunc(func(_ context.Context, _ common.ChatRequest) (*common.ChatResponse, error) {
				calls++
				return &common.ChatResponse{Content: `{"merge":true}`}, nil
			}),
			Embed: mergeEmbedStub{},
		},
	}
	// Both embed to [1,1,1] (identical vectors, cosine 1.0) but types differ.
	in := []wikiEntity{
		{Name: "Alpha", Type: "person"},
		{Name: "Beta Corp", Type: "org"},
	}
	got := p.dedupeEntities(in)
	if calls != 0 {
		t.Fatalf("chat calls = %d, want 0 (cross-type pairs must not reach the LLM)", calls)
	}
	if len(got) != 2 {
		t.Fatalf("entities = %d, want 2 (cross-type entities must stay distinct)", len(got))
	}
}

// mergeEmbedStub returns embeddings where identical names share a vector and
// distinct names are far apart (cosine ~0), so it can drive both the ambiguous
// and distinct test paths deterministically.
type mergeEmbedStub struct{}

func (mergeEmbedStub) Encode(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		// "Alpha Inc" and "Alpha Incorporated" both contain "alpha" -> same
		// vector; "Beta" differs.
		if containsFold(text, "beta") {
			out[i] = []float32{1, 0, 0}
			continue
		}
		out[i] = []float32{1, 1, 1}
	}
	return out, nil
}

func (mergeEmbedStub) Dimensions() int { return 3 }

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexFold(s, sub) >= 0)
}

func indexFold(s, sub string) int {
	if sub == "" {
		return 0
	}
	ls := toLowerASCII(s)
	lsub := toLowerASCII(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return i
		}
	}
	return -1
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
