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

package agentic_rag

// navigate_structure: drill into ONE document's compiled structure (the
// entity/relation outline built at compile time) and return a query-focused
// <structure_navigation> outline with chunk pointers the model can feed to
// list_chunks.
//
// Two selection strategies, in priority order:
//
//  1. CLAIM recall (flat): the document's claim rows (entity_type_kwd="claim")
//     are atomic propositions carrying their own verbatim evidence, recalled
//     with a hybrid lexical + dense leg fused by reciprocal rank. A hit is
//     already the answer material, so it REPLACES the drill instead of
//     layering on top of it: descending the tree can only re-derive what the
//     claim states, and every descent step is another chance to prune the
//     branch holding the answer.
//  2. TREE drill-down (fallback): the compiled structure is drilled with a
//     keyword-relevance beam for documents without claim hits.
//
// Typical use: call navigate_tree first to route to a document, take its
// doc_id, then call navigate_structure with that doc_id to see where the answer
// sits inside the document.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/service/nav"
	"ragflow/internal/tokenizer"
)

const navigateStructureToolName = "navigate_structure"

const navigateStructureToolDescription = `Drill into ONE document's COMPILED STRUCTURE (the entity/relation outline built at compile time) and return a query-focused outline of where the answer sits, with chunk pointers you can pass to list_chunks to deep-read.
Call it with just a query: it routes the query over the datasets' compiled navigation trees itself and returns the compiled CLAIM evidence of the best-matching documents, so no prior navigate_tree call is needed. To drill ONE document the model already chose, pass the doc_id from a <tree_navigation> result instead; that path returns the document's internal structure (entities and how they relate).
Returns an XML <structure_navigation> document: one <doc> element per readable structure, each with a <structure> outline. Outline lines are either claim lines "- [claim] <statement> | Evidence: \"<verbatim quote>\" [chunks: c1] ref=3" (the document's compiled atomic propositions recalled for your query, each carrying its own evidence and the handle to cite it by) or structure lines "- name (type): description [chunks: c1,c2]". When claims hit they replace the structure descent outright. Feed the chunk pointers to list_chunks (anchor_chunk_ids).
An empty result (<structure_navigation count="0" error="...">) means this document has NO compiled structure or claims of the requested kind - the error attribute says which: "route first with navigate_tree" (no doc_id), or "no compiled structure for this kind" (absent in THAT document - fall back to grep_chunks / search_*_chunks; the verdict is per document, so the tool stays available for other documents).`

// navigateStructureArgs is the JSON the model sends into InvokableRun.
type navigateStructureArgs struct {
	Query      string   `json:"query"`
	DocID      string   `json:"doc_id,omitempty"`
	DocIDs     []string `json:"doc_ids,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Keywords   string   `json:"keywords,omitempty"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
}

// Compiled-structure shape / kind constants, mirroring the harness navigation
// contract so the model sees the same signal shape.
var (
	// catalogKinds is the default structure-kind set read when kind is omitted.
	// It accepts the catalog/timeline shapes AND the raw graph/entity/relation
	// shapes so the default "show me this document's structure" reads every
	// compiled-shape row instead of silently dropping graph blobs.
	catalogKinds = map[string]bool{
		"tree": true, "timeline": true, "page_index": true, "pageindex": true,
		"graph": true, "entity": true, "relation": true, "knowledge_graph": true,
	}
	mindmapKinds = map[string]bool{"mindmap": true, "mind_map": true}
	// shapeKwds are the knowledge_graph_kwd ROW SHAPES a compiled structure can
	// be written as. Reading BOTH the compact "graph" blob and the per-entity /
	// per-relation rows is required: otherwise navigation silently returns EMPTY
	// for datasets compiled into per-entity/relation rows rather than a graph blob.
	shapeKwds = []string{"graph", "entity", "relation"}
)

// Drill-down bounds (mirroring the harness _STRUCT_* limits).
const (
	structMaxDepth     = 3   // max TOC levels drilled
	structMaxNodes     = 10  // cap on nodes rendered in the outline
	structBranchK      = 2   // keep top-K most relevant nodes per level
	structRelevanceMin = 1   // a node must match at least this many query terms to descend
	structDescSnippet  = 180 // cap on a node's description snippet
	structMaxChunks    = 4   // cap on chunk snippets returned in the outline
)

// Claim/evidence leg (mirroring the harness _STRUCT_CLAIM_* limits).
const (
	// navClaimRowType is the entity_type_kwd every compiler gives its evidence
	// rows. They deliberately carry no knowledge_graph_kwd and no type_kwd, so
	// they never show up as artifacts-tree nodes — this filter is the only way
	// to find them.
	navClaimRowType = "claim"
	// navClaimDocScope pins recall to doc-level rows: the KB-wide merged rows
	// written by the Build button carry a doc_id too, and without this filter
	// they would answer for a single document.
	navClaimDocScope = "doc"
	// navClaimTopN is the claim count rendered per document.
	navClaimTopN = 8
	// navClaimEvidenceChars caps the rendered verbatim quote. Generous on
	// purpose: a thin quote makes the answer stage judge the context
	// insufficient, and one extra re-search turn costs a full prompt — far more
	// than the extra characters.
	navClaimEvidenceChars = 1200
	// navClaimSufficientHits is the count at which the model may cite the claims
	// without deep-reading chunks. It is a count of RELEVANT hits: both legs
	// apply a relevance floor (below), because a leg with no floor returns a full
	// page for any document and a count over it states nothing.
	navClaimSufficientHits = 3
	// navClaimMinCoverage is the relevance floor applied to every recalled claim:
	// the share of the question's content tokens the claim text must contain.
	// The store's own defaults make a floor mandatory — full-text recall runs at
	// minimum_should_match 0% (one shared token is a hit) and KNN returns the
	// nearest neighbours at any distance — so without it every document yielded a
	// full page for every question. Measured failure: a Mike Tyson question
	// answered from a TV-series document.
	navClaimMinCoverage = 0.3
	// navClaimDenseSimilarity is the dense leg's cosine floor: the paraphrase leg
	// must not hand the fusion rows that are merely nearest, not near.
	navClaimDenseSimilarity = 0.4
	// navClaimMinDocs is the document diversity a sufficient recall needs. One
	// document's claims answer one hop; a multi-document question needs evidence
	// from more than one place, and a single-document recall is exactly what a
	// degenerate router returns for every question.
	navClaimMinDocs = 2
	// navClaimRRFK is the reciprocal-rank-fusion constant. RRF consumes ranks
	// only, so the two legs' scores never have to be comparable — their FLOORS
	// are what decides membership.
	navClaimRRFK = 60
	// navClaimPool is the per-leg candidate pool (mirroring limit = max(top_n, 32)).
	navClaimPool = 32
	// navQueryRouteDocs bounds the documents a query-only call investigates. The
	// two-step contract (navigate_tree -> navigate_structure) is a real adoption
	// barrier: a query-only call removes it, at the cost of claiming over a few
	// routed documents instead of one chosen by the model.
	navQueryRouteDocs = 3
	// navClaimPrefetchTopN bounds the claims the PIPELINE recalls before the
	// loop. It is both a cost bound and an accuracy bound: the block is the whole
	// prompt of the sufficient-claims answer and also what decides that verdict,
	// so a cap below navClaimSufficientHits x navClaimMinDocs could never
	// terminate early.
	navClaimPrefetchTopN = 3 * navClaimTopN
)

// navStructureRoute resolves candidate doc_ids for a query when the caller
// supplied no doc_id. It is the same navigation-tree router navigate_tree uses,
// so a query-only call and the explicit two-step path rank documents identically.
var navStructureRoute = func(ctx context.Context, tenantID, kbID, query string, limit int) ([][2]string, error) {
	return nav.NewNavServiceRouter().Route(ctx, tenantID, kbID, query, nil, limit)
}

// routeDocIDsForQuery routes the query across the bound datasets and returns the
// top doc_ids, deduped across datasets and capped at limit.
func routeDocIDsForQuery(ctx context.Context, tenantID, query string, datasetIDs []string, limit int) []string {
	if limit <= 0 {
		limit = navQueryRouteDocs
	}
	seen := map[string]bool{}
	var out []string
	for _, kbID := range datasetIDs {
		routed, err := navStructureRoute(ctx, tenantID, kbID, query, limit)
		if err != nil {
			common.DebugCtx(ctx, "navigate_structure: query route failed",
				zap.String("kb_id", kbID), zap.Error(err))
			continue
		}
		for _, pair := range routed {
			did := strings.TrimSpace(pair[0])
			if did == "" || seen[did] {
				continue
			}
			seen[did] = true
			out = append(out, did)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

// structureRel is a parent->child relation of a compiled structure hierarchy.
type structureRel struct {
	from    string
	to      string
	relType string
}

// structureNode is a node of a compiled structure hierarchy during drill-down.
type structureNode struct {
	name           string
	nodeType       string
	desc           string
	sourceChunkIDs []string
}

// chunkWithText pairs a chunk id with its text for structure snippet ranking.
type chunkWithText struct {
	id   string
	text string
}

// claimHit is one recalled claim row: an atomic proposition plus its verbatim
// evidence. chunkID is the FIRST source chunk id — the pointer the model feeds
// to list_chunks when it needs the surrounding passage. docID names the document
// the claim was compiled from, which is what makes document diversity checkable.
type claimHit struct {
	docID       string
	chunkID     string
	name        string
	description string
	quote       string
	score       float64
}

// claimReader is the OPTIONAL claim-recall capability of a structureReader. A
// reader that does not implement it simply has no claim leg, and the drill-down
// stays the only strategy (the pre-claim behavior).
type claimReader interface {
	// ReadClaims recalls claim rows of the given compile kinds, hybrid (lexical +
	// dense when qvec is non-empty) fused by reciprocal rank, and returns at most
	// limit hits that clear the relevance floor. A nil/empty docIDs spans the
	// whole dataset, which is what the pre-loop recall uses: it needs no document
	// choice.
	ReadClaims(ctx context.Context, tenantID, kbID string, docIDs, kinds []string, text string, qvec []float64, limit int) ([]claimHit, error)
}

// claimQueryVector encodes the drill query for the claim dense leg, using the
// embedder the nav service already resolved for this tenant — the same model
// that wrote the claim rows' q_<dim>_vec columns.
//
// It reads that embedder through the nav service rather than through an injected
// function: this package already depends on nav for routing, and the service is
// where tenant-model resolution lives, so a second wiring point in the server
// bootstrap would only duplicate it. A missing service, an unresolvable model or
// a panicking encoder all yield nil, which leaves the leg out and recall running
// on the lexical leg alone — a misconfigured embedder must never take a tool
// call down.
func claimQueryVector(ctx context.Context, tenantID, text string) (vec []float64) {
	svc := nav.GetNavService()
	if svc == nil || tenantID == "" || strings.TrimSpace(text) == "" {
		return nil
	}
	defer func() {
		if recover() != nil {
			vec = nil
		}
	}()
	if v := svc.EncodeQueryVector(ctx, tenantID, text); len(v) > 0 {
		return v
	}
	return nil
}

// structureRow is one compiled-structure row: the doc-store fields the structure
// reader reads.
type structureRow struct {
	TemplateKind      string
	KnowledgeGraphKwd string
	Content           string
}

// structureReader reads a document's compiled structure rows. The production
// implementation reads from the doc-store; tests install a fake. Until a reader
// is installed (or the engine is available) navigate_structure reports
// EMPTY/no_structure, falling through to the text tools.
type structureReader interface {
	ReadStructure(ctx context.Context, tenantID, kbID, docID string) ([]structureRow, error)
}

var structReaderInst structureReader

// setStructureReader installs the (production or test) structure reader.
func setStructureReader(r structureReader) {
	structReaderInst = r
}

func getStructureReader() structureReader {
	if structReaderInst != nil {
		return structReaderInst
	}
	return &engineStructureReader{}
}

// engineStructureReader reads compiled-structure rows directly from the
// doc-store via engine.Get(). It mirrors the harness loadStructureGraph, but is
// scoped to ONE document (the tool already knows which document to drill).
type engineStructureReader struct{}

func (engineStructureReader) ReadStructure(ctx context.Context, tenantID, kbID, docID string) ([]structureRow, error) {
	de := engine.Get()
	if de == nil {
		return nil, nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:         []string{indexNameFor(tenantID, "")},
		KbIDs:              []string{kbID},
		SelectFields:       []string{"content_with_weight", "compilation_template_kind_kwd", "type_kwd", "knowledge_graph_kwd"},
		Filter:             map[string]interface{}{"doc_id": []string{docID}, "knowledge_graph_kwd": shapeKwds},
		Limit:              3000,
		IncludeUnavailable: true,
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	rows := make([]structureRow, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		kg, _ := row["type_kwd"].(string)
		if kg == "" {
			kg, _ = row["knowledge_graph_kwd"].(string)
		}
		sr := structureRow{
			TemplateKind:      fmt.Sprint(row["compilation_template_kind_kwd"]),
			KnowledgeGraphKwd: kg,
			Content:           fmt.Sprint(row["content_with_weight"]),
		}
		rows = append(rows, sr)
	}
	return rows, nil
}

// ReadClaims is the claim leg: two independent reads over the claim rows of the
// requested documents — a full-text leg on the tokenized content (exact /
// proper-noun recall) and a KNN leg on the claim's OWN vector (paraphrase
// recall) — fused by reciprocal rank. Each leg is one store round-trip.
//
// A leg that fails is dropped rather than propagated: without an embedder the
// lexical leg alone still recalls claims, and a scope with no compiled claims
// legitimately returns nothing.
//
// The RELEVANCE FLOOR is applied here, in Go, to the fused result. The store
// cannot enforce it: `minimum_should_match` on a multi-field query_string is
// silently ignored (measured: "100%" still returned 1950 rows for a two-term
// query), and a KNN similarity cut would only cover the dense leg. Without a
// floor every document yielded a full page for every question, which is how a
// Mike Tyson question was answered from a TV-series document.
func (engineStructureReader) ReadClaims(ctx context.Context, tenantID, kbID string, docIDs, kinds []string, text string, qvec []float64, limit int) ([]claimHit, error) {
	de := engine.Get()
	if de == nil || strings.TrimSpace(text) == "" {
		return nil, nil
	}
	// The caller owns the cap: the per-document tool path asks for navClaimTopN,
	// while the pre-loop recall asks for the corpus-wide budget
	// (navClaimPrefetchTopN). Clamping every caller to the per-document count
	// silently cut that budget to one page, which is too few claims to satisfy
	// the sufficiency gate's document-diversity rule.
	if limit <= 0 {
		limit = navClaimTopN
	}
	pool := limit
	if pool < navClaimPool {
		pool = navClaimPool
	}
	filter := map[string]interface{}{
		"entity_type_kwd": []string{navClaimRowType},
		"scope_kwd":       []string{navClaimDocScope},
	}
	if len(docIDs) > 0 {
		filter["doc_id"] = docIDs
	}
	if len(kinds) > 0 {
		filter["compile_kwd"] = kinds
	}

	leg := func(exprs []interface{}) []claimHit {
		res, err := de.Search(ctx, &enginetypes.SearchRequest{
			IndexNames: []string{indexNameFor(tenantID, "")},
			KbIDs:      []string{kbID},
			SelectFields: []string{
				"content_with_weight", "source_chunk_ids", "entity_type_kwd", "compile_kwd", "doc_id",
			},
			Filter:             filter,
			MatchExprs:         exprs,
			Limit:              pool,
			IncludeUnavailable: true,
		})
		if err != nil {
			common.DebugCtx(ctx, "claim recall: leg failed",
				zap.Strings("doc_ids", docIDs), zap.Error(err))
			return nil
		}
		hits := make([]claimHit, 0, len(res.Chunks))
		for _, row := range res.Chunks {
			if h, ok := parseClaimHit(row); ok {
				hits = append(hits, h)
			}
		}
		return hits
	}

	var denseLeg, textLeg []claimHit
	if len(qvec) > 0 {
		denseLeg = leg([]interface{}{&enginetypes.MatchDenseExpr{
			VectorColumnName:  fmt.Sprintf("q_%d_vec", len(qvec)),
			EmbeddingData:     qvec,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              pool,
			ExtraOptions:      map[string]interface{}{"similarity": navClaimDenseSimilarity},
		}})
	}
	textLeg = leg([]interface{}{&enginetypes.MatchTextExpr{
		Fields:       []string{"content_ltks", "content_sm_ltks"},
		MatchingText: text,
		TopN:         pool,
	}})
	if len(denseLeg) == 0 && len(textLeg) == 0 {
		return nil, nil
	}

	fused := rrfFuse(navClaimRRFK, pool, denseLeg, textLeg)
	question := claimTokens(text)
	out := make([]claimHit, 0, len(fused))
	for _, h := range fused {
		if claimCoverage(question, h) < navClaimMinCoverage {
			continue
		}
		out = append(out, h)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// parseClaimHit turns one claim row into a hit. A row that is not a usable
// claim — bad payload, unnamed, or carrying no chunk pointer — is dropped: a
// claim the model cannot open is not an answer anchor.
func parseClaimHit(row map[string]interface{}) (claimHit, bool) {
	payload, ok := decodeJSONObject(contentWithWeightFromRaw(row))
	if !ok {
		return claimHit{}, false
	}
	name := strings.TrimSpace(stringFromAny(payload["name"]))
	if name == "" {
		return claimHit{}, false
	}
	ids := navStringList(row["source_chunk_ids"])
	if len(ids) == 0 {
		ids = navStringList(payload["source_chunk_ids"])
	}
	if len(ids) == 0 {
		return claimHit{}, false
	}
	docID := strings.TrimSpace(fmt.Sprint(row["doc_id"]))
	if docID == "<nil>" {
		docID = ""
	}
	return claimHit{
		docID:       docID,
		chunkID:     ids[0],
		name:        name,
		description: strings.TrimSpace(stringFromAny(payload["description"])),
		quote:       firstEvidenceQuote(payload["evidence"]),
		score:       floatFromMap(row, "_score"),
	}, true
}

// claimTokens returns the set of content tokens of a question, used as the
// denominator of the claim relevance floor. It uses the corpus tokenizer when
// the pool is up (same analyzer the claim text was indexed with) and falls back
// to a rune-level splitter, so a deployment without the analyzer still gets a
// relevance floor instead of none. Single-rune tokens are dropped: they are
// stopwords in alphabetic languages.
func claimTokens(text string) map[string]struct{} {
	tokens := map[string]struct{}{}
	if tok, err := tokenizer.Tokenize(text); err == nil {
		for _, t := range strings.Fields(strings.ToLower(tok)) {
			if len([]rune(t)) >= 2 {
				tokens[t] = struct{}{}
			}
		}
	}
	if len(tokens) > 0 {
		return tokens
	}
	for _, t := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(t)) >= 2 {
			tokens[t] = struct{}{}
		}
	}
	return tokens
}

// claimCoverage is the share of the question's content tokens this claim states
// or quotes. It is the floor's only input: a claim that does not repeat any of
// the question's terms cannot be evidence for it, however highly the store
// ranked it (ranks are relative, so the top hit of an irrelevant document is
// still the top hit).
func claimCoverage(question map[string]struct{}, c claimHit) float64 {
	if len(question) == 0 {
		return 1
	}
	text := c.name + " " + c.description + " " + c.quote
	claim := claimTokens(text)
	if len(claim) == 0 {
		return 0
	}
	matched := 0
	for t := range question {
		if _, ok := claim[t]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(question))
}

// firstEvidenceQuote returns the first non-empty verbatim quote of a claim's
// evidence list.
func firstEvidenceQuote(v any) string {
	list, _ := v.([]any)
	for _, item := range list {
		ev, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if q := strings.TrimSpace(stringFromAny(ev["quote"])); q != "" {
			return q
		}
	}
	return ""
}

// rrfFuse merges retrieval legs by RECIPROCAL RANK (k=60 by default). Ranks
// only, so no similarity threshold has to be calibrated across legs with
// incomparable score scales; a hit keeps its best raw score for diagnostics.
func rrfFuse(k, limit int, legs ...[]claimHit) []claimHit {
	if k <= 0 {
		k = navClaimRRFK
	}
	type fused struct {
		hit   claimHit
		score float64
	}
	byName := map[string]*fused{}
	var order []string
	for _, leg := range legs {
		for rank, h := range leg {
			key := strings.ToLower(strings.TrimSpace(h.name))
			if key == "" {
				continue
			}
			f, ok := byName[key]
			if !ok {
				byName[key] = &fused{hit: h, score: 1.0 / float64(k+rank+1)}
				order = append(order, key)
				continue
			}
			f.score += 1.0 / float64(k+rank+1)
			if h.score > f.hit.score {
				f.hit = h
			}
		}
	}
	if len(order) == 0 {
		return nil
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := byName[order[i]], byName[order[j]]
		if a.score != b.score {
			return a.score > b.score
		}
		return order[i] < order[j]
	})
	if limit > 0 && len(order) > limit {
		order = order[:limit]
	}
	out := make([]claimHit, 0, len(order))
	for _, key := range order {
		out = append(out, byName[key].hit)
	}
	return out
}

// navStringList coerces a doc-store column into a string slice: engines return
// []string, []interface{} or a bare string for the same field.
func navStringList(v any) []string {
	switch x := v.(type) {
	case []string:
		var out []string
		for _, s := range x {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		var out []string
		for _, item := range x {
			if s := strings.TrimSpace(stringFromAny(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if s := strings.TrimSpace(x); s != "" {
			return []string{s}
		}
	}
	return nil
}

// stringFromAny reads a scalar as a trimmed string, tolerating the numeric
// types engines decode JSON numbers into.
func stringFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

// claimOutlineLine renders one claim hit: the statement, its verbatim evidence,
// and the chunk pointer that opens the surrounding passage.
//
// The line is ANSWER MATERIAL (a claim carries its own evidence, so the model is
// expected to cite it instead of re-reading the document), which makes the
// passage handle mandatory: an uncited claim line ships an answer the reference
// payload cannot resolve. The handle is appended unquoted (`ref=3`) on purpose -
// claim lines are rendered inside an escaped <structure> text node, where a
// `ref="3"` attribute would arrive as `ref=&quot;3&quot;`.
func claimOutlineLine(ctx context.Context, c claimHit) string {
	line := "- [claim] " + c.name
	if q := strings.TrimSpace(c.quote); q != "" {
		line += ` | Evidence: "` + snippetRunes(q, navClaimEvidenceChars) + `"`
	}
	if c.chunkID != "" {
		line += " [chunks: " + c.chunkID + "]"
	}
	if n := evidenceRegistryFrom(ctx).Stamp(c.chunkID); n >= 0 {
		line += " ref=" + strconv.Itoa(n)
	}
	return line
}

// claimOutline is the claim-first selection: the claims ARE the outline, so
// nothing is drilled and no chunk snippet is loaded.
func claimOutline(ctx context.Context, claims []claimHit) string {
	lines := make([]string, 0, len(claims))
	for _, c := range claims {
		lines = append(lines, claimOutlineLine(ctx, c))
	}
	return strings.Join(lines, "\n")
}

// indexNameFor resolves the document/structure search index: the tenant's
// default "ragflow_<tenant_id>" unless a tenant overrides index_name.
func indexNameFor(tenantID, configured string) string {
	if configured != "" {
		return configured
	}
	return "ragflow_" + tenantID
}

// normalizeKindKey applies the API's kind normalization (page_index / graph ->
// timeline, mind_map -> mindmap).
func normalizeKindKey(kind string) string {
	switch strings.ToLower(kind) {
	case "page_index", "graph":
		return "timeline"
	case "mind_map":
		return "mindmap"
	}
	return strings.ToLower(kind)
}

// rowKind derives the normalized structure kind from a compiled-structure row,
// using its knowledge_graph_kwd (the row SHAPE: graph / entity / relation) or,
// failing that, its compilation template kind. This is what keeps a "graph"
// blob from being filtered out as the wrong kind.
func rowKind(row structureRow) string {
	kind := row.KnowledgeGraphKwd
	if kind == "" {
		kind = row.TemplateKind
	}
	return normalizeKindKey(kind)
}

// parseCompiledStructure merges the two row shapes (graph blob / per-entity /
// per-relation) into entities / relations, mirroring _query in navigation.py.
func parseCompiledStructure(rows []structureRow, kinds []string) ([]map[string]any, []map[string]any) {
	want := map[string]bool{}
	for _, k := range kinds {
		if k != "" {
			// Normalize the requested kind the same way rowKind normalizes the
			// row's own: without this "graph" (which rowKind maps to "timeline")
			// would be filtered out before its entities/relations are parsed.
			want[normalizeKindKey(k)] = true
		}
	}
	var entities, relations []map[string]any
	for _, row := range rows {
		kind := rowKind(row)
		if len(want) > 0 && !want[kind] {
			continue
		}
		graph, ok := decodeJSONObject(row.Content)
		if !ok {
			continue
		}
		switch row.KnowledgeGraphKwd {
		case "graph":
			entities = append(entities, objectList(graph["entities"])...)
			relations = append(relations, objectList(graph["relations"])...)
		case "entity":
			entities = append(entities, graph)
		case "relation":
			relations = append(relations, graph)
		}
	}
	return entities, relations
}

// structureGraphFromRaw maps what parseCompiledStructure returns onto the typed
// nodes/relations the drill-down consumes (entity -> node conversion included so
// the drill-down only ever sees structureNode).
func structureGraphFromRaw(rawEntities, rawRels []map[string]any) ([]structureNode, []structureRel) {
	var out []structureNode
	for _, e := range rawEntities {
		name, _ := e["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		typ, _ := e["type"].(string)
		desc, _ := e["description"].(string)
		n := structureNode{name: name, nodeType: typ, desc: desc}
		if ids, ok := e["source_chunk_ids"].([]interface{}); ok {
			for _, id := range ids {
				if s, ok := id.(string); ok && s != "" {
					n.sourceChunkIDs = append(n.sourceChunkIDs, s)
				}
			}
		}
		out = append(out, n)
	}
	var rels []structureRel
	for _, r := range rawRels {
		pRaw, _ := r["from"].(string)
		cRaw, _ := r["to"].(string)
		p := strings.TrimSpace(pRaw)
		c := strings.TrimSpace(cRaw)
		if p == "" || c == "" || p == c {
			continue
		}
		relType, _ := r["type"].(string)
		rels = append(rels, structureRel{from: p, to: c, relType: relType})
	}
	return out, rels
}

// queryTerms mirrors navigation.py's regex tokenizer: lowercase coarse word
// tokens of length >= 2 (the BM25-ish keyword signal).
func queryTerms(q string) []string {
	if q == "" {
		return nil
	}
	lower := strings.ToLower(q)
	var out []string
	var cur []byte
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			cur = append(cur, c)
			continue
		}
		if len(cur) >= 2 {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}
	if len(cur) >= 2 {
		out = append(out, string(cur))
	}
	return out
}

// nodeRelevance mirrors _node_relevance: how many query terms occur in a node's
// name+description.
func nodeRelevance(terms []string, name, desc string) int {
	if len(terms) == 0 {
		return 0
	}
	text := strings.ToLower(name + " " + desc)
	n := 0
	for _, t := range terms {
		if strings.Contains(text, t) {
			n++
		}
	}
	return n
}

// buildTocTree mirrors _build_toc_tree: a name->node map, the parent->children
// map, the child->parent map (single parent, last-wins) and the root names.
func buildTocTree(nodes []structureNode, rels []structureRel) (map[string]structureNode, map[string][]string, map[string]string, []string) {
	byName := map[string]structureNode{}
	for _, n := range nodes {
		if n.name != "" {
			byName[n.name] = n
		}
	}
	children := map[string][]string{}
	parents := map[string]string{}
	for _, r := range rels {
		p, c := r.from, r.to
		if p == "" || c == "" || p == c {
			continue
		}
		if _, ok := children[p]; !ok {
			children[p] = []string{}
		}
		dup := false
		for _, x := range children[p] {
			if x == c {
				dup = true
				break
			}
		}
		if !dup {
			children[p] = append(children[p], c)
		}
		parents[c] = p
	}
	var roots []string
	for n := range byName {
		if _, has := parents[n]; !has {
			roots = append(roots, n)
		}
	}
	if len(roots) == 0 {
		for n := range byName {
			if _, has := children[n]; !has {
				roots = append(roots, n)
			}
		}
	}
	if len(roots) == 0 {
		for n := range byName {
			roots = append(roots, n)
		}
	}
	return byName, children, parents, roots
}

// chunkPtrs mirrors _chunk_ptrs: a bounded, deduped comma-joined slice of an
// item's source chunk ids (<=8, the anchors the model sees).
func chunkPtrs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	var seen []string
	for _, c := range ids {
		if c == "" || slices.Contains(seen, c) {
			continue
		}
		seen = append(seen, c)
		if len(seen) >= 8 {
			break
		}
	}
	return strings.Join(seen, ",")
}

// collectChunkIDs mirrors _collect_chunk_ids: a deduped, capped union of source
// chunk ids across a set of nodes.
func collectChunkIDs(nodes []structureNode, capN int) []string {
	if capN <= 0 {
		capN = 32
	}
	var out []string
	for _, n := range nodes {
		for _, c := range n.sourceChunkIDs {
			if c == "" || slices.Contains(out, c) {
				continue
			}
			out = append(out, c)
			if len(out) >= capN {
				return out
			}
		}
	}
	return out
}

// sortedKeptNames returns a kept-name set as a sorted slice (map iteration is
// randomised per process in Go, so sorting gives a stable, reproducible order).
func sortedKeptNames(names map[string]bool) []string {
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// drillKeptNodes descends from the TOC roots, keeping top-K children per level by
// keyword relevance, and returns the kept nodes, the single-parent map and the
// kept-name set.
func drillKeptNodes(terms []string, nodes []structureNode, rels []structureRel) ([]structureNode, map[string]string, map[string]bool) {
	byName, children, parents, roots := buildTocTree(nodes, rels)
	if len(roots) == 0 {
		return nil, nil, nil
	}
	frontier := roots
	keptNames := map[string]bool{}
	depth := 0
	for len(frontier) > 0 && depth <= structMaxDepth {
		type scored struct {
			score float64
			name  string
		}
		var scoredList []scored
		for _, n := range frontier {
			if e, ok := byName[n]; ok {
				scoredList = append(scoredList, scored{float64(nodeRelevance(terms, e.name, e.desc)), n})
			}
		}
		if len(scoredList) == 0 {
			break
		}
		sort.SliceStable(scoredList, func(i, j int) bool { return scoredList[i].score > scoredList[j].score })
		var top []scored
		for _, s := range scoredList {
			if s.score >= float64(structRelevanceMin) {
				top = append(top, s)
				if len(top) >= structBranchK {
					break
				}
			}
		}
		if len(top) == 0 {
			break
		}
		var next []string
		for _, s := range top {
			keptNames[s.name] = true
			next = append(next, children[s.name]...)
		}
		frontier = next
		depth++
	}
	// Ancestors of a kept node are pulled in so a path renders root -> node.
	for name := range keptNames {
		cur := parents[name]
		guard := 0
		for cur != "" && !keptNames[cur] && guard < structMaxDepth {
			keptNames[cur] = true
			cur = parents[cur]
			guard++
		}
	}
	var kept []structureNode
	for _, n := range sortedKeptNames(keptNames) {
		if e, ok := byName[n]; ok {
			kept = append(kept, e)
		}
	}
	return kept, parents, keptNames
}

// navTypeOr returns a node type, defaulting to "other" (a missing key reads as
// absent, not "<nil>").
func navTypeOr(t string) string {
	if t == "" {
		return "other"
	}
	return t
}

// navOutlineLine renders one "- name (type): desc [chunks: c1,c2]" line.
func navOutlineLine(indent, name, nodeType, desc string, chunks []string) string {
	line := indent + "- " + name + " (" + navTypeOr(nodeType) + ")"
	if desc != "" {
		line += ": " + snippetRunes(desc, structDescSnippet)
	}
	if c := chunkPtrs(chunks); c != "" {
		line += " [chunks: " + c + "]"
	}
	return line
}

// renderOutline mirrors _render_outline: a compact flat outline of entities then
// relations (fallback when no query terms are usable).
func renderOutline(nodes []structureNode, rels []structureRel) string {
	var lines []string
	capE, capR := 40, 40
	for i, n := range nodes {
		if i >= capE || n.name == "" {
			continue
		}
		lines = append(lines, navOutlineLine("", n.name, n.nodeType, n.desc, n.sourceChunkIDs))
	}
	for i, r := range rels {
		if i >= capR || r.from == "" || r.to == "" {
			continue
		}
		rt := r.relType
		if rt == "" {
			rt = "related_to"
		}
		lines = append(lines, "- "+r.from+" -["+rt+"]-> "+r.to)
	}
	return strings.Join(lines, "\n")
}

// renderTocDrilldown drills a document's compiled structure toward the query: no
// usable query terms fall back to the flat outline, otherwise the tree is
// descended by keyword relevance. loader, when non-nil, fetches chunk texts so
// short snippets can be appended.
func renderTocDrilldown(query string, nodes []structureNode, rels []structureRel, loader func(ids []string) []chunkWithText) string {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return renderOutline(nodes, rels)
	}
	kept, parents, keptNames := drillKeptNodes(terms, nodes, rels)
	if len(kept) == 0 {
		return renderOutline(nodes, rels)
	}
	// depth_of: node -> number of kept ancestors (rendering indentation).
	depthOf := map[string]int{}
	for n := range keptNames {
		d := 0
		cur := parents[n]
		for cur != "" && keptNames[cur] {
			d++
			cur = parents[cur]
		}
		depthOf[n] = d
	}
	var lines []string
	count := len(kept)
	if count > structMaxNodes {
		count = structMaxNodes
	}
	for _, e := range kept[:count] {
		name := strings.TrimSpace(e.name)
		if name == "" {
			continue
		}
		indent := strings.Repeat("  ", depthOf[name])
		lines = append(lines, navOutlineLine(indent, name, e.nodeType, e.desc, e.sourceChunkIDs))
	}
	if loader != nil {
		wanted := collectChunkIDs(kept, 32)
		if len(wanted) > 0 {
			chunks := loader(wanted)
			capN := len(chunks)
			if capN > structMaxChunks {
				capN = structMaxChunks
			}
			for _, c := range chunks[:capN] {
				text := strings.TrimSpace(c.text)
				if text == "" {
					continue
				}
				lines = append(lines, "- [chunk "+c.id+"]: "+snippetRunes(text, 300))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// structureDocSegment renders one <doc> element (doc_id / entities / relations
// plus the <structure> outline).
func structureDocSegment(docID string, rank, entities, relations int, outline string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `  <doc rank="%d" doc_id="%s" entities="%d" relations="%d">`, rank, xmlEscape(docID), entities, relations)
	if outline != "" {
		b.WriteString("\n    <structure>" + xmlEscape(outline) + "</structure>")
	}
	b.WriteString("\n  </doc>")
	return b.String()
}

// navigateStructures renders the compiled structures of the given documents into
// a single <structure_navigation> with one <doc> per readable structure. A
// document without a compiled structure of the requested kind is skipped; an
// empty result (no doc carried a structure) reports no_structure.
func navigateStructures(ctx context.Context, tenantID string, query string, docIDs, datasetIDs []string, kind string, reader structureReader) string {
	if len(docIDs) == 0 {
		return navStructEmpty("route first with navigate_tree")
	}
	kinds := structureKindsFor(kind)
	var segs []string
	for _, did := range docIDs {
		outline, entities, relations, ok := readStructureDocCore(ctx, tenantID, query, did, kinds, datasetIDs, reader)
		if !ok {
			continue
		}
		segs = append(segs, structureDocSegment(did, len(segs)+1, entities, relations, outline))
	}
	if len(segs) == 0 {
		return navStructEmpty("no compiled structure for this kind")
	}
	parts := append([]string{
		fmt.Sprintf(`<structure_navigation count="%d" query="%s" kind="%s">`, len(segs), xmlEscape(query), xmlEscape(kind)),
	}, segs...)
	parts = append(parts, "</structure_navigation>")
	return strings.Join(parts, "\n")
}

// navigateClaimsForQuery is the query-only path: it claims over the ROUTED
// documents and nothing else. No tree read, no drill — a document without claim
// hits contributes nothing, because this path exists to surface compiled
// evidence, not to duplicate what the explicit doc_id path already does.
// limit caps the TOTAL claims rendered across documents (0 = uncapped, the
// model-invoked behavior). Round-robin across documents rather than first-doc
// order, so a tight cap still covers every routed document.
func navigateClaimsForQuery(ctx context.Context, tenantID, query string, docIDs, datasetIDs []string, kind string, limit int, reader structureReader) (string, int) {
	docs, perDoc := recallClaimsForQuery(ctx, tenantID, query, docIDs, datasetIDs, kind, limit, reader)
	var segs []string
	for i, claims := range perDoc {
		if len(claims) == 0 {
			continue
		}
		segs = append(segs, structureDocSegment(docs[i], len(segs)+1, 0, 0, claimOutline(ctx, claims)))
	}
	if len(segs) == 0 {
		return "", 0
	}
	parts := append([]string{
		fmt.Sprintf(`<structure_navigation count="%d" query="%s" kind="%s">`, len(segs), xmlEscape(query), xmlEscape(kind)),
	}, segs...)
	parts = append(parts, "</structure_navigation>")
	return strings.Join(parts, "\n"), len(segs)
}

// claimKindList flattens the requested compile kinds into the sorted filter the
// claim reader takes. Claim rows are selected by compile_kwd, so every claim read
// needs it — the per-document tool path and the corpus-wide pre-loop recall alike.
func claimKindList(kinds map[string]bool) []string {
	out := make([]string, 0, len(kinds))
	for k := range kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// recallClaimsForQuery runs the claim leg over the given documents and returns
// the documents that hit, aligned with their claims. limit caps the TOTAL claims
// (0 = uncapped); the trim is round-robin, so a tight budget still touches every
// document instead of being spent on the first one.
//
// It is the retrieval half of the per-document tool path; the pre-loop recall
// reads corpus-wide instead (see recallClaimsAcrossDatasets).
func recallClaimsForQuery(ctx context.Context, tenantID, query string, docIDs, datasetIDs []string, kind string, limit int, reader structureReader) ([]string, [][]claimHit) {
	kindList := claimKindList(structureKindsFor(kind))

	var perDoc [][]claimHit
	var docs []string
	for _, did := range docIDs {
		claims := recallClaimsForDoc(ctx, tenantID, query, did, kindList, datasetIDs, reader)
		if len(claims) == 0 {
			continue
		}
		perDoc = append(perDoc, claims)
		docs = append(docs, did)
	}
	if len(perDoc) == 0 {
		return nil, nil
	}
	if limit > 0 {
		perDoc = trimClaimsRoundRobin(perDoc, limit)
	}
	return docs, perDoc
}

// trimClaimsRoundRobin keeps at most limit claims in total, taking one per
// document per round so no document monopolizes a small budget.
func trimClaimsRoundRobin(perDoc [][]claimHit, limit int) [][]claimHit {
	if limit <= 0 {
		return perDoc
	}
	out := make([][]claimHit, len(perDoc))
	total := 0
	for round := 0; total < limit; round++ {
		progressed := false
		for i := range perDoc {
			if round >= len(perDoc[i]) {
				continue
			}
			out[i] = append(out[i], perDoc[i][round])
			total++
			progressed = true
			if total >= limit {
				break
			}
		}
		if !progressed {
			break
		}
	}
	return out
}

// readStructureDocCore reads ONE document's compiled structure of the requested
// kind and drills it toward the query. It returns the rendered outline, the
// entity/relation counts, and whether the document carried a structure at all.
//
// Claim recall runs FIRST and, when it hits, replaces the drill entirely: the
// claims carry their own evidence, so descending the tree could only re-derive
// what they already state.
func readStructureDocCore(ctx context.Context, tenantID, query, docID string, kinds map[string]bool, datasetIDs []string, reader structureReader) (string, int, int, bool) {
	if query == "" || docID == "" {
		return "", 0, 0, false
	}
	kindList := claimKindList(kinds)
	if claims := recallClaimsForDoc(ctx, tenantID, query, docID, kindList, datasetIDs, reader); len(claims) > 0 {
		return claimOutline(ctx, claims), 0, 0, true
	}

	var nodes []structureNode
	var rels []structureRel
	read := false
	for _, kbID := range datasetIDs {
		rows, err := reader.ReadStructure(ctx, tenantID, kbID, docID)
		if err != nil {
			common.DebugCtx(ctx, "navigate_structure: read failed",
				zap.String("kb_id", kbID), zap.String("doc_id", docID), zap.Error(err))
			continue
		}
		if len(rows) == 0 {
			continue
		}
		read = true
		entities, relations := parseCompiledStructure(rows, kindList)
		es, rs := structureGraphFromRaw(entities, relations)
		nodes = append(nodes, es...)
		rels = append(rels, rs...)
	}
	if !read || len(nodes) == 0 {
		return "", 0, 0, false
	}
	loader := func(ids []string) []chunkWithText { return structureNodeLoader(ctx, tenantID, ids) }
	return renderTocDrilldown(query, nodes, rels, loader), len(nodes), len(rels), true
}

// recallClaimsForDoc runs the claim leg across the document's datasets, keeping
// the fused rank order. Hits are deduped by claim NAME (two datasets cannot both
// hold the same document, but a reader may return a claim twice) — deliberately
// NOT by chunk id: several distinct claims commonly come from one chunk, and
// dropping all but the first would throw evidence away.
func recallClaimsForDoc(ctx context.Context, tenantID, query, docID string, kindList, datasetIDs []string, reader structureReader) []claimHit {
	cr, ok := reader.(claimReader)
	if !ok {
		return nil
	}
	qvec := claimQueryVector(ctx, tenantID, query)
	var out []claimHit
	seen := map[string]bool{}
	for _, kbID := range datasetIDs {
		hits, err := cr.ReadClaims(ctx, tenantID, kbID, []string{docID}, kindList, query, qvec, navClaimTopN)
		if err != nil {
			common.DebugCtx(ctx, "navigate_structure: claim recall failed",
				zap.String("kb_id", kbID), zap.String("doc_id", docID), zap.Error(err))
			continue
		}
		for _, h := range hits {
			key := strings.ToLower(strings.TrimSpace(h.name))
			if h.chunkID == "" || key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, h)
			if len(out) >= navClaimTopN {
				return out
			}
		}
	}
	return out
}

// structureNodeLoader fetches chunk texts by id for the drill-down snippet pass.
func structureNodeLoader(ctx context.Context, tenantID string, ids []string) []chunkWithText {
	de := engine.Get()
	if de == nil || len(ids) == 0 {
		return nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:   []string{indexNameFor(tenantID, "")},
		SelectFields: []string{"content_with_weight", "docnm_kwd", "doc_id", "id"},
		Filter:       map[string]interface{}{"id": ids},
		Limit:        len(ids),
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil
	}
	out := make([]chunkWithText, 0, len(res.Chunks))
	for _, c := range res.Chunks {
		id := fmt.Sprint(c["id"])
		if id == "" {
			continue
		}
		out = append(out, chunkWithText{id: id, text: fmt.Sprint(c["content_with_weight"])})
	}
	return out
}

// structureKindsFor maps a navigate_structure kind string to the compiled-kinds
// set to read, mirroring navigation.py _structure_kinds_for. Defaults to catalog.
func structureKindsFor(kind string) map[string]bool {
	k := strings.TrimSpace(strings.ToLower(kind))
	if k == "" {
		k = "catalog"
	}
	switch k {
	case "mindmap", "mind_map", "concept":
		out := make(map[string]bool, len(mindmapKinds))
		for kk := range mindmapKinds {
			out[kk] = true
		}
		return out
	case "graph", "kg", "entity", "ontology":
		return map[string]bool{"graph": true, "ontology": true, "entity": true, "raptor": true}
	default:
		out := make(map[string]bool, len(catalogKinds))
		for kk := range catalogKinds {
			out[kk] = true
		}
		return out
	}
}

func decodeJSONObject(s string) (map[string]any, bool) {
	v := firstJSONValue(s)
	m, ok := v.(map[string]any)
	return m, ok
}

// firstJSONValue finds the first JSON object in s (tolerating surrounding prose or
// code fences) and returns it decoded.
func firstJSONValue(s string) any {
	start := strings.Index(s, "{")
	if start < 0 {
		return nil
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			// Inside a string literal a backslash escapes the next byte, so a
			// `\"` must not be read as the closing quote (which would flip the
			// in-string state and corrupt the brace depth - observed with entity
			// descriptions that carry escaped quotes).
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				raw := s[start : i+1]
				var v any
				if err := json.Unmarshal([]byte(raw), &v); err == nil {
					return v
				}
				return nil
			}
		}
	}
	return nil
}

func objectList(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// snippetRunes truncates s to at most n runes, appending an ellipsis when cut.
func snippetRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// navStructEmpty returns the empty <structure_navigation count="0"> XML carrying
// an optional error="..." attribute, mirroring the harness navigation contract.
func navStructEmpty(label string) string {
	attr := ""
	if label != "" {
		attr = ` error="` + xmlEscape(label) + `"`
	}
	return "<structure_navigation count=\"0\"" + attr + ">\n</structure_navigation>"
}

// NavigateStructureTool drills a document's compiled structure. It is stateless:
// the tenant and dataset scope are injected at construction from the session.
type NavigateStructureTool struct {
	tenantID   string
	datasetIDs []string
}

// NewNavigateStructureTool returns a NavigateStructureTool scoped to the given
// tenant and datasets, implementing eino's runtime.InvokableTool.
func NewNavigateStructureTool(tenantID string, datasetIDs []string) *NavigateStructureTool {
	return &NavigateStructureTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model.
func (g *NavigateStructureTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "REQUIRED: the question (or topic) to focus the structure drill on. Phrase it as the subject you are investigating."
    },
    "doc_id": {
      "type": "string",
      "description": "OPTIONAL: the document to drill, taken from a navigate_tree <doc doc_id> result. Omit it to route the query yourself and get compiled claim evidence from the best-matching documents directly (no navigate_tree call needed)."
    },
    "doc_ids": {
      "type": "array",
      "description": "Optional explicit document ids to drill (at most 10). When omitted, doc_id is used; when both are omitted the query is routed.",
      "items": { "type": "string" },
      "maxItems": 10
    },
    "kind": {
      "type": "string",
      "description": "Optional structure kind to read: \"catalog\" (default, tree/page_index/timeline), \"mindmap\", or \"graph\"/\"kg\"/\"entity\"/\"ontology\"."
    },
    "keywords": {
      "type": "string",
      "description": "Optional extra keywords that enrich the drill query without being required to match."
    },
    "dataset_ids": {
      "type": "array",
      "description": "Optional dataset ids to restrict to (at most 10). When omitted, the current conversation's bound datasets are used.",
      "items": { "type": "string" },
      "maxItems": 10
    }
  },
  "required": ["query"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("navigate_structure: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        navigateStructureToolName,
		Desc:        navigateStructureToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun drills the document structure and returns the XML
// <structure_navigation> document.
func (g *NavigateStructureTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, navigateStructureToolName, g.invokableRun, argumentsInJSON)
}

// claimPrefetch is the ONE claim recall a turn performs before its loop, with
// the verdict the run flow acts on.
//
// The recall exists because the compiled tools are injected rather than
// declared, and a model that prefers one-step keyword search never calls them:
// in the FRAMES runs every turn was offered navigate_structure and the tool was
// called zero times. Evidence a model cannot be relied upon to request has to be
// retrieved by the pipeline, and the claim leg is exactly that - a flat read that
// needs no document choice.
//
// The result is a CONTROL SIGNAL, not prompt text. Claims that answer the
// question terminate the turn without reading a single raw chunk; a recall that
// does not cover the question is dropped and the ordinary tool-driven
// exploration runs. Nothing is appended to the system instruction: a block there
// is re-billed on every model call of the turn, which is the cost this replaces.
type claimPrefetch struct {
	docs   []string
	claims []claimHit
	// evidence is the rendered claim block, carrying the [ID:n] handles its
	// source chunks were stamped with. It is rendered ONLY for a sufficient
	// recall, because rendering is what publishes handles: a discarded recall
	// must not leave handles behind for passages the model never saw.
	evidence string
	// sufficient is true when claimsAnswerQuestion accepted the recall.
	sufficient bool
}

// recallCompiledClaims recalls the bound datasets' claims for this turn's
// question and decides whether they answer it themselves.
func recallCompiledClaims(ctx context.Context, tenantID string, datasetIDs []string, messages []*schema.Message) claimPrefetch {
	question := strings.TrimSpace(lastUserQuestion(messages))
	if question == "" || len(datasetIDs) == 0 {
		return claimPrefetch{}
	}
	// A compiled dataset does not imply claims: trees, timelines and mindmaps all
	// carry compile_kwd but no evidence rows. Probing first keeps both retrieval
	// legs from running over a corpus that can only come back empty.
	if !claimRowsPresent(ctx, tenantID, datasetIDs) {
		common.DebugCtx(ctx, "agentic_rag: claim recall skipped",
			zap.Strings("datasets", datasetIDs),
			zap.String("reason", "no claim rows in the bound datasets"))
		return claimPrefetch{}
	}
	reader := getStructureReader()
	// NO document routing: the claim leg is a flat, corpus-wide read, which is
	// the whole reason it can run before the loop. Routing first (the nav-tree
	// KNN) reintroduced the document choice this path exists to avoid, and it
	// fails silently where it matters: a dataset whose nav tree holds a handful
	// of rows routes every question to the same one or two documents, and the
	// recall then dutifully returns their claims for every question.
	//
	// The turn's document scope, when set, is still honoured — as a filter on the
	// claim rows themselves.
	docScope, _ := resolveDocumentScope(ctx, nil)
	// A non-nil EMPTY scope means the turn allows no documents. The reader drops
	// its doc_id filter when the scope is empty, so passing one through would read
	// as "unrestricted" and recall the whole dataset — the opposite of the turn's
	// scope. Stop before the read instead.
	if docScope != nil && len(docScope) == 0 {
		common.DebugCtx(ctx, "agentic_rag: claim recall skipped",
			zap.Strings("datasets", datasetIDs),
			zap.String("reason", "no documents in the turn's scope"))
		return claimPrefetch{}
	}
	claims := recallClaimsAcrossDatasets(ctx, tenantID, datasetIDs, docScope, question, navClaimPrefetchTopN, reader)

	pf := claimPrefetch{claims: claims, docs: claimDocsInOrder(claims)}
	pf.sufficient = claimsAnswerQuestion(claims, len(pf.docs))
	if !pf.sufficient {
		common.InfoCtx(ctx, "agentic_rag: claim recall insufficient",
			zap.String("query", question),
			zap.Int("claims", len(claims)),
			zap.Int("docs", len(pf.docs)),
			zap.Int("required_claims", navClaimSufficientHits),
			zap.Int("required_docs", navClaimMinDocs))
		return pf
	}
	pf.evidence = claimEvidenceBlock(ctx, claims)
	common.InfoCtx(ctx, "agentic_rag: claim recall sufficient",
		zap.String("query", question),
		zap.Int("claims", len(claims)),
		zap.Strings("docs", pf.docs),
		zap.Int("evidence_bytes", len(pf.evidence)))
	return pf
}

// recallClaimsAcrossDatasets runs the flat claim recall over every bound
// dataset and merges the results by claim name, keeping the first (best-ranked)
// occurrence. Each dataset is one pair of legs; the corpus-wide read is what
// makes the pre-loop recall independent of any document ranking.
func recallClaimsAcrossDatasets(ctx context.Context, tenantID string, datasetIDs, docScope []string, query string, limit int, reader structureReader) []claimHit {
	cr, ok := reader.(claimReader)
	if !ok {
		return nil
	}
	kindList := claimKindList(structureKindsFor(""))
	qvec := claimQueryVector(ctx, tenantID, query)

	// Group the hits by source document, in ranked order, then take them
	// round-robin. A ranking's top-N is routinely dominated by ONE document, and
	// evidence from a single document cannot answer a multi-document question;
	// taking one claim per document per round is what makes the sufficiency
	// diversity check meaningful instead of accidental.
	var docOrder []string
	byDoc := map[string][]claimHit{}
	seen := map[string]struct{}{}
	for _, kbID := range datasetIDs {
		hits, err := cr.ReadClaims(ctx, tenantID, kbID, docScope, kindList, query, qvec, limit)
		if err != nil {
			common.DebugCtx(ctx, "claim recall: dataset read failed",
				zap.String("kb_id", kbID), zap.Error(err))
			continue
		}
		for _, h := range hits {
			key := strings.ToLower(strings.TrimSpace(h.name))
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			if _, ok := byDoc[h.docID]; !ok {
				docOrder = append(docOrder, h.docID)
			}
			byDoc[h.docID] = append(byDoc[h.docID], h)
		}
	}
	if len(docOrder) == 0 {
		return nil
	}
	groups := make([][]claimHit, 0, len(docOrder))
	for _, d := range docOrder {
		groups = append(groups, byDoc[d])
	}
	if limit > 0 {
		groups = trimClaimsRoundRobin(groups, limit)
	}
	out := make([]claimHit, 0, limit)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// claimDocsInOrder returns the distinct source documents of a claim set, in
// first-appearance order, for logging and evidence grouping.
func claimDocsInOrder(claims []claimHit) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(claims))
	for _, c := range claims {
		if c.docID == "" {
			continue
		}
		if _, dup := seen[c.docID]; dup {
			continue
		}
		seen[c.docID] = struct{}{}
		out = append(out, c.docID)
	}
	return out
}

// claimsAnswerQuestion reports whether a recall covers the question well enough
// to answer without reading any raw chunk.
//
// Two conditions, both necessary:
//   - COUNT: at least navClaimSufficientHits claims. Only meaningful because
//     recall already applied the relevance floor (navClaimMinCoverage), so a hit
//     states something about the question rather than sharing a stopword with it.
//   - DIVERSITY: claims from at least navClaimMinDocs documents. A page of
//     statements from ONE document answers at most one hop, and a single-document
//     recall is also exactly what a broken document ranking produces for every
//     question — the shape of the zero-score run this guard was added for.
func claimsAnswerQuestion(claims []claimHit, docs int) bool {
	return len(claims) >= navClaimSufficientHits && docs >= navClaimMinDocs
}

// claimEvidenceBlock renders the recalled claims as answer material, one <doc>
// element per source document so a multi-document question keeps its provenance
// and the answer can name which document each fact came from.
func claimEvidenceBlock(ctx context.Context, claims []claimHit) string {
	var b strings.Builder
	current := ""
	for _, c := range claims {
		if c.docID != current {
			if current != "" {
				b.WriteString("</doc>\n")
			}
			if c.docID != "" {
				fmt.Fprintf(&b, "<doc doc_id=\"%s\">\n", xmlEscape(c.docID))
			}
			current = c.docID
		}
		b.WriteString(claimOutlineLine(ctx, c))
		b.WriteString("\n")
	}
	if current != "" {
		b.WriteString("</doc>\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g *NavigateStructureTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args navigateStructureArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("navigate_structure: parse arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("navigate_structure: query is required and must be a non-empty string")
	}
	// Entry trace: logged BEFORE the enablement check and the backend call, so
	// the tool-usage gate can tell "never called" from "called and bailed out".
	common.InfoCtx(ctx, "agentic_rag: navigate_structure called",
		zap.String("query", query), zap.String("kind", args.Kind),
		zap.Int("docs", len(args.DocIDs)))
	if !navToolEnabled(ctx, navigateStructureToolName) {
		// Marked unavailable for this conversation (bound datasets have no
		// compiled structure): skip the backend and return the same verdict the
		// caller falls back from.
		common.DebugCtx(ctx, "navigate_structure: marked unavailable; skipping backend (no structure)")
		return navStructEmpty("no structure"), nil
	}
	datasetIDs, err := resolveDatasetScope(g.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("navigate_structure: %w", err)
	}
	if len(datasetIDs) == 0 {
		return navStructEmpty("no bound datasets"), nil
	}

	reader := getStructureReader()

	text := strings.TrimSpace(query + " " + strings.TrimSpace(args.Keywords))

	docIDs := args.DocIDs
	if len(docIDs) == 0 && args.DocID != "" {
		docIDs = []string{args.DocID}
	}
	// A doc_id is optional: with none, route the query ourselves so the tool is
	// usable in one call. Only CLAIMS are read on this path - the drill needs a
	// document the model actually chose.
	routedByQuery := false
	if len(docIDs) == 0 {
		// The router ranks the whole bound corpus, so a turn restricted to a set
		// of documents must intersect here - the same rule the pre-loop recall
		// applies to its claim rows.
		docIDs = intersectDocumentScope(ctx, routeDocIDsForQuery(ctx, g.tenantID, text, datasetIDs, navQueryRouteDocs))
		if len(docIDs) == 0 {
			return navStructEmpty("route first with navigate_tree"), nil
		}
		routedByQuery = true
	}
	if len(docIDs) > 10 {
		docIDs = docIDs[:10]
	}

	if routedByQuery {
		// 0 = uncapped: the model asked for this call, so its result lands in the
		// history once instead of being re-sent on every turn of the run. The
		// pipeline's own pre-loop recall applies its cap instead.
		out, docs := navigateClaimsForQuery(ctx, g.tenantID, text, docIDs, datasetIDs, args.Kind, 0, reader)
		common.InfoCtx(ctx, "agentic_rag: navigate_structure (query route)",
			zap.String("query", query),
			zap.Int("datasets", len(datasetIDs)),
			zap.Strings("routed", docIDs),
			zap.Int("docs_with_claims", docs),
			zap.Int("out_bytes", len(out)))
		if docs == 0 {
			return navStructEmpty("no claim hits in the routed documents; fall back to search_chunks or list_chunks"), nil
		}
		return out, nil
	}

	out := navigateStructures(ctx, g.tenantID, text, docIDs, datasetIDs, args.Kind, reader)
	// Per-call INFO: the tool-usage gate (kg_tool_calls) is read off the server
	// log, so a graph tool that rendered a structure without leaving a line here
	// cannot be told from one that was never called.
	common.InfoCtx(ctx, "agentic_rag: navigate_structure",
		zap.String("query", query),
		zap.Int("datasets", len(datasetIDs)),
		zap.Int("docs", len(docIDs)),
		zap.Strings("names", docIDs),
		zap.Int("out_bytes", len(out)))
	return out, nil
}
