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

package harness

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// stubCompiledStore is an in-memory CompiledStore so the expander logic can be
// exercised in the unit tier. It behaves like the doc store:
//
//   - rows carry knowledge_graph_kwd / compile_kwd / available_int / from_entity_kwd
//     / to_entity_kwd / name_kwd / content_with_weight / source_chunk_ids / doc_id.
//   - filters are AND-ed, each value an OR-set (a row matches if its field's
//     string form equals a member).
//   - matchText selects rows whose name/content contains it (stand-in for the
//     dense+keyword retrieval the store normally does).
type stubCompiledStore struct {
	rows   []map[string]any
	chunks map[string]string
	// loadErrByChunk makes LoadChunks fail when one of the requested ids is
	// listed, standing in for a per-doc store error.
	loadErrByChunk map[string]error
}

func (s *stubCompiledStore) SearchCompiled(_ context.Context, _, _ string, _ []string, filters map[string][]string, matchText string, topN int) ([]map[string]any, error) {
	var out []map[string]any
	for _, r := range s.rows {
		if !rowMatchesFilters(r, filters) {
			continue
		}
		if matchText != "" {
			hay := strings.ToLower(asString(r["name_kwd"]) + " " + asString(r["content_with_weight"]))
			if !strings.Contains(hay, strings.ToLower(matchText)) {
				continue
			}
		}
		out = append(out, r)
		if topN > 0 && len(out) >= topN {
			break
		}
	}
	return out, nil
}

func (s *stubCompiledStore) LoadChunks(_ context.Context, _, _ string, chunkIDs []string) ([]map[string]any, error) {
	for _, id := range chunkIDs {
		if err, ok := s.loadErrByChunk[id]; ok {
			return nil, err
		}
	}
	var out []map[string]any
	for _, id := range chunkIDs {
		if content, ok := s.chunks[id]; ok {
			out = append(out, map[string]any{"id": id, "content": content, "doc_id": "d1"})
		}
	}
	return out, nil
}

func (s *stubCompiledStore) Vectorize(_ context.Context, _ string) ([]float64, error) {
	return nil, nil
}

// scopeRecordingStore records the (kb, tenant, docs) triple of every
// SearchCompiled call and returns no rows, so only scope resolution is under
// test.
type scopeRecordingStore struct {
	calls []scopeCall
}

type scopeCall struct {
	kb, tenant string
	docs       []string
}

func (s *scopeRecordingStore) SearchCompiled(_ context.Context, kbID, tenantID string, docIDs []string, _ map[string][]string, _ string, _ int) ([]map[string]any, error) {
	s.calls = append(s.calls, scopeCall{kb: kbID, tenant: tenantID, docs: append([]string(nil), docIDs...)})
	return nil, nil
}

func (s *scopeRecordingStore) LoadChunks(context.Context, string, string, []string) ([]map[string]any, error) {
	return nil, nil
}

func (s *scopeRecordingStore) Vectorize(context.Context, string) ([]float64, error) {
	return nil, nil
}

// ownerRecordingStore wraps stubCompiledStore to record the owner each
// LoadChunks call is issued against.
type ownerRecordingStore struct {
	*stubCompiledStore
	loads []scopeCall
}

func (s *ownerRecordingStore) LoadChunks(ctx context.Context, kbID, tenantID string, chunkIDs []string) ([]map[string]any, error) {
	s.loads = append(s.loads, scopeCall{kb: kbID, tenant: tenantID, docs: append([]string(nil), chunkIDs...)})
	return s.stubCompiledStore.LoadChunks(ctx, kbID, tenantID, chunkIDs)
}

// TestCompiledExpanderLoadsFromEachDocsOwnerAndSkipsUnresolvable mirrors Python
// _load_chunks_for_doc: a neighbour row's source chunks are fetched from THAT
// row's doc owner (not the searched scope), and a row whose doc id cannot be
// resolved (empty / merged pseudo doc) contributes nothing.
func TestCompiledExpanderLoadsFromEachDocsOwnerAndSkipsUnresolvable(t *testing.T) {
	store := &ownerRecordingStore{stubCompiledStore: &stubCompiledStore{
		rows: []map[string]any{
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "Culdcept", "content_with_weight": `{"name":"Culdcept"}`, "doc_id": "seedDoc"},
			{"knowledge_graph_kwd": "relation", "compilation_template_kind_kwd": "knowledge_graph",
				"from_entity_kwd": "Culdcept", "to_entity_kwd": "OmiyaSoft", "doc_id": "seedDoc"},
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "OmiyaSoft", "content_with_weight": `{"name":"OmiyaSoft"}`,
				"source_chunk_ids": []string{"s1"}, "doc_id": "docA"},
			// No doc id -> Python's _resolve_doc_tenant fails, so its source
			// chunks are dropped.
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "OmiyaSoft", "content_with_weight": `{"name":"OmiyaSoft"}`,
				"source_chunk_ids": []string{"s2"}, "doc_id": ""},
		},
		chunks: map[string]string{"s1": "a", "s2": "b"},
	}}
	exp := NewCompiledExpander(store, CompiledScopeConfig{
		DatasetIDs:        []string{"kb1"},
		TenantID:          "t1",
		KBs:               []*entity.Knowledgebase{{ID: "kb1", TenantID: "t1"}},
		DocTenantResolver: stubTenantResolver{owner: DocTenant{KBID: "kbOwner", TenantID: "tOwner"}},
	}).(*compiledExpander)

	kb := &Kbinfos{}
	if err := exp.Expand(context.Background(), kb, "culdcept", "", nil); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(store.loads) != 1 {
		t.Fatalf("LoadChunks calls = %d (%+v), want 1 (the unresolvable doc must be skipped)", len(store.loads), store.loads)
	}
	if store.loads[0].kb != "kbOwner" || store.loads[0].tenant != "tOwner" {
		t.Errorf("load owner = %+v, want the doc's own owner kbOwner/tOwner", store.loads[0])
	}
	got := compiledIDs(kb.Chunks)
	if len(got) != 1 || got[0] != "s1" {
		t.Errorf("expanded = %v, want only s1", got)
	}
}

// TestNewCompiledExpanderGuard pins the fail-closed cases: a nil store, no bound
// dataset, or no fallback tenant leaves expansion disabled rather than searching
// an unscoped/empty index.
func TestNewCompiledExpanderGuard(t *testing.T) {
	store := &scopeRecordingStore{}
	if exp := NewCompiledExpander(nil, CompiledScopeConfig{DatasetIDs: []string{"kb1"}, TenantID: "t1"}); exp != nil {
		t.Error("nil store must disable expansion")
	}
	if exp := NewCompiledExpander(store, CompiledScopeConfig{TenantID: "t1"}); exp != nil {
		t.Error("no dataset must disable expansion")
	}
	if exp := NewCompiledExpander(store, CompiledScopeConfig{DatasetIDs: []string{"kb1"}}); exp != nil {
		t.Error("no tenant must disable expansion")
	}
	if exp := NewCompiledExpander(store, CompiledScopeConfig{DatasetIDs: []string{"kb1"}, TenantID: "t1"}); exp == nil {
		t.Error("store + dataset + tenant must enable expansion")
	}
}

// TestCompiledExpanderScansEachDatasetUnderItsOwnTenant mirrors Python
// `[(kb.id, kb.tenant_id, None) for kb in tools.kbs]`: with no doc scope each
// bound dataset is scanned under ITS OWN owner tenant, not the request tenant.
func TestCompiledExpanderScansEachDatasetUnderItsOwnTenant(t *testing.T) {
	store := &scopeRecordingStore{}
	exp := NewCompiledExpander(store, CompiledScopeConfig{
		DatasetIDs: []string{"kb1", "kb2"},
		TenantID:   "request-tenant",
		KBs: []*entity.Knowledgebase{
			{ID: "kb1", TenantID: "t1"},
			{ID: "kb2", TenantID: "t2"},
		},
	})
	if err := exp.Expand(context.Background(), &Kbinfos{}, "q", "", nil); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(store.calls) == 0 {
		t.Fatal("no compiled search was performed")
	}
	tenants := map[string]string{}
	for _, c := range store.calls {
		tenants[c.kb] = c.tenant
		if len(c.docs) != 0 {
			t.Errorf("no doc scope must be sent for %s, got %v", c.kb, c.docs)
		}
		if c.tenant == "request-tenant" {
			t.Errorf("request tenant leaked into scope %+v; each dataset must use its owner tenant", c)
		}
	}
	if tenants["kb1"] != "t1" || tenants["kb2"] != "t2" {
		t.Errorf("per-dataset tenants = %v, want kb1=t1 kb2=t2", tenants)
	}
}

// TestCompiledExpanderGroupsDocScopeByRealOwner mirrors the doc-scope branch of
// Python _kg_scopes: documents are grouped by their real owning (kb, tenant) —
// which may lie OUTSIDE the bound datasets — and the bound list is not scanned.
func TestCompiledExpanderGroupsDocScopeByRealOwner(t *testing.T) {
	store := &scopeRecordingStore{}
	exp := NewCompiledExpander(store, CompiledScopeConfig{
		DatasetIDs: []string{"kb1", "kb2"},
		TenantID:   "request-tenant",
		KBs: []*entity.Knowledgebase{
			{ID: "kb1", TenantID: "t1"},
			{ID: "kb2", TenantID: "t2"},
		},
		DocTenantResolver: stubTenantResolver{owner: DocTenant{KBID: "kbX", TenantID: "tX"}},
	})
	if err := exp.Expand(context.Background(), &Kbinfos{}, "q", "", []string{"d1"}); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(store.calls) == 0 {
		t.Fatal("no compiled search was performed")
	}
	for _, c := range store.calls {
		if c.kb != "kbX" || c.tenant != "tX" {
			t.Errorf("scope = %+v, want the document's real owner kbX/tX", c)
		}
		if len(c.docs) != 1 || c.docs[0] != "d1" {
			t.Errorf("docs = %v, want [d1]", c.docs)
		}
	}
}

func rowMatchesFilters(r map[string]any, filters map[string][]string) bool {
	for k, vals := range filters {
		rv := asString(r[k])
		matched := false
		for _, v := range vals {
			if rv == v {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func compiledIDs(chunks []map[string]any) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, asString(c["id"]))
	}
	return out
}

// TestSeedNameFromRowParsesContentWithWeight mirrors Python L293-300: the seed
// name comes from content_with_weight JSON (name, else title), never from name_kwd.
func TestSeedNameFromRowParsesContentWithWeight(t *testing.T) {
	if got := seedNameFromRow(map[string]any{"content_with_weight": `{"name":"Culdcept","title":"Game"}`}); got != "Culdcept" {
		t.Errorf("name: got %q", got)
	}
	if got := seedNameFromRow(map[string]any{"content_with_weight": `{"title":"Released 1984"}`}); got != "Released 1984" {
		t.Errorf("title fallback: got %q", got)
	}
	if got := seedNameFromRow(map[string]any{"content_with_weight": `not json`}); got != "" {
		t.Errorf("bad json: got %q", got)
	}
	// name_kwd is not consulted — Python reads only the JSON payload.
	if got := seedNameFromRow(map[string]any{"content_with_weight": `{}`, "name_kwd": "X"}); got != "" {
		t.Errorf("name_kwd must be ignored, got %q", got)
	}
}

func TestLowerUnionSorted(t *testing.T) {
	got := lowerUnionSorted(map[string]bool{"OmiyaSoft": true, "X": true})
	// Byte-order ascending over the original + lowercased forms.
	want := []string{"OmiyaSoft", "X", "omiyasoft", "x"}
	if len(got) != len(want) {
		t.Fatalf("lowerUnionSorted = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("lowerUnionSorted[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}

// TestExpandEntityStrategyOneHop verifies the 1-hop entity strategy: a seed
// entity matching the query (found via its template kind) walks a forward
// relation to a neighbour, resolves the neighbour by exact name_kwd, and loads
// its source chunks.
func TestExpandEntityStrategyOneHop(t *testing.T) {
	store := &stubCompiledStore{
		rows: []map[string]any{
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "Culdcept", "content_with_weight": `{"name":"Culdcept"}`},
			{"knowledge_graph_kwd": "relation", "compilation_template_kind_kwd": "knowledge_graph",
				"from_entity_kwd": "Culdcept", "to_entity_kwd": "OmiyaSoft"},
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "OmiyaSoft", "content_with_weight": `{"name":"OmiyaSoft"}`,
				"source_chunk_ids": []string{"s1", "s2"}},
		},
		chunks: map[string]string{"s1": "a", "s2": "b"},
	}
	exp := NewCompiledExpander(store, CompiledScopeConfig{DatasetIDs: []string{"kb1"}, TenantID: "t1"}).(*compiledExpander)
	kb := &Kbinfos{}
	if err := exp.Expand(context.Background(), kb, "culdcept", "", nil); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(kb.Chunks) != 2 {
		t.Fatalf("expanded = %d (%v), want 2 neighbour source chunks", len(kb.Chunks), compiledIDs(kb.Chunks))
	}
}

// TestExpandBackwardRelation confirms the strategy also walks incoming
// (to_entity_kwd) relations, mirroring Python's fwd+bwd merge.
func TestExpandBackwardRelation(t *testing.T) {
	store := &stubCompiledStore{
		rows: []map[string]any{
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "mind_map",
				"name_kwd": "Culdcept", "content_with_weight": `{"name":"Culdcept"}`},
			// Relation where the seed is the TARGET, not the source.
			{"knowledge_graph_kwd": "relation", "compilation_template_kind_kwd": "mind_map",
				"from_entity_kwd": "OmiyaSoft", "to_entity_kwd": "Culdcept"},
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "mind_map",
				"name_kwd": "OmiyaSoft", "content_with_weight": `{"name":"OmiyaSoft"}`,
				"source_chunk_ids": []string{"o1"}},
		},
		chunks: map[string]string{"o1": "from"},
	}
	exp := NewCompiledExpander(store, CompiledScopeConfig{DatasetIDs: []string{"kb1"}, TenantID: "t1"}).(*compiledExpander)
	kb := &Kbinfos{}
	if err := exp.Expand(context.Background(), kb, "culdcept", "", nil); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(kb.Chunks) != 1 {
		t.Fatalf("backward relation gave %d (%v), want 1", len(kb.Chunks), compiledIDs(kb.Chunks))
	}
}

// TestExpandWikiSetsSimilarityAndBlendsByIt verifies wiki synthesis pages load
// their source chunks with a similarity boost (0.9) and the final sort puts the
// higher-similarity wiki chunk ahead of the entity (0-similarity) chunk.
func TestExpandWikiSetsSimilarityAndBlendsByIt(t *testing.T) {
	store := &stubCompiledStore{
		rows: []map[string]any{
			// knowledge_graph entity 1-hop -> chunk e1 (similarity 0 by default).
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "Culdcept", "content_with_weight": `{"name":"Culdcept"}`},
			{"knowledge_graph_kwd": "relation", "compilation_template_kind_kwd": "knowledge_graph",
				"from_entity_kwd": "Culdcept", "to_entity_kwd": "OmiyaSoft"},
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "OmiyaSoft", "content_with_weight": `{"name":"OmiyaSoft"}`,
				"source_chunk_ids": []string{"e1"}},
			// wiki synthesis page matching the query -> chunk w1.
			{"compile_kwd": "wiki_page", "available_int": "1", "doc_id": "d2",
				"name_kwd": "Culdcept", "content_with_weight": `{"title":"Culdcept article"}`,
				"source_chunk_ids": []string{"w1"}},
		},
		chunks: map[string]string{"e1": "entity", "w1": "wiki"},
	}
	exp := NewCompiledExpander(store, CompiledScopeConfig{DatasetIDs: []string{"kb1"}, TenantID: "t1"}).(*compiledExpander)
	kb := &Kbinfos{}
	if err := exp.Expand(context.Background(), kb, "culdcept", "", nil); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(kb.Chunks) != 2 {
		t.Fatalf("expanded = %d (%v), want 2", len(kb.Chunks), compiledIDs(kb.Chunks))
	}
	var wSim, eSim float64
	for _, c := range kb.Chunks {
		switch asString(c["id"]) {
		case "w1":
			wSim = similarityOf(c)
		case "e1":
			eSim = similarityOf(c)
		}
	}
	if wSim != 0.9 {
		t.Errorf("wiki similarity = %v, want 0.9", wSim)
	}
	if eSim != 0.0 {
		t.Errorf("entity chunk similarity = %v, want 0.0", eSim)
	}
	// Final global sort: wiki (0.9) must rank before entity (0).
	if compiledIDs(kb.Chunks)[0] != "w1" {
		t.Errorf("final sort should put wiki (0.9) first, got %v", compiledIDs(kb.Chunks))
	}
}

// TestExpandCapsMaxChunksAndSkipsSeen confirms a strategy stops adding at 5 and
// chunks already in kbinfos are not duplicated.
func TestExpandCapsMaxChunksAndSkipsSeen(t *testing.T) {
	store := &stubCompiledStore{
		rows: []map[string]any{
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "A", "content_with_weight": `{"name":"A"}`},
			{"knowledge_graph_kwd": "relation", "compilation_template_kind_kwd": "knowledge_graph",
				"from_entity_kwd": "A", "to_entity_kwd": "B"},
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "B", "content_with_weight": `{"name":"B"}`,
				"source_chunk_ids": []string{"b1", "b2", "b3", "b4", "b5", "b6", "b7"}},
		},
		chunks: map[string]string{"b1": "1", "b2": "2", "b3": "3", "b4": "4", "b5": "5", "b6": "6", "b7": "7"},
	}
	exp := NewCompiledExpander(store, CompiledScopeConfig{DatasetIDs: []string{"kb1"}, TenantID: "t1"}).(*compiledExpander)
	// b1 is already present in the working set; it must not come back.
	kb := &Kbinfos{Chunks: []map[string]any{{"id": "b1", "content": "pre-existing"}}}
	if err := exp.Expand(context.Background(), kb, "a", "", nil); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	// 5 fresh (max_chunks) minus the seen b1 that was skipped within the first
	// load batch -> at most 5 new.
	if len(kb.Chunks) > 6 { // 1 pre-existing + up to 5 new
		t.Fatalf("exceeded max_chunks=5 new chunks: got %d total (%v)", len(kb.Chunks), compiledIDs(kb.Chunks))
	}
	for _, c := range kb.Chunks {
		if asString(c["id"]) == "b1" && asString(c["content"]) != "pre-existing" {
			t.Error("seen chunk b1 was duplicated")
		}
	}
}

// mapTenantResolver resolves each doc id to its own owner and can fail the whole
// batch, standing in for the batched DB lookup (a per-doc lookup in Python).
type mapTenantResolver struct {
	owners map[string]DocTenant
	err    error
}

func (m mapTenantResolver) ResolveDocTenants(_ context.Context, docIDs []string) (map[string]DocTenant, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make(map[string]DocTenant, len(docIDs))
	for _, d := range docIDs {
		if o, ok := m.owners[d]; ok {
			out[d] = o
		}
	}
	return out, nil
}

// TestCompiledExpanderReportsDocTenantResolutionFailure pins the two rules apart:
// a row whose owner cannot be resolved is dropped silently (see the test above),
// but a FAILED batched lookup is a transport error — Python lets it raise out of
// _load_chunks_for_doc, so nothing is loaded and the failure is reported instead
// of masquerading as "no row resolved".
func TestCompiledExpanderReportsDocTenantResolutionFailure(t *testing.T) {
	store := &ownerRecordingStore{stubCompiledStore: &stubCompiledStore{
		rows: []map[string]any{
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "Culdcept", "content_with_weight": `{"name":"Culdcept"}`, "doc_id": "seedDoc"},
			{"knowledge_graph_kwd": "relation", "compilation_template_kind_kwd": "knowledge_graph",
				"from_entity_kwd": "Culdcept", "to_entity_kwd": "OmiyaSoft", "doc_id": "seedDoc"},
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "OmiyaSoft", "content_with_weight": `{"name":"OmiyaSoft"}`,
				"source_chunk_ids": []string{"s1"}, "doc_id": "docA"},
		},
		chunks: map[string]string{"s1": "a"},
	}}
	exp := NewCompiledExpander(store, CompiledScopeConfig{
		DatasetIDs:        []string{"kb1"},
		TenantID:          "t1",
		KBs:               []*entity.Knowledgebase{{ID: "kb1", TenantID: "t1"}},
		DocTenantResolver: mapTenantResolver{err: errors.New("db unavailable")},
	}).(*compiledExpander)

	kb := &Kbinfos{}
	err := exp.Expand(context.Background(), kb, "culdcept", "", nil)
	if err == nil {
		t.Fatal("Expand must report the resolver failure instead of silently loading nothing")
	}
	if !strings.Contains(err.Error(), "doc-tenant resolution failed for 1 doc(s)") {
		t.Errorf("error = %q, want it to name the failed batch and its size", err)
	}
	if len(store.loads) != 0 {
		t.Errorf("LoadChunks calls = %+v, want none after a resolver failure", store.loads)
	}
	if len(kb.Chunks) != 0 {
		t.Errorf("chunks = %v, want none", compiledIDs(kb.Chunks))
	}
}

// TestCompiledExpanderLogsAndKeepsOtherDocsWhenOneLoadFails mirrors Python's
// per-doc load: the failing doc is logged (compiled_expansion.py:248) and
// dropped, while the remaining docs still contribute.
func TestCompiledExpanderLogsAndKeepsOtherDocsWhenOneLoadFails(t *testing.T) {
	var buf bytes.Buffer
	prevLog := _LOG
	_LOG = log.New(&buf, "", 0)
	t.Cleanup(func() { _LOG = prevLog })

	store := &ownerRecordingStore{stubCompiledStore: &stubCompiledStore{
		rows: []map[string]any{
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "Culdcept", "content_with_weight": `{"name":"Culdcept"}`, "doc_id": "seedDoc"},
			{"knowledge_graph_kwd": "relation", "compilation_template_kind_kwd": "knowledge_graph",
				"from_entity_kwd": "Culdcept", "to_entity_kwd": "OmiyaSoft", "doc_id": "seedDoc"},
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "OmiyaSoft", "content_with_weight": `{"name":"OmiyaSoft"}`,
				"source_chunk_ids": []string{"s1"}, "doc_id": "docA"},
			{"knowledge_graph_kwd": "entity", "compilation_template_kind_kwd": "knowledge_graph",
				"name_kwd": "OmiyaSoft", "content_with_weight": `{"name":"OmiyaSoft"}`,
				"source_chunk_ids": []string{"s2"}, "doc_id": "docB"},
		},
		chunks:         map[string]string{"s1": "a", "s2": "b"},
		loadErrByChunk: map[string]error{"s1": errors.New("store unavailable")},
	}}
	exp := NewCompiledExpander(store, CompiledScopeConfig{
		DatasetIDs: []string{"kb1"},
		TenantID:   "t1",
		KBs:        []*entity.Knowledgebase{{ID: "kb1", TenantID: "t1"}},
		DocTenantResolver: mapTenantResolver{owners: map[string]DocTenant{
			"docA": {KBID: "kbA", TenantID: "tA"},
			"docB": {KBID: "kbB", TenantID: "tB"},
		}},
	}).(*compiledExpander)

	kb := &Kbinfos{}
	if err := exp.Expand(context.Background(), kb, "culdcept", "", nil); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got := compiledIDs(kb.Chunks); len(got) != 1 || got[0] != "s2" {
		t.Errorf("expanded = %v, want only s2 (docA's load failed)", got)
	}
	if !strings.Contains(buf.String(), "failed to load chunks for doc_id=docA") {
		t.Errorf("log = %q, want the failing doc named", buf.String())
	}
}
