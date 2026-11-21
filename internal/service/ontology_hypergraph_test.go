package service

import (
	"context"
	"testing"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/service/nlp"
)

// ---- fixtures ----

// ontologyHypernodeRow builds one hypernode row the way the compiler writes it:
// both halves carry path, value and membership, and differ only by role.
func ontologyHypernodeRow(id, role, path, prop, value string, edges []interface{}, score float64) map[string]interface{} {
	return map[string]interface{}{
		"id":               id,
		"path_kwd":         path,
		"prop_kwd":         prop,
		"node_role_kwd":    role,
		"node_value_kwd":   value,
		"hyperedge_ids":    edges,
		"depth_int":        float64(0),
		"source_chunk_ids": []interface{}{"c-" + id},
		"doc_id":           "doc-1",
		"similarity":       score,
	}
}

// ontologyHyperedgeRow builds one factual block row.
func ontologyHyperedgeRow(id, payload string, chunks ...string) map[string]interface{} {
	ids := make([]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		ids = append(ids, chunk)
	}
	return map[string]interface{}{
		"id":                  id,
		"content_with_weight": payload,
		"name_kwd":            "ada",
		"entity_type_kwd":     "person",
		"depth_int":           float64(1),
		"source_chunk_ids":    ids,
		"doc_id":              "doc-1",
	}
}

// fakeOntologyRowReader answers the two reads the assembly stages issue, keyed on
// the filter that distinguishes them.
type fakeOntologyRowReader struct {
	edges   map[string]map[string]interface{}
	names   []map[string]interface{}
	filters []map[string]interface{}
	err     error
}

func (f *fakeOntologyRowReader) read(_ context.Context, _ string, _ []string, filter map[string]interface{}, _ []string, _ int) ([]map[string]interface{}, error) {
	f.filters = append(f.filters, filter)
	if f.err != nil {
		return nil, f.err
	}
	if _, byName := filter["node_value_kwd"]; byName {
		return f.names, nil
	}
	if ids, ok := filter["id"].([]string); ok {
		out := make([]map[string]interface{}, 0, len(ids))
		for _, id := range ids {
			if row, ok := f.edges[id]; ok {
				out = append(out, row)
			}
		}
		return out, nil
	}
	return nil, nil
}

// fakeOntologyChunkReader loads the original chunks a block points at.
type fakeOntologyChunkReader struct {
	rows map[string]map[string]interface{}
	ids  []string
}

func (f *fakeOntologyChunkReader) read(_ context.Context, _ string, _ []string, ids []string) []map[string]interface{} {
	f.ids = ids
	out := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		if row, ok := f.rows[id]; ok {
			out = append(out, row)
		}
	}
	return out
}

// newHypergraphTestService wires the hypergraph seams on top of the existing
// ontology test service.
func newHypergraphTestService(
	rows *fakeOntologyRowReader,
	chunks *fakeOntologyChunkReader,
	results ...*nlp.RetrievalResult,
) (*ontologyTestService, *fakeOntologyRowReader, *fakeOntologyChunkReader) {
	base := newOntologyTestService(guildScope(), nil, nil, results...)
	base.svc.rowReader = rows.read
	if chunks != nil {
		base.svc.chunkReader = chunks.read
	}
	return base, rows, chunks
}

// ---- S4: dedupe and merge ----

// A slot recalled by both legs is ONE slot at its best score, and its membership
// is the union of what the two rows reported: counting the two halves separately
// would double every slot and make coverage unreachable.
func TestMergeOntologyHypernodeHitsKeepsOneSlotAtBestScore(t *testing.T) {
	keyHits := []ontologyHypernodeHit{{
		nodeKey: "person:name\x00ada", path: "person:name", slot: "person:name",
		value: "ada", role: "key", score: 0.4, edgeIDs: []string{"e1"},
	}}
	valueHits := []ontologyHypernodeHit{{
		nodeKey: "person:name\x00ada", path: "person:name", slot: "person:name",
		value: "ada", role: "value", score: 0.9, edgeIDs: []string{"e2"},
	}}
	hits := mergeOntologyHypernodeHits(keyHits, valueHits)
	if len(hits) != 1 {
		t.Fatalf("one slot produced %d hits, want 1", len(hits))
	}
	if hits[0].score != 0.9 {
		t.Errorf("score = %v, want the best of the two legs (0.9)", hits[0].score)
	}
	if len(hits[0].edgeIDs) != 2 {
		t.Errorf("membership = %v, want the union of both rows", hits[0].edgeIDs)
	}
}

// A key row whose value could not be read is identified by its own row id: it is
// a slot that cannot be joined, not a slot equal to every other key half of the
// same path.
func TestOntologyHypernodeKeyFallsBackToRowID(t *testing.T) {
	if got := ontologyHypernodeKey("person:name", "ada", "r1"); got != "person:name\x00ada" {
		t.Fatalf("key = %q", got)
	}
	if got := ontologyHypernodeKey("person:name", "", "r1"); got != "row:r1" {
		t.Fatalf("key without a value = %q, want row:r1", got)
	}
}

// slot_coverage counts (class, attribute) pairs, so the pair is read off the
// path's last segment: the same attribute name on two classes is two slots.
func TestOntologyHypergraphSlotIsClassAndAttribute(t *testing.T) {
	for path, want := range map[string]string{
		"person:name":                          "person:name",
		"person:born_in.place:name":            "place:name",
		"person:born_in.place:has_part.x:name": "x:name",
	} {
		if got := ontologyHypergraphSlot(path); got != want {
			t.Errorf("slot(%q) = %q, want %q", path, got, want)
		}
	}
}

// ---- S5: greedy coverage selection ----

// The block that explains more of the recalled slots wins, and every selected
// block reports how many slots it contributed — that number is what makes the
// selection explainable after the fact.
func TestSelectOntologyHyperedgeBlocksPrefersCoverage(t *testing.T) {
	hits := []ontologyHypernodeHit{
		{nodeKey: "n1", depth: 1, score: 0.9, edgeIDs: []string{"narrow"}},
		{nodeKey: "n2", depth: 1, score: 0.5, edgeIDs: []string{"wide"}},
		{nodeKey: "n3", depth: 1, score: 0.5, edgeIDs: []string{"wide"}},
	}
	selection := selectOntologyHyperedgeBlocks(hits, 4)
	if len(selection.edgeIDs) != 2 {
		t.Fatalf("selected %v, want both blocks", selection.edgeIDs)
	}
	if selection.edgeIDs[0] != "wide" {
		t.Fatalf("first block = %q, want the one covering two slots", selection.edgeIDs[0])
	}
	if selection.contributions["wide"] != 2 || selection.contributions["narrow"] != 1 {
		t.Fatalf("contributions = %v, want wide:2 narrow:1", selection.contributions)
	}
	if len(selection.covered) != 3 {
		t.Fatalf("covered = %v, want every slot", selection.covered)
	}
}

// Ties are broken by score, then depth, then row id — never by map order. Two
// runs over the same rows must assemble the same blocks, or their token counts
// cannot be compared.
func TestSelectOntologyHyperedgeBlocksTieBreakIsDeterministic(t *testing.T) {
	tie := func(edges ...string) []ontologyHypernodeHit {
		hits := make([]ontologyHypernodeHit, 0, len(edges))
		for _, edge := range edges {
			hits = append(hits, ontologyHypernodeHit{nodeKey: "n" + edge, score: 0.5, depth: 2, edgeIDs: []string{edge}})
		}
		return hits
	}
	first := selectOntologyHyperedgeBlocks(tie("b", "a"), 1)
	second := selectOntologyHyperedgeBlocks(tie("a", "b"), 1)
	if first.edgeIDs[0] != second.edgeIDs[0] {
		t.Fatalf("selection depends on input order: %q vs %q", first.edgeIDs[0], second.edgeIDs[0])
	}
	if first.edgeIDs[0] != "a" {
		t.Fatalf("lexicographic tie-break picked %q, want a", first.edgeIDs[0])
	}
	// A higher score wins before the id does.
	hits := []ontologyHypernodeHit{
		{nodeKey: "n1", score: 0.9, edgeIDs: []string{"z"}},
		{nodeKey: "n2", score: 0.4, edgeIDs: []string{"a"}},
	}
	if got := selectOntologyHyperedgeBlocks(hits, 1).edgeIDs[0]; got != "z" {
		t.Fatalf("score tie-break picked %q, want z", got)
	}
}

// The number of blocks assembled is capped; what is left out is still covered by
// the coverage numbers, which is the point of reporting them.
func TestSelectOntologyHyperedgeBlocksRespectsCap(t *testing.T) {
	hits := []ontologyHypernodeHit{
		{nodeKey: "n1", score: 0.9, edgeIDs: []string{"e1"}},
		{nodeKey: "n2", score: 0.8, edgeIDs: []string{"e2"}},
		{nodeKey: "n3", score: 0.7, edgeIDs: []string{"e3"}},
	}
	selection := selectOntologyHyperedgeBlocks(hits, 2)
	if len(selection.edgeIDs) != 2 {
		t.Fatalf("selected %d blocks, want 2", len(selection.edgeIDs))
	}
	if len(selection.covered) != 2 {
		t.Fatalf("covered = %d slots, want 2 of 3", len(selection.covered))
	}
}

// A recalled slot with no membership is an orphan: it must be counted, and the
// selection must still work on the slots that do have blocks.
func TestSelectOntologyHyperedgeBlocksCountsOrphans(t *testing.T) {
	hits := []ontologyHypernodeHit{
		{nodeKey: "n1", score: 0.9, edgeIDs: []string{"e1"}},
		{nodeKey: "n2", score: 0.8},
	}
	selection := selectOntologyHyperedgeBlocks(hits, 4)
	if selection.orphanHits != 1 {
		t.Fatalf("orphanHits = %d, want 1", selection.orphanHits)
	}
	if len(selection.edgeIDs) != 1 {
		t.Fatalf("selected %v, want only the block that exists", selection.edgeIDs)
	}
}

// ---- S8: coverage ----

// Sufficiency is a coverage threshold AND the absence of orphans: enough slots
// covered over a slot set that was itself complete.
func TestOntologyHypergraphCoverageGatesOnCoverageAndOrphans(t *testing.T) {
	hits := []ontologyHypernodeHit{
		{nodeKey: "n1", slot: "person:name"},
		{nodeKey: "n2", slot: "person:born_in"},
		{nodeKey: "n3", slot: "person:age"},
		{nodeKey: "n4", slot: "person:name"},
	}
	selection := ontologyHyperedgeSelection{covered: map[string]bool{"n1": true, "n2": true, "n4": true}}

	cov := ontologyHypergraphCoverage(hits, selection, false)
	if cov.HitNodes != 4 || cov.CoveredNodes != 3 {
		t.Fatalf("coverage = %d/%d, want 3/4", cov.CoveredNodes, cov.HitNodes)
	}
	if cov.NodeCoverage != 0.75 {
		t.Errorf("node_coverage = %v, want 0.75", cov.NodeCoverage)
	}
	// Two of three distinct (class, attribute) pairs are covered.
	if cov.SlotCoverage != 2.0/3.0 {
		t.Errorf("slot_coverage = %v, want 2/3", cov.SlotCoverage)
	}
	if !cov.Sufficient {
		t.Error("sufficient = false, want true at 0.75 with no orphans")
	}

	selection.orphanHits = 1
	if ontologyHypergraphCoverage(hits, selection, false).Sufficient {
		t.Error("sufficient = true with an orphan slot; an unassembled slot must not read as complete")
	}
	if !ontologyHypergraphCoverage(hits, selection, true).Partial {
		t.Error("partial = false after a failed leg")
	}
}

// ---- end to end through Retrieve ----

// With the switch on, the digest carries the assembled blocks, the original
// chunks behind them and the coverage they explain; the assertion path and its
// filter are untouched.
func TestOntologyABoxRetrievalAssemblesHypergraphBlocks(t *testing.T) {
	rows := &fakeOntologyRowReader{edges: map[string]map[string]interface{}{
		"e1": ontologyHyperedgeRow("e1", `{"person name": "ada"}`, "c1"),
		"e2": ontologyHyperedgeRow("e2", `{"organization name": "acme"}`, "c2"),
	}}
	chunks := &fakeOntologyChunkReader{rows: map[string]map[string]interface{}{
		"c1": {"chunk_id": "c1", "doc_id": "doc-1", "docnm_kwd": "ada.md", "content_with_weight": "Ada led the guild."},
		"c2": {"chunk_id": "c2", "doc_id": "doc-1", "docnm_kwd": "acme.md", "content_with_weight": "Acme was chartered."},
	}}
	base, reader, _ := newHypergraphTestService(rows, chunks,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{
			ontologyHypernodeRow("k1", "key", "person:name", "name", "ada", []interface{}{"e1"}, 0.8),
		}, Total: 1},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{
			ontologyHypernodeRow("v1", "value", "person:name", "name", "ada", []interface{}{"e1"}, 0.7),
			ontologyHypernodeRow("v2", "value", "organization:name", "name", "acme", []interface{}{"e2"}, 0.6),
		}, Total: 2},
	)

	req := validOntologyABoxRequest()
	req.HypergraphRecall = true
	result, err := base.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	hg := result.Digest.Hypergraph
	if hg == nil {
		t.Fatal("digest carries no hypergraph section")
	}
	if len(hg.Edges) != 2 {
		t.Fatalf("blocks = %d, want 2", len(hg.Edges))
	}
	if hg.Edges[0].RowID != "e1" {
		t.Errorf("first block = %q, want the higher-scoring e1", hg.Edges[0].RowID)
	}
	if len(hg.Edges[0].Facts) != 1 || hg.Edges[0].Facts[0].Path != "person name" {
		t.Errorf("block facts = %v, want the rendered payload", hg.Edges[0].Facts)
	}
	if len(hg.Chunks) != 2 || hg.Chunks[0].ChunkID != "c1" {
		t.Errorf("chunks = %v, want c1 then c2", hg.Chunks)
	}
	if hg.Coverage.HitNodes != 2 || hg.Coverage.CoveredNodes != 2 {
		t.Errorf("coverage = %d/%d, want 2/2 — the key and value halves of one slot are ONE slot",
			hg.Coverage.CoveredNodes, hg.Coverage.HitNodes)
	}
	if !hg.Coverage.Sufficient {
		t.Error("sufficient = false, want true")
	}
	if hg.Degrade != nil {
		t.Errorf("degrade = %v, want none", hg.Degrade)
	}
	// The legs are the only extra searches: promotion and assembly go through the
	// row reader, not through retrieval.
	if got := base.searcher.callCount(); got != 3 {
		t.Errorf("retrieval calls = %d, want 3 (assertions + two hypernode legs)", got)
	}
	if len(reader.filters) == 0 {
		t.Error("assembly read no rows")
	}
}

// With the switch off nothing extra is read and the digest carries no section:
// the assertion-only path has to be byte-for-byte what it was, or the A/B is a
// comparison of two different things.
func TestOntologyABoxRetrievalHypergraphOffChangesNothing(t *testing.T) {
	base, reader, _ := newHypergraphTestService(&fakeOntologyRowReader{}, nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	result, err := base.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Digest.Hypergraph != nil {
		t.Error("digest carries a hypergraph section although the switch is off")
	}
	if got := base.searcher.callCount(); got != 1 {
		t.Errorf("retrieval calls = %d, want 1", got)
	}
	if len(reader.filters) != 0 {
		t.Errorf("row reads = %d, want none", len(reader.filters))
	}
}

// No slot recalled is a degraded section, not an error: the assertion rows the
// caller already has must still answer.
func TestOntologyABoxRetrievalHypergraphAbsentIsDegraded(t *testing.T) {
	base, _, _ := newHypergraphTestService(&fakeOntologyRowReader{}, nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1},
		&nlp.RetrievalResult{},
		&nlp.RetrievalResult{},
	)
	req := validOntologyABoxRequest()
	req.HypergraphRecall = true
	result, err := base.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	hg := result.Digest.Hypergraph
	if hg == nil || hg.Degrade == nil {
		t.Fatalf("hypergraph = %+v, want a degraded section", hg)
	}
	if hg.Degrade.Code != agentic_rag.OntologyDegradeHypergraphAbsent {
		t.Errorf("degrade = %q, want %q", hg.Degrade.Code, agentic_rag.OntologyDegradeHypergraphAbsent)
	}
	if len(result.Digest.Assertions) != 1 {
		t.Error("the assertion rows must survive an empty hypergraph section")
	}
}

// A failed leg narrows the slot set rather than ending the section, and the
// coverage says so: numbers over an incomplete recall must not read as complete.
func TestOntologyABoxRetrievalHypergraphLegFailureIsPartial(t *testing.T) {
	rows := &fakeOntologyRowReader{edges: map[string]map[string]interface{}{
		"e1": ontologyHyperedgeRow("e1", `{"person name": "ada"}`, "c1"),
	}}
	base, _, _ := newHypergraphTestService(rows, nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{
			ontologyHypernodeRow("k1", "key", "person:name", "name", "ada", []interface{}{"e1"}, 0.8),
		}, Total: 1},
	)
	// Fail the value leg only: the assertion search and the key leg succeed.
	base.svc.searcher = &failingLegSearcher{inner: base.searcher, failAt: 2}

	req := validOntologyABoxRequest()
	req.HypergraphRecall = true
	result, err := base.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	hg := result.Digest.Hypergraph
	if hg == nil {
		t.Fatal("no hypergraph section")
	}
	if !hg.Coverage.Partial {
		t.Error("partial = false after a leg failed")
	}
	if len(hg.Edges) != 1 {
		t.Errorf("blocks = %d, want the one block the working leg found", len(hg.Edges))
	}
}

// failingLegSearcher fails one call and delegates the rest.
type failingLegSearcher struct {
	inner  *fakeOntologySearcher
	failAt int
}

func (f *failingLegSearcher) Retrieval(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error) {
	if len(f.inner.calls) == f.failAt {
		f.inner.calls = append(f.inner.calls, req)
		return nil, context.DeadlineExceeded
	}
	return f.inner.Retrieval(ctx, req)
}

// ---- assertion narrowing ----

// A relation whose two endpoints both appear inside one block is already stated
// by that block; listing it again spends the assertion budget on a duplicate.
// A relation spread across two blocks is NOT a duplicate and stays.
func TestApplyOntologyHypergraphDropsRelationStatedInOneBlock(t *testing.T) {
	digest := &agentic_rag.OntologyDigest{
		Assertions: []agentic_rag.OntologyAssertion{
			{Kind: "relation", From: "Ada", To: "Acme"},
			{Kind: "relation", From: "Ada", To: "London"},
			{Kind: "entity", Name: "Ada"},
		},
		UnreviewedCount: 1,
	}
	hg := &agentic_rag.OntologyHypergraph{Edges: []agentic_rag.OntologyHyperedge{{
		Facts: []agentic_rag.OntologyHypergraphFact{
			{Path: "person:name", Value: "Ada"},
			{Path: "organization:name", Value: "Acme"},
		},
	}}}

	applyOntologyHypergraph(digest, hg, nil)
	if len(digest.Assertions) != 2 {
		t.Fatalf("assertions = %d, want 2 (the Ada->Acme duplicate is dropped)", len(digest.Assertions))
	}
	if digest.Assertions[0].To != "London" {
		t.Errorf("kept relation = %q, want the one no block states", digest.Assertions[0].To)
	}
	if digest.Hypergraph != hg {
		t.Error("the section was not attached")
	}
	// The counts are recomputed over what is left, not decremented by hand.
	if digest.UnreviewedCount != 0 {
		t.Errorf("unreviewed = %d, want 0 after the pending row was dropped", digest.UnreviewedCount)
	}
}
