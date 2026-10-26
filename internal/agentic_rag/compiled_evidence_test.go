//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package agentic_rag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
)

// installAnchorSearch replaces the pre-loop chunk search so the strategy can be
// exercised without a retrieval backend.
func installAnchorSearch(t *testing.T, fn func(context.Context, string, []string, []string, string, int) []runtime.RetrievalChunk) {
	t.Helper()
	prev := compiledAnchorSearch
	compiledAnchorSearch = fn
	t.Cleanup(func() { compiledAnchorSearch = prev })
}

// installClaimLookup replaces the chunk -> claim lookup.
func installClaimLookup(t *testing.T, fn func(context.Context, string, []string, []string, []string, int) []claimHit) {
	t.Helper()
	prev := compiledClaimsByChunk
	compiledClaimsByChunk = fn
	t.Cleanup(func() { compiledClaimsByChunk = prev })
}

func installClaimProbe(t *testing.T, present bool) {
	t.Helper()
	prev := claimRowsPresent
	claimRowsPresent = func(context.Context, string, []string) bool { return present }
	t.Cleanup(func() { claimRowsPresent = prev })
}

func chunkPage(ids ...string) []runtime.RetrievalChunk {
	out := make([]runtime.RetrievalChunk, 0, len(ids))
	for _, id := range ids {
		out = append(out, runtime.RetrievalChunk{ID: id, Content: "body of " + id})
	}
	return out
}

func questionMessages(q string) []*schema.Message {
	return []*schema.Message{{Role: schema.User, Content: q}}
}

// TestPrefetchSearchesThenLooksUpClaims: the compiled leg is anchored on the
// search - the lookup is asked about the chunk ids the retrieval returned, in
// retrieval order, and the rendered block is answer material with handles.
func TestPrefetchSearchesThenLooksUpClaims(t *testing.T) {
	installClaimProbe(t, true)
	var gotChunks []string
	installAnchorSearch(t, func(_ context.Context, _ string, _ []string, _ []string, q string, _ int) []runtime.RetrievalChunk {
		if q != "when was Acme founded" {
			t.Errorf("anchor search query = %q", q)
		}
		return chunkPage("c1", "c2")
	})
	installClaimLookup(t, func(_ context.Context, _ string, _ []string, chunkIDs, _ []string, limit int) []claimHit {
		gotChunks = append([]string(nil), chunkIDs...)
		if limit < compiledPrefetchClaims {
			t.Errorf("lookup limit = %d, want at least %d", limit, compiledPrefetchClaims)
		}
		return []claimHit{
			{chunkID: "c2", docID: "d2", name: "Acme employs 900 people", quote: "a workforce of 900"},
			{chunkID: "c1", docID: "d1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"},
		}
	})
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())

	pf := prefetchCompiledEvidence(ctx, "tenant-1", []string{"kb1"}, questionMessages("when was Acme founded"))

	if len(gotChunks) != 2 || gotChunks[0] != "c1" || gotChunks[1] != "c2" {
		t.Fatalf("lookup must be asked about the retrieved chunks, got %v", gotChunks)
	}
	if len(pf.claims) != 2 {
		t.Fatalf("claims = %d, want 2", len(pf.claims))
	}
	// Anchored ranking: the best-ranked chunk's claim comes first even though the
	// other claim was returned first by the store.
	if pf.claims[0].chunkID != "c1" {
		t.Errorf("claims must follow the retrieval's chunk order, got first %q", pf.claims[0].chunkID)
	}
	for _, want := range []string{`doc_id="d1"`, `doc_id="d2"`, "ref=0", "ref=1"} {
		if !strings.Contains(pf.evidence, want) {
			t.Errorf("evidence must contain %q, got:\n%s", want, pf.evidence)
		}
	}
	if len(pf.chunks) != 2 {
		t.Errorf("prefetch must report its anchors, got %v", pf.chunks)
	}
}

// TestPrefetchSkippedWithoutClaimRows: a compiled but claim-less corpus (tree,
// timeline, mindmap) must not pay for a search or a verdict call.
func TestPrefetchSkippedWithoutClaimRows(t *testing.T) {
	installClaimProbe(t, false)
	installAnchorSearch(t, func(context.Context, string, []string, []string, string, int) []runtime.RetrievalChunk {
		t.Error("a corpus without claim rows must not be searched")
		return nil
	})
	pf := prefetchCompiledEvidence(context.Background(), "tenant-1", []string{"kb1"}, questionMessages("q"))
	if len(pf.claims) != 0 || pf.evidence != "" {
		t.Errorf("claim-less corpus must yield an empty prefetch, got %+v", pf)
	}
}

// TestPrefetchEmptyDocumentScopeSkips: a non-nil EMPTY turn scope allows no
// documents; the retrieval would read it as "unrestricted", so the prefetch must
// stop before searching.
func TestPrefetchEmptyDocumentScopeSkips(t *testing.T) {
	installClaimProbe(t, true)
	installAnchorSearch(t, func(context.Context, string, []string, []string, string, int) []runtime.RetrievalChunk {
		t.Error("an empty document scope must stop before the search")
		return nil
	})
	ctx := withDocumentScope(context.Background(), []string{})
	pf := prefetchCompiledEvidence(ctx, "tenant-1", []string{"kb1"}, questionMessages("q"))
	if len(pf.claims) != 0 {
		t.Errorf("empty scope must yield an empty prefetch, got %+v", pf)
	}
}

// TestPrefetchWithoutAnchorChunks: a search that finds nothing leaves the loop as
// the only path, and no verdict call is bought.
func TestPrefetchWithoutAnchorChunks(t *testing.T) {
	installClaimProbe(t, true)
	installAnchorSearch(t, func(context.Context, string, []string, []string, string, int) []runtime.RetrievalChunk {
		return nil
	})
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		t.Error("no anchor chunks means no claim lookup")
		return nil
	})
	pf := prefetchCompiledEvidence(context.Background(), "tenant-1", []string{"kb1"}, questionMessages("q"))
	if len(pf.claims) != 0 || len(pf.chunks) != 0 {
		t.Errorf("no anchors must yield an empty prefetch, got %+v", pf)
	}
}

// TestCompiledClaimsRankedAndDeduped: the page is deduped by statement, ordered by
// anchor then coverage, and capped.
func TestCompiledClaimsRankedAndDeduped(t *testing.T) {
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{
			{chunkID: "c2", name: "Acme employs 900 people", quote: "a workforce of 900"},
			{chunkID: "c1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"},
			{chunkID: "c1", name: "acme WAS founded in 1994", quote: "duplicate"},
			{chunkID: "c1", name: "Acme is based in Zurich", quote: "the Zurich-based firm"},
		}
	})
	claims := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, []string{"c1", "c2"}, "where was Acme founded and when", 2)
	if len(claims) != 2 {
		t.Fatalf("claims = %d, want the cap 2: %+v", len(claims), claims)
	}
	for _, c := range claims {
		if c.chunkID != "c1" {
			t.Errorf("the cap must be spent on the best-ranked chunk first, got %q", c.chunkID)
		}
	}
	if claims[0].name != "Acme was founded in 1994" {
		t.Errorf("the on-topic claim must lead, got %q", claims[0].name)
	}
}

// TestCompiledClaimsFallBackWhenNothingClearsTheFloor: the anchor chunk proves
// relevance even when the claims are worded differently from the question, so a
// floor that would empty the page falls back to the anchored claims instead.
func TestCompiledClaimsFallBackWhenNothingClearsTheFloor(t *testing.T) {
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{{chunkID: "c1", name: "Something unrelated entirely", quote: "x"}}
	})
	claims := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, []string{"c1"}, "when was Acme founded", 4)
	if len(claims) != 1 {
		t.Fatalf("the anchored claim must survive an empty floor result, got %+v", claims)
	}
}

// claimRow builds the raw row an engine read returns for a claim: the payload is
// JSON inside content_with_weight, the source chunk ids are a keyword array.
func claimRow(name, docID, quote string, chunkIDs ...string) map[string]interface{} {
	payload := map[string]interface{}{
		"name":             name,
		"source_chunk_ids": chunkIDs,
		"evidence":         []interface{}{map[string]interface{}{"quote": quote}},
	}
	encoded, _ := json.Marshal(payload)
	ids := make([]interface{}, len(chunkIDs))
	for i, id := range chunkIDs {
		ids[i] = id
	}
	return map[string]interface{}{
		"content_with_weight": string(encoded),
		"source_chunk_ids":    ids,
		"doc_id":              docID,
	}
}

// TestParseClaimHitKeepsEverySourceChunk: a claim compiled from a multi-chunk
// passage names all of them, and the anchored lookup has to see all of them.
func TestParseClaimHitKeepsEverySourceChunk(t *testing.T) {
	hit, ok := parseClaimHit(claimRow("Acme was founded in 1994", "d1", "Acme, founded in 1994", "c9", "c2"))
	if !ok {
		t.Fatal("claim row must parse")
	}
	if len(hit.sourceChunkIDs) != 2 || hit.sourceChunkIDs[0] != "c9" || hit.sourceChunkIDs[1] != "c2" {
		t.Fatalf("sourceChunkIDs = %v, want [c9 c2]", hit.sourceChunkIDs)
	}
	if hit.chunkID != "c9" {
		t.Errorf("default attribution = %q, want the stored first source chunk", hit.chunkID)
	}
}

// TestClaimsAttributedToTheMatchedAnchor: the claim is attributed to the anchor
// it actually matched, not to whichever source chunk happened to be stored first.
func TestClaimsAttributedToTheMatchedAnchor(t *testing.T) {
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{{
			docID:          "d1",
			chunkID:        "c9", // NOT in the page
			sourceChunkIDs: []string{"c9", "c2"},
			name:           "Acme was founded in 1994",
			quote:          "Acme, founded in 1994",
		}}
	})
	claims := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, []string{"c1", "c2"}, "when was Acme founded", 4)
	if len(claims) != 1 {
		t.Fatalf("claims = %d, want 1: %+v", len(claims), claims)
	}
	if claims[0].chunkID != "c2" {
		t.Errorf("claim must be attributed to the matched anchor c2, got %q", claims[0].chunkID)
	}
}

// TestClaimsOffEveryAnchorAreDropped: a claim compiled from a passage the search
// did not return is not evidence for what the search found. It must not be
// appended to the page, however plausible its text looks.
func TestClaimsOffEveryAnchorAreDropped(t *testing.T) {
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{
			{docID: "d2", chunkID: "other", sourceChunkIDs: []string{"other"}, name: "Burj Khalifa is 828 m tall", quote: "828 metres"},
			{docID: "d1", chunkID: "c1", sourceChunkIDs: []string{"c1"}, name: "Acme was founded in 1994", quote: "Acme, founded in 1994"},
		}
	})
	claims := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, []string{"c1"}, "when was Acme founded", 4)
	if len(claims) != 1 {
		t.Fatalf("only the anchored claim may survive, got %+v", claims)
	}
	if claims[0].chunkID != "c1" {
		t.Errorf("surviving claim = %q, want c1", claims[0].chunkID)
	}
}

func noticeHits(ids ...string) []snippetHit {
	out := make([]snippetHit, 0, len(ids))
	for _, id := range ids {
		out = append(out, snippetHit{chunk: runtime.RetrievalChunk{ID: id, Content: "body " + id}, snippet: "snip " + id})
	}
	return out
}

// TestCompiledEvidenceNoticeShownOnce: a result set carries its claims the first
// time, and a later search that returns the same claims does not pay for them
// again - the transcript already holds them under the same handles.
func TestCompiledEvidenceNoticeShownOnce(t *testing.T) {
	installClaimProbe(t, true)
	installClaimLookup(t, func(_ context.Context, _ string, _ []string, chunkIDs, _ []string, _ int) []claimHit {
		return []claimHit{{chunkID: chunkIDs[0], docID: "d1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"}}
	})
	ctx := withCompiledRun(WithQuery(context.Background(), "when was Acme founded"), "tenant-1", []string{"kb1"})
	ctx = WithEvidenceRegistry(ctx, NewEvidenceRegistry())

	first := compiledEvidenceNotice(ctx, searchChunksToolName, noticeHits("c1", "c2"))
	if !strings.Contains(first, "<compiled_evidence count=\"1\">") || !strings.Contains(first, "- [claim] Acme was founded in 1994") {
		t.Fatalf("first result must carry the compiled claim, got:\n%s", first)
	}
	if !strings.Contains(first, "do NOT call list_chunks") {
		t.Errorf("the block must tell the model it can answer without a deep read, got:\n%s", first)
	}
	if second := compiledEvidenceNotice(ctx, searchChunksToolName, noticeHits("c1", "c2")); second != "" {
		t.Errorf("an already shown claim must not be repeated, got:\n%s", second)
	}
}

// TestCompiledEvidenceNoticeSkippedWithoutCorpusClaims: no compiled claims means
// no block, and the corpus-level probe runs once for the whole turn.
func TestCompiledEvidenceNoticeSkippedWithoutCorpusClaims(t *testing.T) {
	probes := 0
	prev := claimRowsPresent
	claimRowsPresent = func(context.Context, string, []string) bool { probes++; return false }
	t.Cleanup(func() { claimRowsPresent = prev })
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		t.Error("a corpus without claims must not be looked up")
		return nil
	})
	ctx := withCompiledRun(context.Background(), "tenant-1", []string{"kb1"})

	for i := 0; i < 3; i++ {
		if out := compiledEvidenceNotice(ctx, searchChunksToolName, noticeHits("c1")); out != "" {
			t.Fatalf("call %d: expected no block, got:\n%s", i, out)
		}
	}
	if probes != 1 {
		t.Errorf("the corpus probe must run once per turn, ran %d times", probes)
	}
}

// TestCompiledEvidenceNoticeWithoutRunScope: a run that published no scope (the
// pre-loop path builds its own) must not break the locate renderers.
func TestCompiledEvidenceNoticeWithoutRunScope(t *testing.T) {
	if out := compiledEvidenceNotice(context.Background(), searchChunksToolName, noticeHits("c1")); out != "" {
		t.Errorf("no run scope must yield no block, got:\n%s", out)
	}
	web := []snippetHit{{chunk: runtime.RetrievalChunk{ID: "web-1", URL: "https://example.test"}, web: true}}
	if out := compiledEvidenceNotice(context.Background(), webSearchToolName, web); out != "" {
		t.Errorf("web hits have no corpus chunk behind them, got:\n%s", out)
	}
}

// TestLocateResultsCarryCompiledEvidenceFirst: the block is rendered AHEAD of the
// hits it was compiled from, which is the ordering the strategy depends on.
func TestLocateResultsCarryCompiledEvidenceFirst(t *testing.T) {
	installClaimProbe(t, true)
	installClaimLookup(t, func(_ context.Context, _ string, _ []string, chunkIDs, _ []string, _ int) []claimHit {
		return []claimHit{{chunkID: chunkIDs[0], docID: "d1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"}}
	})
	ctx := withCompiledRun(WithQuery(context.Background(), "when was Acme founded"), "tenant-1", []string{"kb1"})
	ctx = WithEvidenceRegistry(ctx, NewEvidenceRegistry())

	out := formatLocateResultsXML(ctx, searchChunksToolName, "acme", noticeHits("c1"))
	claimAt := strings.Index(out, "<compiled_evidence")
	chunkAt := strings.Index(out, "<chunk ")
	if claimAt < 0 || chunkAt < 0 {
		t.Fatalf("result must carry both the block and the hits, got:\n%s", out)
	}
	if claimAt > chunkAt {
		t.Errorf("compiled evidence must precede the raw hits, got:\n%s", out)
	}
}
