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
	"fmt"
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
		if limit < len(chunkIDs) {
			t.Errorf("lookup limit = %d, want at least one row per chunk (%d)", limit, len(chunkIDs))
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

// TestPrefetchServesDedupedChunksOnlyAsClaims: a chunk whose statement another
// chunk of the same page already stated is deduped off the claim page, but it
// still HAS claims - so its raw text must not be appended as if it carried none.
// Otherwise the same fact is offered twice, once as a statement and once as prose.
func TestPrefetchServesDedupedChunksOnlyAsClaims(t *testing.T) {
	installClaimProbe(t, true)
	installAnchorSearch(t, func(context.Context, string, []string, []string, string, int) []runtime.RetrievalChunk {
		return chunkPage("c1", "c2")
	})
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{
			{chunkID: "c1", docID: "d1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"},
			{chunkID: "c2", docID: "d2", name: "ACME was founded in 1994", quote: "duplicate of c1"},
		}
	})
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())

	pf := prefetchCompiledEvidence(ctx, "tenant-1", []string{"kb1"}, questionMessages("when was Acme founded"))

	if len(pf.claims) != 1 {
		t.Fatalf("claims = %d, want the deduped page of 1: %+v", len(pf.claims), pf.claims)
	}
	if !strings.Contains(pf.evidence, "Acme was founded in 1994") {
		t.Errorf("the surviving statement must be on the page, got:\n%s", pf.evidence)
	}
	for _, body := range []string{"body of c1", "body of c2"} {
		if strings.Contains(pf.evidence, body) {
			t.Errorf("chunk %q already has claims and must not be served as raw text:\n%s", body, pf.evidence)
		}
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

// TestCompiledClaimsDedupedAndGroupedByAnchor: the page is deduped by
// statement, every claim of every chunk is kept, and the best-ranked anchor's
// claims all lead (closest to the question first) before the next anchor's.
func TestCompiledClaimsDedupedAndGroupedByAnchor(t *testing.T) {
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{
			{chunkID: "c2", name: "Acme employs 900 people", quote: "a workforce of 900"},
			{chunkID: "c1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"},
			{chunkID: "c1", name: "acme WAS founded in 1994", quote: "duplicate"},
			{chunkID: "c1", name: "Acme is based in Zurich", quote: "the Zurich-based firm"},
		}
	})
	claims := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, []string{"c1", "c2"}, "where was Acme founded and when", 0, 0).claims
	if len(claims) != 3 {
		t.Fatalf("claims = %d, want the 3 distinct anchored claims: %+v", len(claims), claims)
	}
	if claims[0].name != "Acme was founded in 1994" {
		t.Errorf("the best-ranked anchor must lead with its best-covering claim, got %q", claims[0].name)
	}
	if claims[1].chunkID != "c1" {
		t.Errorf("the best anchor's remaining claims must stay with it, got %q", claims[1].chunkID)
	}
	if claims[2].chunkID != "c2" {
		t.Errorf("the next anchor's claims must follow the best anchor's, got %q", claims[2].chunkID)
	}
}

// TestCompiledClaimsKeepDifferentlyWordedClaim: the anchor chunk is the
// relevance proof, so a claim that repeats none of the question's terms still
// shows up - a floor on question-token coverage would empty the page and report
// "the corpus has no compiled evidence" for a chunk that has it.
func TestCompiledClaimsKeepDifferentlyWordedClaim(t *testing.T) {
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{{chunkID: "c1", name: "Something unrelated entirely", quote: "x"}}
	})
	claims := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, []string{"c1"}, "when was Acme founded", 0, 0).claims
	if len(claims) != 1 {
		t.Fatalf("the anchored claim must survive regardless of wording, got %+v", claims)
	}
}

// TestCompiledClaimsBudgetKeepsSecondHopAnchor: the fact that answers a second
// hop is often compiled from a mid-ranked chunk, names the second-hop entity
// rather than the question's terms, and is not the best-covering claim of that
// chunk. The per-anchor floor is what keeps it on a budgeted page.
func TestCompiledClaimsBudgetKeepsSecondHopAnchor(t *testing.T) {
	anchors := []string{"c0", "c1", "c2", "c3", "c4", "c5"}
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		rows := make([]claimHit, 0, 42)
		// Eight claims on each of the five top anchors, so the budget binds and
		// the page is the retrieval's top of a long tail rather than everything
		// the store holds.
		for _, anchor := range anchors[:5] {
			for i := 0; i < 8; i++ {
				rows = append(rows, claimHit{
					chunkID: anchor,
					name:    fmt.Sprintf("The World Series between the Atlanta Braves and Houston Astros reached extra innings in game %d of the %s account", i+1, anchor),
					quote:   "the Braves beat the Astros in the World Series",
				})
			}
		}
		// The second-hop facts live on the LAST-ranked anchor, and the debut is
		// not its best-covering claim (the 1956 appearance ranks closer to
		// "last ... World Series"). Both fit the per-anchor floor, which is what
		// keeps them on a page whose budget the top anchors would otherwise eat.
		rows = append(rows,
			claimHit{
				chunkID: "c5",
				name:    "Jackie Robinson's last MLB appearance was on October 10, 1956, for the Brooklyn Dodgers.",
				quote:   "Last MLB appearance October 10, 1956, for the Brooklyn Dodgers",
			},
			claimHit{
				chunkID: "c5",
				name:    "Jackie Robinson made his MLB debut on April 15, 1947, for the Brooklyn Dodgers.",
				quote:   "MLB: April 15, 1947, for the Brooklyn Dodgers",
			},
		)
		return rows
	})
	page := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, anchors,
		"As of August 2024, the Atlanta Braves beat the Houston Astros the last time they won the World Series. How many years before this did Jackie Robinson join the Brooklyn Dodgers?",
		compiledPrefetchPageClaims, compiledPagePerAnchor)
	if len(page.claims) != compiledPrefetchPageClaims {
		t.Fatalf("page size = %d, want the budget %d (42 claims were available)", len(page.claims), compiledPrefetchPageClaims)
	}
	if page.dropped == 0 || len(page.omitted) == 0 {
		t.Fatalf("a truncated page must name what it left out: dropped=%d omitted=%v", page.dropped, page.omitted)
	}
	found := false
	for _, c := range page.claims {
		if c.chunkID == "c5" && strings.Contains(c.name, "April 15, 1947") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("the second-hop anchor #5 must get a slot, got %d claims from %v", len(page.claims), claimAnchorsOf(page.claims))
	}
}

// TestListChunksAlwaysServesThePassage: the deep read is a deep read, on a
// claim-carrying corpus as well. Withholding the passage until the caller asks
// again cost a round trip in 63 of 64 measured calls (FRAMES tree corpus), so
// compiled statements travel on the locate notice instead.
func TestListChunksAlwaysServesThePassage(t *testing.T) {
	fe := &grepFakeEngine{
		regexpChunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "prose one", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(0)},
		},
	}
	runtime.SetGrepService(NewGrepAdapter(fe))
	defer runtime.SetGrepService(nil)
	stubOwner(t, "kb1")

	// The corpus DOES carry claims for this chunk: the read must still return
	// its text on the first call.
	installClaimProbe(t, true)
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{{chunkID: "c1", docID: "d1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"}}
	})
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	ctx = WithQuery(ctx, "when was Acme founded")
	ctx = withCompiledRun(ctx, "t", []string{"kb1"})

	tool := NewListChunksTool("t", []string{"kb1"})
	out, err := tool.InvokableRun(ctx, `{"doc_id":"d1","anchor_chunk_ids":["c1"],"intent":"founding year of Acme"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "prose one") {
		t.Fatalf("the read must return the passage on the first call, got:\n%s", out)
	}
	if strings.Contains(out, "<compiled_evidence") {
		t.Fatalf("the read must not be replaced by compiled statements, got:\n%s", out)
	}
}

// claimAnchorsOf lists the anchors a page was spent on, for failure messages.
func claimAnchorsOf(claims []claimHit) []string {
	out := make([]string, 0, len(claims))
	for _, c := range claims {
		out = append(out, c.chunkID)
	}
	return out
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
	claims := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, []string{"c1", "c2"}, "when was Acme founded", 0, 0).claims
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
	claims := compiledClaimsForChunks(context.Background(), "tenant-1", []string{"kb1"}, []string{"c1"}, "when was Acme founded", 0, 0).claims
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
// time, and a later search that returns the same claims does not pay for their
// text again - it names the handles the turn already holds, so the model neither
// re-reads the statements nor deep-reads the chunks to see what they said.
func TestCompiledEvidenceNoticeShownOnce(t *testing.T) {
	installClaimProbe(t, true)
	installClaimLookup(t, func(_ context.Context, _ string, _ []string, chunkIDs, _ []string, _ int) []claimHit {
		return []claimHit{{chunkID: chunkIDs[0], docID: "d1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"}}
	})
	ctx := withCompiledRun(WithQuery(context.Background(), "when was Acme founded"), "tenant-1", []string{"kb1"})
	ctx = WithEvidenceRegistry(ctx, NewEvidenceRegistry())

	first := compiledEvidenceNotice(ctx, searchChunksToolName, "acme", noticeHits("c1", "c2"))
	if !strings.Contains(first, "<compiled_evidence count=\"1\">") || !strings.Contains(first, "- [claim] Acme was founded in 1994") {
		t.Fatalf("first result must carry the compiled claim, got:\n%s", first)
	}
	if !strings.Contains(first, "do NOT call list_chunks") {
		t.Errorf("the block must tell the model it can answer without a deep read, got:\n%s", first)
	}
	second := compiledEvidenceNotice(ctx, searchChunksToolName, "acme", noticeHits("c1", "c2"))
	if strings.Contains(second, "- [claim] Acme was founded in 1994") {
		t.Errorf("an already shown statement must not be repeated, got:\n%s", second)
	}
	if !strings.Contains(second, "<already_shown>") || !strings.Contains(second, "c1=[ID:0]") {
		t.Errorf("a repeat result must name the handles the turn already holds, got:\n%s", second)
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
		if out := compiledEvidenceNotice(ctx, searchChunksToolName, "acme", noticeHits("c1")); out != "" {
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
	if out := compiledEvidenceNotice(context.Background(), searchChunksToolName, "acme", noticeHits("c1")); out != "" {
		t.Errorf("no run scope must yield no block, got:\n%s", out)
	}
	web := []snippetHit{{chunk: runtime.RetrievalChunk{ID: "web-1", URL: "https://example.test"}, web: true}}
	if out := compiledEvidenceNotice(context.Background(), webSearchToolName, "acme", web); out != "" {
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

// TestCompiledEvidenceNoticeCoversTheResultSet: a locate result's block is what
// keeps deep reads out of the turn, so its budget is spent ONE claim per hit
// across as many hits as it can afford - not two-deep on the top three, which is
// the shape that left 19 of every 22 hits without a statement.
func TestCompiledEvidenceNoticeCoversTheResultSet(t *testing.T) {
	installClaimProbe(t, true)
	installClaimLookup(t, func(_ context.Context, _ string, _ []string, chunkIDs, _ []string, _ int) []claimHit {
		out := make([]claimHit, 0, len(chunkIDs)*3)
		for _, id := range chunkIDs {
			for i := 0; i < 3; i++ {
				out = append(out, claimHit{
					chunkID: id,
					docID:   "doc-" + id,
					name:    fmt.Sprintf("%s states fact %d", id, i),
					quote:   fmt.Sprintf("quote from %s", id),
				})
			}
		}
		return out
	})
	ctx := withCompiledRun(WithQuery(context.Background(), "when was Acme founded"), "tenant-1", []string{"kb1"})
	ctx = WithEvidenceRegistry(ctx, NewEvidenceRegistry())
	hits := noticeHits("c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8")

	out := compiledEvidenceNotice(ctx, searchChunksToolName, "acme", hits)
	if out == "" {
		t.Fatal("the notice must be rendered for a result set with claims")
	}
	// The block is budget-bound, and the budget is spent on COVERAGE: one claim
	// per hit first, then the remainder deepening the top hits. Two-deep on the
	// top three is the shape that left 19 of every 22 hits without a statement.
	if got := strings.Count(out, "- [claim] "); got != compiledNoticeClaims {
		t.Errorf("the block must spend its whole budget of %d claims, got %d lines:\n%s",
			compiledNoticeClaims, got, out)
	}
	for _, h := range hits {
		if !strings.Contains(out, h.chunk.ID+" states fact ") {
			t.Errorf("every hit must carry at least one statement, %s has none:\n%s", h.chunk.ID, out)
		}
	}
	// The coverage it actually achieved is stated, so the model knows which
	// chunks still have to be read instead of re-reading the covered ones.
	if !strings.Contains(out, fmt.Sprintf("compiled artifacts of %d of the %d chunks", len(hits), len(hits))) {
		t.Errorf("the note must report the coverage it achieved, got:\n%s", out)
	}
}

// TestCompiledEvidenceNoticeRanksOnThisCall: the anchors keep the retrieval's
// own order - that ranking IS this call's decision - but WHICH claim of a chunk
// leads is decided against this call's query, not the turn's question. Ranking on
// the question would lead a hop-2 search's chunk with the hop-1 statement an
// earlier notice already showed, and takeUnseen would then drop the whole page.
func TestCompiledEvidenceNoticeRanksOnThisCall(t *testing.T) {
	installClaimProbe(t, true)
	installClaimLookup(t, func(_ context.Context, _ string, _ []string, _ []string, _ []string, _ int) []claimHit {
		return []claimHit{
			{chunkID: "c1", docID: "d1", name: "Acme was founded in 1994", quote: "founded in 1994"},
			{chunkID: "c1", docID: "d1", name: "Acme Trucking Ltd was acquired by Zeta Holdings", quote: "acquired by Zeta Holdings"},
		}
	})
	ctx := withCompiledRun(WithQuery(context.Background(), "when was Acme founded"), "tenant-1", []string{"kb1"})
	ctx = WithEvidenceRegistry(ctx, NewEvidenceRegistry())

	out := compiledEvidenceNotice(ctx, searchBm25ChunksToolName, "who acquired Acme Trucking Ltd", noticeHits("c1"))
	zetaAt := strings.Index(out, "Zeta Holdings")
	foundedAt := strings.Index(out, "founded in 1994")
	if zetaAt < 0 || foundedAt < 0 {
		t.Fatalf("both claims belong to the hit and must be offered, got:\n%s", out)
	}
	if zetaAt > foundedAt {
		t.Errorf("the claim matching THIS call must lead, got:\n%s", out)
	}
}

// TestPrefetchKeepsEveryClaimOfAnAnchor: the page is the retrieved chunks' own
// claims, so a chunk that states twenty facts contributes twenty rows. Capping a
// chunk at its top-ranked claims would decide, before the verdict, which of its
// statements count as evidence.
func TestPrefetchKeepsEveryClaimOfAnAnchor(t *testing.T) {
	installClaimProbe(t, true)
	installAnchorSearch(t, func(context.Context, string, []string, []string, string, int) []runtime.RetrievalChunk {
		return chunkPage("c1")
	})
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		rows := make([]claimHit, 0, 20)
		for i := 0; i < 20; i++ {
			rows = append(rows, claimHit{
				chunkID: "c1", docID: "d1",
				name:  fmt.Sprintf("Leonardo completed work %d", i+1),
				quote: fmt.Sprintf("work %d", i+1),
			})
		}
		return rows
	})
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())

	pf := prefetchCompiledEvidence(ctx, "tenant-1", []string{"kb1"}, questionMessages("what did Leonardo work on"))
	if len(pf.claims) != 20 {
		t.Fatalf("claims = %d, want every claim of the anchor (20)", len(pf.claims))
	}
	if got := strings.Count(pf.evidence, "[claim]"); got != 20 {
		t.Errorf("evidence lines = %d, want 20", got)
	}
}

// TestPrefetchServesClaimlessChunkAsText: an anchor that carries no compiled
// claims is on the page as its own text. Leaving it out would ask the verdict
// whether the evidence answers the question while withholding one of the
// retrieved chunks.
func TestPrefetchServesClaimlessChunkAsText(t *testing.T) {
	installClaimProbe(t, true)
	installAnchorSearch(t, func(context.Context, string, []string, []string, string, int) []runtime.RetrievalChunk {
		return chunkPage("c1", "c2")
	})
	installClaimLookup(t, func(context.Context, string, []string, []string, []string, int) []claimHit {
		return []claimHit{{chunkID: "c1", docID: "d1", name: "Acme was founded in 1994", quote: "founded in 1994"}}
	})
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())

	pf := prefetchCompiledEvidence(ctx, "tenant-1", []string{"kb1"}, questionMessages("when was Acme founded"))
	if len(pf.claims) != 1 {
		t.Fatalf("claims = %d, want the one claim c1 carries", len(pf.claims))
	}
	if !strings.Contains(pf.evidence, "Acme was founded in 1994") {
		t.Errorf("claimed chunk must be served as its statements:\n%s", pf.evidence)
	}
	if !strings.Contains(pf.evidence, "body of c2") {
		t.Errorf("claim-less chunk must be served as its text:\n%s", pf.evidence)
	}
}

// TestClaimGapOverlap: the diagnostic must separate "the page had it" from "the
// page never had it", because those two refusals need different work.
func TestClaimGapOverlap(t *testing.T) {
	claims := []claimHit{{name: "The locomotive's top speed was 96 km/h", quote: "top speed 96 km/h"}}
	if got := claimGapOverlap(claims, "the top speed of the locomotive"); got < 0.5 {
		t.Errorf("a gap the page states must score high, got %.2f", got)
	}
	if got := claimGapOverlap(claims, "the boiler pressure of the locomotive"); got >= 0.5 {
		t.Errorf("a gap the page never states must score low, got %.2f", got)
	}
	if got := claimGapOverlap(claims, ""); got != 0 {
		t.Errorf("an empty gap must score 0, got %.2f", got)
	}
}
