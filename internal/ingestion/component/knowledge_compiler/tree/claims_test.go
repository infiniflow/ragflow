package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// recordingEmbedder captures the texts it was asked to embed, so a test can
// assert what actually reached the vector.
type recordingEmbedder struct {
	inputs []string
}

func (e *recordingEmbedder) Encode(ctx context.Context, texts []string) ([][]float32, error) {
	e.inputs = append(e.inputs, texts...)
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1}
	}
	return out, nil
}

func (e *recordingEmbedder) Dimensions() int { return 1 }

// fakeChatForClaims returns canned claim JSON for whichever prompt it sees.
type fakeChatForClaims struct {
	reply string
	calls int
}

func (f *fakeChatForClaims) Chat(ctx context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	f.calls++
	return &common.ChatResponse{Content: f.reply}, nil
}

// flakyChatForClaims fails the first N calls with a retryable error, then
// succeeds — exercises the claim-extraction retry path.
type flakyChatForClaims struct {
	reply    string
	failures int
	calls    int
}

func (f *flakyChatForClaims) Chat(ctx context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, fmt.Errorf("429 rate limit exceeded")
	}
	return &common.ChatResponse{Content: f.reply}, nil
}

func TestLocateEvidenceExactAndReflowed(t *testing.T) {
	text := "Post-war immigration added 2.1 million residents to the city."
	if _, _, ok := LocateEvidence(text, text); !ok {
		t.Fatal("exact quote not located")
	}
	// A sentence copied across a line break collapses to a single space; the
	// match must still succeed and map back to the original offsets.
	src := "Post-war immigration added\n2.1 million residents to the city."
	quote := text
	start, end, ok := LocateEvidence(quote, src)
	if !ok {
		t.Fatalf("reflowed quote not located")
	}
	if got, want := strings.Join(strings.Fields(src[start:end]), " "), strings.Join(strings.Fields(quote), " "); got != want {
		t.Fatalf("span sliced %q, want %q", got, want)
	}
}

func TestLocateEvidenceMultibyteOffsets(t *testing.T) {
	// A byte offset from strings.Index runs ahead of the rune offset, so a map
	// indexed per rune resolves every lookup past the first multi-byte rune to
	// the wrong span. Offsets must stay byte-aligned.
	src := "前言。中国的首都是北京。尾声。"
	quote := "中国的首都是北京。"
	start, end, ok := LocateEvidence(quote, src)
	if !ok {
		t.Fatalf("CJK quote not located in %q", src)
	}
	if got := src[start:end]; got != quote {
		t.Fatalf("span sliced %q, want %q", got, quote)
	}

	// A match that runs to the end of the text has no following byte to take
	// the offset from; the end must be len(text), not "last matched byte + 1",
	// which would land inside the final rune.
	tail := "前言。中国的首都是北京。"
	start, end, ok = LocateEvidence(quote, tail)
	if !ok {
		t.Fatal("quote at the end of the text not located")
	}
	if end != len(tail) {
		t.Fatalf("end = %d, want len(text) = %d", end, len(tail))
	}
	if got := tail[start:end]; got != quote {
		t.Fatalf("span sliced %q, want %q", got, quote)
	}
}

func TestLocateEvidenceRejectsHallucination(t *testing.T) {
	if _, _, ok := LocateEvidence("a sentence that is not in the source", "something else entirely"); ok {
		t.Fatal("hallucinated quote should not be located")
	}
	if _, _, ok := LocateEvidence("", "some text"); ok {
		t.Fatal("empty quote should not be located")
	}
	if _, _, ok := LocateEvidence("q", ""); ok {
		t.Fatal("empty source should not be located")
	}
}

func TestValidateClaimsKeepsClaimWhenEvidenceRejected(t *testing.T) {
	// Soft mode: an unverifiable quote is dropped, but the claim survives
	// without evidence. Discarding the claim too would silently lose facts.
	claims := []Claim{{
		Name:           "invented",
		SourceChunkIDs: []string{"c1"},
		Evidence:       []EvidenceRef{{Quote: "not in source", ChunkID: "c1"}},
	}}
	_, verified, rejected := ValidateClaims(claims, map[string]string{"c1": "real text"}, EvidenceGateSoft)
	if verified != 0 || rejected != 1 {
		t.Fatalf("got verified=%d rejected=%d, want 0/1", verified, rejected)
	}
	if len(claims) != 1 {
		t.Fatalf("claim should survive in soft mode, got %d", len(claims))
	}
	if claims[0].Evidence != nil {
		t.Fatalf("rejected evidence should be removed, got %v", claims[0].Evidence)
	}
}

func TestValidateClaimsRecordsOffsets(t *testing.T) {
	src := "Preamble.   The   capital   is   Paris.  Trailing."
	claims := []Claim{{
		Name:           "capital",
		SourceChunkIDs: []string{"c1"},
		Evidence:       []EvidenceRef{{Quote: "The capital is Paris.", ChunkID: "c1"}},
	}}
	_, verified, rejected := ValidateClaims(claims, map[string]string{"c1": src}, EvidenceGateSoft)
	if verified != 1 || rejected != 0 {
		t.Fatalf("got verified=%d rejected=%d, want 1/0", verified, rejected)
	}
	got := claims[0].Evidence
	if len(got) != 1 {
		t.Fatalf("evidence lost: %v", claims[0].Evidence)
	}
	// The span must slice out the ORIGINAL whitespace, not the normalized one.
	if want := "The   capital   is   Paris."; src[got[0].Start:got[0].End] != want {
		t.Fatalf("span sliced %q, want %q", src[got[0].Start:got[0].End], want)
	}
}

func TestParseClaimItemsToleratesProseAndFences(t *testing.T) {
	for _, raw := range []string{
		`{"items": [{"name": "a"}]}`,
		"Here is the JSON:\n```json\n" + `{"items": [{"name": "a"}]}` + "\n```",
		"Sure! " + `{"items": [{"name": "a"}]}` + " hope that helps",
	} {
		items := parseClaimItems(raw)
		if len(items) != 1 || items[0].Name != "a" {
			t.Fatalf("parseClaimItems(%q) = %+v, want one claim named a", raw, items)
		}
	}
	if items := parseClaimItems(`{"items": [{"description": "no name"}]}`); len(items) != 0 {
		t.Fatalf("claim without a name should be skipped: %+v", items)
	}
	if items := parseClaimItems("not json at all"); items != nil {
		t.Fatalf("unparseable input should yield nil, got %+v", items)
	}
}

func TestBuildClaimContentFallsBackToText(t *testing.T) {
	texts := []string{"raw alpha", "raw beta"}
	chunkIDs := []string{"c1", "c2"}

	// No claims at all -> "" so the caller can fall back to raw text.
	if got := BuildClaimContent(texts, chunkIDs, []int{0, 1}, nil); got != "" {
		t.Fatalf("expected empty content with no claims, got %q", got)
	}

	// A chunk with no claims contributes its own text rather than vanishing.
	claims := map[string][]Claim{
		"c1": {{Name: "Alpha happened"}},
	}
	got := BuildClaimContent(texts, chunkIDs, []int{0, 1}, claims)
	if !strings.Contains(got, "- Alpha happened") {
		t.Fatalf("claim missing from content: %q", got)
	}
	if !strings.Contains(got, "raw beta") {
		t.Fatalf("claim-less chunk should fall back to its text: %q", got)
	}

	// Verified evidence is rendered next to the claim.
	claims["c1"] = []Claim{{Name: "Alpha happened",
		Evidence: []EvidenceRef{{Quote: "alpha really happened", ChunkID: "c1"}}}}
	got = BuildClaimContent(texts, chunkIDs, []int{0}, claims)
	if !strings.Contains(got, `Evidence: "alpha really happened"`) {
		t.Fatalf("evidence missing from content: %q", got)
	}
}

func TestExtractClaimsForChunksRequiresChat(t *testing.T) {
	// No chat client -> extraction disabled, not an error.
	deps := common.Deps{TenantID: "t"}
	chunks := []common.Chunk{{ID: "c1", Text: "some text"}}
	if got := ExtractClaimsForChunks(context.Background(), deps, "llm", chunks, EvidenceGateSoft); got != nil {
		t.Fatalf("expected nil without a chat client, got %v", got)
	}
}

func TestExtractClaimsForChunksValidatesAndKeysByChunk(t *testing.T) {
	src := "The capital of France is Paris."
	reply, _ := json.Marshal(map[string]any{"items": []any{
		map[string]any{
			"type":             "claim",
			"name":             "Paris is the capital of France",
			"source_chunk_ids": []any{"c1"},
			"evidence":         []any{map[string]any{"quote": src, "chunk_id": "c1"}},
		},
	}})
	deps := common.Deps{Chat: &fakeChatForClaims{reply: string(reply)}, TenantID: "t"}
	chunks := []common.Chunk{{ID: "c1", Text: src}}

	got := ExtractClaimsForChunks(context.Background(), deps, "llm", chunks, EvidenceGateSoft)
	claims := got["c1"]
	if len(claims) != 1 {
		t.Fatalf("expected one claim for c1, got %+v", got)
	}
	if claims[0].Name != "Paris is the capital of France" {
		t.Fatalf("unexpected name: %q", claims[0].Name)
	}
	if len(claims[0].Evidence) != 1 {
		t.Fatalf("evidence should have been verified: %+v", claims[0])
	}
	if s, e := claims[0].Evidence[0].Start, claims[0].Evidence[0].End; src[s:e] != src {
		t.Fatalf("span %d:%d does not cover the quote", s, e)
	}
	// description defaults to the claim when the model omits it.
	if claims[0].Description != claims[0].Name {
		t.Fatalf("description should default to name, got %q", claims[0].Description)
	}
}

func TestExtractClaimsForChunksAttribution(t *testing.T) {
	// Attribution mirrors Python raptor._extract_claim_for_chunk: trust the
	// model when it names a chunk in the batch, fall back to the chunk the
	// verified quote was located in, and drop the claim when neither can place
	// it (rather than filing it under text it never came from).
	src := "The capital of France is Paris."
	marshal := func(items ...any) string {
		reply, err := json.Marshal(map[string]any{"items": items})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(reply)
	}
	run := func(reply string) map[string][]Claim {
		deps := common.Deps{Chat: &fakeChatForClaims{reply: reply}, TenantID: "t"}
		return ExtractClaimsForChunks(context.Background(), deps, "llm", []common.Chunk{{ID: "c1", Text: src}}, EvidenceGateSoft)
	}

	got := run(marshal(map[string]any{
		"name":             "Paris is the capital",
		"source_chunk_ids": []any{"c1"},
		"evidence":         []any{map[string]any{"quote": src, "chunk_id": "c1"}},
	}))
	if len(got["c1"]) != 1 || got["c1"][0].SourceChunkIDs[0] != "c1" {
		t.Fatalf("a batch-local source should be kept: %+v", got)
	}

	// The model named a chunk outside the batch — the quote still locates the
	// claim, so attribution is recovered from the evidence.
	got = run(marshal(map[string]any{
		"name":             "Paris is the capital",
		"source_chunk_ids": []any{"does-not-exist"},
		"evidence":         []any{map[string]any{"quote": src, "chunk_id": "c1"}},
	}))
	if len(got["c1"]) != 1 || got["c1"][0].SourceChunkIDs[0] != "c1" {
		t.Fatalf("attribution should be recovered from the verified evidence: %+v", got)
	}

	got = run(marshal(map[string]any{
		"name":             "a claim",
		"source_chunk_ids": []any{"does-not-exist"},
	}))
	if len(got) != 0 {
		t.Fatalf("an unattributable claim should be dropped, got %+v", got)
	}
}

func TestExtractClaimsForChunksBatchesChunksPerCall(t *testing.T) {
	// Mirrors Python _CLAIM_BATCH_SIZE: one call harvests a whole batch, so the
	// call count stays flat instead of growing one-per-chunk.
	reply, _ := json.Marshal(map[string]any{"items": []any{
		map[string]any{
			"name":             "a claim",
			"source_chunk_ids": []any{"c1"},
			"evidence":         []any{map[string]any{"quote": "alpha text", "chunk_id": "c1"}},
		},
	}})
	chat := &fakeChatForClaims{reply: string(reply)}
	deps := common.Deps{Chat: chat, TenantID: "t"}
	chunks := []common.Chunk{
		{ID: "c1", Text: "alpha text"},
		{ID: "c2", Text: "beta text"},
		{ID: "c3", Text: "gamma text"},
		{ID: "c4", Text: "delta text"},
	}
	ExtractClaimsForChunks(context.Background(), deps, "llm", chunks, EvidenceGateSoft)
	if chat.calls != 1 {
		t.Fatalf("expected 1 call for %d chunks, got %d", len(chunks), chat.calls)
	}
}

func TestExtractClaimsForChunksRetriesTransientErrors(t *testing.T) {
	// Mirrors Python _RETRYABLE_LLM_ERR: a rate-limit failure is worth waiting
	// out instead of silently dropping the batch's claims.
	old := claimRetryBaseDelay
	claimRetryBaseDelay = time.Millisecond
	t.Cleanup(func() { claimRetryBaseDelay = old })

	reply, _ := json.Marshal(map[string]any{"items": []any{
		map[string]any{
			"name":             "a claim",
			"source_chunk_ids": []any{"c1"},
			"evidence":         []any{map[string]any{"quote": "alpha text", "chunk_id": "c1"}},
		},
	}})
	deps := common.Deps{Chat: &flakyChatForClaims{reply: string(reply), failures: 1}, TenantID: "t"}
	got := ExtractClaimsForChunks(context.Background(), deps, "llm", []common.Chunk{{ID: "c1", Text: "alpha text"}}, EvidenceGateSoft)
	if len(got["c1"]) != 1 {
		t.Fatalf("a retried call should still yield its claims, got %+v", got)
	}
}

func TestValidateClaimsHardModeDropsTheClaim(t *testing.T) {
	claims := []Claim{{
		Name:           "invented",
		SourceChunkIDs: []string{"c1"},
		Evidence:       []EvidenceRef{{Quote: "not in source", ChunkID: "c1"}},
	}}
	kept, verified, rejected := ValidateClaims(claims, map[string]string{"c1": "real text"}, EvidenceGateHard)
	if verified != 0 || rejected != 1 {
		t.Fatalf("got verified=%d rejected=%d, want 0/1", verified, rejected)
	}
	if len(kept) != 0 {
		t.Fatalf("hard mode should drop the claim, got %d survivor(s)", len(kept))
	}
}

func TestBuildTreeClaimProductsExcludesEvidenceFromVector(t *testing.T) {
	deps := common.Deps{Embed: &recordingEmbedder{}, TenantID: "t"}
	claims := map[string][]Claim{
		"c1": {{
			Name:           "Paris is the capital",
			Description:    "Paris is the capital",
			SourceChunkIDs: []string{"c1"},
			Evidence:       []EvidenceRef{{Quote: "VERBATIM_SOURCE_SENTENCE", ChunkID: "c1", Start: 0, End: 22}},
		}},
	}
	prods, err := buildTreeClaimProducts(context.Background(), deps, "d1", claims, "")
	if err != nil {
		t.Fatalf("buildTreeClaimProducts: %v", err)
	}
	if len(prods) != 1 {
		t.Fatalf("expected 1 product, got %d", len(prods))
	}
	emb := deps.Embed.(*recordingEmbedder)
	if len(emb.inputs) != 1 {
		t.Fatalf("expected 1 embedding request, got %d", len(emb.inputs))
	}
	if strings.Contains(emb.inputs[0], "VERBATIM_SOURCE_SENTENCE") {
		t.Fatalf("evidence reached the vector input: %q", emb.inputs[0])
	}
	if !strings.Contains(emb.inputs[0], "Paris is the capital") {
		t.Fatalf("claim text missing from vector input: %q", emb.inputs[0])
	}

	// The row must be stamped so the writer can keep it out of the artifacts
	// tree, while the payload still carries the evidence for the read path.
	if got := prods[0].Meta["entity_type"]; got != "claim" {
		t.Fatalf("entity_type = %v, want claim", got)
	}
	if got := prods[0].Meta["kind"]; got != "claim" {
		t.Fatalf("kind = %v, want claim", got)
	}
	if got := prods[0].Meta["compile_kwd"]; got != "tree" {
		t.Fatalf("compile_kwd = %v, want tree", got)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(prods[0].Content), &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if payload["evidence"] == nil {
		t.Fatalf("evidence must be stored on the payload: %v", payload)
	}
	if payload["type"] != "claim" {
		t.Fatalf("payload type = %v, want claim", payload["type"])
	}
}
