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

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"

	"go.uber.org/zap"
)

// compiledServeMode selects which corpus text reaches the model after a locate
// recall: raw chunks (off), compiled products only (only), or compiled products
// with the raw body available on demand via list_chunks (first).
type csMode int

const (
	csOff csMode = iota
	csOnly
	csFirst
)

func compiledServeMode() csMode {
	switch strings.ToLower(strings.TrimSpace(common.GetEnv(common.EnvCompiledServe))) {
	case "only":
		return csOnly
	case "first":
		return csFirst
	default:
		return csOff
	}
}

// compiledServeActive reports whether the locate exit of this tool should serve
// compiled products instead of the raw snippet. web_search is never in scope
// (its hits have no corpus document to derive a product from), and an
// unrecognised tool is never served — the scope is an allowlist.
func compiledServeActive(tool string) bool {
	return compiledServeMode() != csOff && compiledServeAppliesTo(tool)
}

func compiledServeOnlyMode() bool {
	return compiledServeMode() == csOnly
}

// compiledServeExcluded lists the tools that must NOT serve compiled products.
// Every other tool that renders a locate result set is in scope, because the
// serve point (formatLocateResultsXML) is shared by ALL locate tools: an
// allowlist would silently skip any chunk-searching tool added later.
//
//   - list_chunks is the deep-read fallback. Serving claims there would remove
//     the exact place the model goes back to for the raw text.
//   - web_search hits are web pages, not corpus chunks, so they carry no
//     source_chunk_ids to derive a product from.
//   - search_metadata returns document-level records, not passage text.
//   - the graph/navigation tools return structure (nodes, outlines), not chunks.
var compiledServeExcluded = map[string]bool{
	listChunksToolName:        true,
	webSearchToolName:         true,
	metadataSearchToolName:    true,
	graphExploreToolName:      true,
	graphDocOutlineToolName:   true,
	navigateTreeToolName:      true,
	navigateStructureToolName: true,
}

func compiledServeAppliesTo(tool string) bool {
	if override := strings.TrimSpace(common.GetEnv(common.EnvCompiledServeScope)); override != "" {
		m := make(map[string]bool, 4)
		for _, t := range strings.Split(override, ",") {
			if t = strings.TrimSpace(t); t != "" {
				m[t] = true
			}
		}
		return m[tool]
	}
	return !compiledServeExcluded[tool]
}

func compiledServeTopN() int {
	return envIntDefault(common.EnvCompiledServeTopN, 12)
}

func compiledServeKinds() []string {
	return nonEmptyStrings(strings.Split(common.GetEnv(common.EnvCompiledServeKinds), ","))
}

// compiledServeTypes narrows the served rows to these entity_type_kwd values.
// Empty keeps every type, which is the graph/hypergraph behaviour: those rows
// carry no entity_type_kwd. Setting TYPES=claim is what makes the same arm serve
// claims compiled from the recalled chunk instead of abstract graph products.
func compiledServeTypes() []string {
	return nonEmptyStrings(strings.Split(common.GetEnv(common.EnvCompiledServeTypes), ","))
}

// compiledRow is one knowledge-compiled product derived from a recalled chunk.
type compiledRow struct {
	id          string
	kind        string // compile_kwd (graph/hypergraph/tree/wiki/...)
	typeName    string // payload "type" (person/event/summary/...)
	name        string
	description string
	docID       string
	chunkIDs    []string // source_chunk_ids — the lineage back to raw chunks
	mention     int      // mention_count_int — drives the relevance ordering
}

// fetchCompiledProductsFn is the seam the rendering path calls. It is a package
// var (not a direct call) so tests can substitute an engine-free source; the
// real implementation reads ES/Infinity by source_chunk_ids lineage.
var fetchCompiledProductsFn = fetchCompiledProducts

// buildCompiledFilter is the lineage query: every compiled row whose
// source_chunk_ids contains one of the recalled chunk ids, restricted to rows
// that actually carry compile_kwd (ordinary chunks do not). This is the B design
// — pure provenance linkage, no keyword/text matching on the compiled text.
func buildCompiledFilter(chunkIDs, kinds, types []string) map[string]interface{} {
	f := map[string]interface{}{
		"exists":           "compile_kwd",
		"source_chunk_ids": chunkIDs,
	}
	if len(kinds) > 0 {
		f["compile_kwd"] = kinds
	}
	if len(types) > 0 {
		// Claim rows share compile_kwd=tree with tree nodes; entity_type_kwd is the
		// only field that tells them apart, so claim serving is KINDS=tree +
		// TYPES=claim.
		f["entity_type_kwd"] = types
	}
	return f
}

func fetchCompiledProducts(ctx context.Context, datasetIDs, chunkIDs []string) []compiledRow {
	tenantID := decisionScopeFrom(ctx).tenantID
	de := engine.Get()
	if de == nil || len(chunkIDs) == 0 {
		return nil
	}
	start := time.Now()
	res, err := de.Search(ctx, &enginetypes.SearchRequest{
		IndexNames:   []string{indexNameFor(tenantID, "")},
		KbIDs:        datasetIDs,
		SelectFields: []string{"id", "content_with_weight", "source_chunk_ids", "compile_kwd", "doc_id"},
		Filter:       buildCompiledFilter(chunkIDs, compiledServeKinds(), compiledServeTypes()),
		Limit:        compiledServeTopN() * 4, // overscan so a mention_count sort can trim to top-N
		// IncludeUnavailable mirrors datasetsHaveCompiledKnowledge: a product the
		// pipeline hid should still be showable here, and the junk filter below
		// drops rows that carry no usable content.
		IncludeUnavailable: true,
	})
	if err != nil || res == nil {
		common.DebugCtx(ctx, "agentic_rag: compiled serve fetch failed",
			zap.Strings("datasets", datasetIDs), zap.Error(err))
		return nil
	}
	rows := make([]compiledRow, 0, len(res.Chunks))
	for _, raw := range res.Chunks {
		if r, ok := parseCompiledRow(raw); ok {
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return mentionCountOf(rows[i]) > mentionCountOf(rows[j])
	})
	if len(rows) > compiledServeTopN() {
		rows = rows[:compiledServeTopN()]
	}
	common.InfoCtx(ctx, "agentic_rag: compiled serve fetched",
		zap.Int("rows", len(rows)),
		zap.Int("chunks", len(chunkIDs)),
		zap.Duration("latency", time.Since(start)))
	return rows
}

// parseCompiledRow turns one compiled row into a hit, dropping junk: rows whose
// compile_kwd is the "*" placeholder carry no content, and rows whose payload
// yields neither a name nor a description are unusable anchors.
func parseCompiledRow(raw map[string]interface{}) (compiledRow, bool) {
	kind := strings.TrimSpace(fmt.Sprint(raw["compile_kwd"]))
	if kind == "" || kind == "*" {
		return compiledRow{}, false
	}
	payload, ok := decodeJSONObject(contentWithWeightFromRaw(raw))
	if !ok || payload == nil {
		return compiledRow{}, false
	}
	name := strings.TrimSpace(stringFromAny(payload["name"]))
	desc := strings.TrimSpace(stringFromAny(payload["description"]))
	if name == "" && desc == "" {
		return compiledRow{}, false
	}
	if desc == "" {
		desc = name
	}
	return compiledRow{
		id:          fmt.Sprint(raw["id"]),
		kind:        kind,
		typeName:    strings.TrimSpace(stringFromAny(payload["type"])),
		name:        name,
		description: desc,
		docID:       strings.TrimSpace(fmt.Sprint(raw["doc_id"])),
		chunkIDs:    navStringList(raw["source_chunk_ids"]),
		mention:     atoiOrZero(fmt.Sprint(raw["mention_count_int"])),
	}, true
}

func mentionCountOf(r compiledRow) int {
	return r.mention
}

// atoiOrZero parses an ES int field that may arrive as a string; a missing or
// non-numeric value (the common case for rows never stamped with a count) is 0.
func atoiOrZero(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func compileProductText(p compiledRow) string {
	t := p.name
	if p.typeName != "" {
		t = p.typeName + ": " + p.name
	}
	if p.description != "" {
		if t != "" {
			t += " — " + p.description
		} else {
			t = p.description
		}
	}
	return t
}

// distinctChunkIDs / distinctDatasetIDs gather the recall scope from locate hits.
func distinctChunkIDs(hits []snippetHit) []string {
	seen := make(map[string]bool, len(hits))
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		id := h.chunk.ID
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func distinctDatasetIDs(hits []snippetHit) []string {
	seen := make(map[string]bool, len(hits))
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		id := h.chunk.DatasetID
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// tryFormatLocateCompiled serves the compiled products for a locate result set.
// It returns ok=false when no product was found, so the caller can passthrough
// the raw snippet (the failure philosophy: never hand the model an empty result
// that would make it re-search and inflate tokens).
func tryFormatLocateCompiled(ctx context.Context, tool, query string, hits []snippetHit, notices ...string) (string, bool) {
	products := fetchCompiledProductsFn(ctx, distinctDatasetIDs(hits), distinctChunkIDs(hits))
	if len(products) == 0 {
		return "", false
	}
	reg := evidenceRegistryFrom(ctx)
	compiledCount := make(map[string]int, len(products))
	for _, p := range products {
		for _, cid := range p.chunkIDs {
			compiledCount[cid]++
		}
	}
	mode := compiledServeMode()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<search_results count=\"%d\" query=\"%s\">\n",
		len(hits), xmlEscape(query)))
	for _, notice := range notices {
		b.WriteString(notice)
		b.WriteString("\n")
	}
	for i, h := range hits {
		c := h.chunk
		compAttr := ""
		if n, ok := compiledCount[c.ID]; ok {
			compAttr = fmt.Sprintf(` compiled="%d"`, n)
		}
		// Metadata row only: rank/score/handle, NO <match_snippet> — the raw
		// text is deliberately withheld; the model reads the compiled block.
		b.WriteString(fmt.Sprintf(
			"<chunk rank=\"%d\" chunk_id=\"%s\" doc_id=\"%s\" page_num=\"%d\" chunk_index=\"%d\" dataset_id=\"%s\" doc_name=\"%s\" score=\"%.3f\" chunk_runes=\"%d\"%s>\n",
			i+1,
			xmlEscape(c.ID), xmlEscape(c.DocumentID), c.PageNum, c.ChunkIndex,
			xmlEscape(c.DatasetID), xmlEscape(c.DocumentName), c.Score,
			utf8.RuneCountInString(c.Content), compAttr,
		))
		b.WriteString("</chunk>\n")
	}
	b.WriteString(fmt.Sprintf("<compiled_products count=\"%d\">\n", len(products)))
	for _, p := range products {
		text := compileProductText(p)
		refAttr := ""
		if n, _ := reg.ServeText(p.id, text); n >= 0 {
			refAttr = fmt.Sprintf(` ref="%d"`, n)
		}
		kind := xmlEscape(p.kind)
		if kind == "" {
			kind = "compiled"
		}
		b.WriteString(fmt.Sprintf("<product kind=\"%s\" doc_id=\"%s\"%s>%s</product>\n",
			kind, xmlEscape(p.docID), refAttr, xmlEscape(text)))
	}
	b.WriteString("</compiled_products>\n")
	if mode == csFirst {
		b.WriteString("<deep_read_hint>If the compiled_products above do not contain the fact you need, read the ORIGINAL chunk that a product was derived from: call list_chunks(doc_id, anchor_chunk_ids=[that chunk_id], number_neighbors=0) — number_neighbors=0 returns EXACTLY that one chunk's original text, NOT the surrounding document (do not expand to the whole article). The chunk handles (chunk_id) and their compiled=\"N\" counts are in the chunk rows above. Only read a chunk whose compiled product failed to answer you.</deep_read_hint>\n")
	}
	if len(hits) == 0 {
		b.WriteString("<hint>0 hits — the corpus may use a different WORD FORM of your terms. Retry with variants before concluding the corpus lacks the answer.</hint>\n")
	}
	b.WriteString("</search_results>")
	return b.String(), true
}

// CompiledServeChunks applies compiled-serve to the standard (non-agentic)
// retrieval result that chat_pipeline.go feeds to kbPrompt. It returns the
// transformed chunk list, or nil when compiled-serve is inactive or no product
// was found for any recalled chunk (the caller then keeps the raw chunks). The
// only mode swaps each chunk's model-visible text for its compiled products; the
// first mode prepends the products and keeps the original text so the model can
// fall back to the raw chunk.
func CompiledServeChunks(ctx context.Context, tenantID string, chunks []map[string]interface{}, datasetIDs []string) []map[string]interface{} {
	mode := compiledServeMode()
	if mode == csOff || len(chunks) == 0 {
		return nil
	}
	// The standard retrieval path does not publish a decision scope; the compiled
	// store lookup needs the tenant to resolve the index name.
	ctx = withDecisionScope(ctx, tenantID, "")
	chunkIDs := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if id := chunkIDOf(c); id != "" {
			chunkIDs = append(chunkIDs, id)
		}
	}
	products := fetchCompiledProductsFn(ctx, datasetIDs, chunkIDs)
	if len(products) == 0 {
		common.InfoCtx(ctx, "agentic_rag: compiled serve (standard) no products",
			zap.Int("chunks", len(chunkIDs)))
		return nil
	}
	byChunk := map[string][]compiledRow{}
	for _, p := range products {
		for _, cid := range p.chunkIDs {
			byChunk[cid] = append(byChunk[cid], p)
		}
	}
	out := make([]map[string]interface{}, 0, len(chunks))
	served := 0
	for _, c := range chunks {
		id := chunkIDOf(c)
		nc := shallowCopyChunk(c)
		prods := byChunk[id]
		if len(prods) == 0 {
			out = append(out, c)
			continue
		}
		org := ""
		if v, _ := c["content_with_weight"].(string); v != "" {
			org = v
		} else if v, _ := c["content"].(string); v != "" {
			org = v
		}
		block := renderCompiledProductBlock(ctx, prods, mode)
		var text string
		if mode == csOnly {
			text = block
		} else {
			text = block + "\n\n" + org
		}
		setChunkModelContent(nc, text)
		out = append(out, nc)
		served++
	}
	common.InfoCtx(ctx, "agentic_rag: compiled serve (standard) applied",
		zap.Int("chunks", len(out)), zap.Int("served", served), zap.Int("products", len(products)))
	return out
}

func chunkIDOf(c map[string]interface{}) string {
	if id, _ := c["id"].(string); id != "" {
		return id
	}
	if id, _ := c["chunk_id"].(string); id != "" {
		return id
	}
	return ""
}

func shallowCopyChunk(c map[string]interface{}) map[string]interface{} {
	nc := make(map[string]interface{}, len(c))
	for k, v := range c {
		nc[k] = v
	}
	return nc
}

func setChunkModelContent(c map[string]interface{}, text string) {
	c["content_with_weight"] = text
	c["content"] = text
}

// renderCompiledProductBlock renders the <compiled_products> block (and, in
// first mode, the deep-read hint) for a set of products derived from one chunk.
func renderCompiledProductBlock(ctx context.Context, prods []compiledRow, mode csMode) string {
	var b strings.Builder
	reg := evidenceRegistryFrom(ctx)
	b.WriteString(fmt.Sprintf("<compiled_products count=\"%d\">\n", len(prods)))
	for _, p := range prods {
		txt := compileProductText(p)
		refAttr := ""
		if n, _ := reg.ServeText(p.id, txt); n >= 0 {
			refAttr = fmt.Sprintf(` ref="%d"`, n)
		}
		kind := xmlEscape(p.kind)
		if kind == "" {
			kind = "compiled"
		}
		b.WriteString(fmt.Sprintf("<product kind=\"%s\" doc_id=\"%s\"%s>%s</product>\n",
			kind, xmlEscape(p.docID), refAttr, xmlEscape(txt)))
	}
	b.WriteString("</compiled_products>\n")
	if mode == csFirst {
		b.WriteString("<deep_read_hint>If the compiled_products above do not contain the fact you need, read the ORIGINAL chunk that a product was derived from: call list_chunks(doc_id, anchor_chunk_ids=[that chunk_id], number_neighbors=0) — number_neighbors=0 returns EXACTLY that one chunk's original text, NOT the surrounding document. Only read a chunk whose compiled product failed to answer you.</deep_read_hint>\n")
	}
	return b.String()
}

// tryFormatChunksCompiled serves the compiled products of the chunks a deep-read
// just fetched (only mode). A chunk with no compiled derivative keeps its raw
// content (logged) so the read is never empty; ok=false means the whole set had
// no derivatives and the caller should serve raw.
func tryFormatChunksCompiled(ctx context.Context, docID string, chunks []runtime.RetrievalChunk, anchorMeta, notice string) (string, bool) {
	ds := make([]string, 0, len(chunks))
	cids := make([]string, 0, len(chunks))
	seen := make(map[string]bool)
	for _, c := range chunks {
		if c.DatasetID != "" && !seen[c.DatasetID] {
			seen[c.DatasetID] = true
			ds = append(ds, c.DatasetID)
		}
		if c.ID != "" {
			cids = append(cids, c.ID)
		}
	}
	products := fetchCompiledProductsFn(ctx, ds, cids)
	if len(products) == 0 {
		return "", false
	}
	byChunk := make(map[string][]compiledRow, len(chunks))
	for _, p := range products {
		for _, cid := range p.chunkIDs {
			byChunk[cid] = append(byChunk[cid], p)
		}
	}
	reg := evidenceRegistryFrom(ctx)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<chunks doc_id=\"%s\" fetched=\"%d\"%s>\n",
		xmlEscape(docID), len(chunks), anchorMeta))
	if notice != "" {
		b.WriteString(fmt.Sprintf("<notice>%s</notice>\n", xmlEscape(notice)))
	}
	for _, c := range chunks {
		b.WriteString(fmt.Sprintf(
			"<chunk chunk_id=\"%s\" doc_id=\"%s\" page_num=\"%d\" chunk_index=\"%d\" dataset_id=\"%s\" doc_name=\"%s\">\n",
			xmlEscape(c.ID), xmlEscape(c.DocumentID), c.PageNum, c.ChunkIndex,
			xmlEscape(c.DatasetID), xmlEscape(c.DocumentName)))
		if prods := byChunk[c.ID]; len(prods) > 0 {
			for _, p := range prods {
				text := compileProductText(p)
				refAttr := ""
				if n, _ := reg.ServeText(p.id, text); n >= 0 {
					refAttr = fmt.Sprintf(` ref="%d"`, n)
				}
				kind := xmlEscape(p.kind)
				if kind == "" {
					kind = "compiled"
				}
				b.WriteString(fmt.Sprintf("<content kind=\"%s\"%s>%s</content>\n", kind, refAttr, xmlEscape(text)))
			}
		} else {
			// No derivative for this chunk: fall back to the raw body.
			b.WriteString(fmt.Sprintf("<content>%s</content>\n", xmlEscape(c.Content)))
		}
		b.WriteString("</chunk>\n")
	}
	b.WriteString("</chunks>")
	return b.String(), true
}
