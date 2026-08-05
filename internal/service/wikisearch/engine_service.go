package wikisearch

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// compileKWDWikiPage is the canonical compile_kwd for compiled wiki pages. It
// must match Python's WIKI_PAGE_COMPILE_KWD ("wiki_page", wiki.py:1661) AND the
// Go compiler's variantCompileKWD[VariantWiki] (component.go), so both Python-
// and Go-produced wiki pages are surfaced by this service.
const compileKWDWikiPage = "wiki_page"

// tenantIndexName returns the tenant-scoped chunk index name
// ("ragflow_<tenantID>"), matching how the rest of the stack derives the index
// (internal/handler/dataset.go). The dataset IDs are passed as KB filters, NOT
// used as index names.
func tenantIndexName(tenantID string) string {
	return fmt.Sprintf("ragflow_%s", tenantID)
}

// engineWikiService is the concrete compiled-wiki search service, backed
// directly by the document engine.
//
//   - Every operation derives the chunk index from the tenantID
//     (ragflow_<tenantID>) and passes dataset IDs only as KB filters, so scope
//     can never cross tenants.
//   - QueryPages issues an engine Search restricted to compile_kwd="wiki_page"
//     (+ supported kinds) so ordinary source chunks are never relabeled as wiki
//     pages; the raw rows also carry source_chunk_ids, which are emitted for
//     P7/R3 evidence backfill.
//   - BackfillChunks fetches original chunks by id via GetChunk, scoped to the
//     tenant index + dataset IDs.
type engineWikiService struct {
	engine engine.DocEngine // may be nil -> degrade
}

// NewEngineService builds the concrete wiki search service from the document
// engine (engine.Get() in production; nil disables all operations gracefully).
func NewEngineService(docEngine engine.DocEngine) Service {
	return &engineWikiService{engine: docEngine}
}

func (s *engineWikiService) AvailableFor(ctx context.Context, tenantID string, datasetIDs []string) bool {
	if s.engine == nil || tenantID == "" || len(datasetIDs) == 0 {
		return false
	}
	// "Wiki available" must mean the bound KBs actually carry wiki pages, not
	// merely that a chunk store exists (a non-wiki KB would otherwise trigger a
	// needless empty wiki-query/fallback round trip). Do a bounded existence
	// search (Limit=1) filtered to compile_kwd="wiki_page", returning true if
	// any page row exists.
	res, err := s.engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{tenantIndexName(tenantID)},
		KbIDs:      datasetIDs,
		Limit:      1,
		Filter: map[string]interface{}{
			"compile_kwd":   compileKWDWikiPage,
			"available_int": 1,
		},
	})
	if err != nil {
		return false
	}
	return res != nil && len(res.Chunks) > 0
}

func (s *engineWikiService) QueryPages(ctx context.Context, tenantID string, datasetIDs []string, query, keywords string, topN int) (SearchResult, error) {
	if s.engine == nil || tenantID == "" || len(datasetIDs) == 0 || strings.TrimSpace(query) == "" {
		return SearchResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}}, nil
	}
	if topN <= 0 {
		topN = 12
	}
	text := query
	if kw := strings.TrimSpace(keywords); kw != "" {
		text = query + " " + kw
	}
	req := &types.SearchRequest{
		IndexNames: []string{tenantIndexName(tenantID)},
		KbIDs:      datasetIDs,
		Limit:      topN,
		// Select the exact projection we consume. Infinity's default projection
		// omits slug_kwd and source_chunk_ids (chunk.go:736-748); without them
		// the page slug is blank and P7 evidence backfill has no provenance.
		SelectFields: []string{
			"id", "kb_id", "doc_id", "docnm_kwd", "content_with_weight",
			"slug_kwd", "source_chunk_ids",
		},
		// Discriminate wiki pages by compile_kwd="wiki_page". There is NO
		// "kc_kind" column in the chunk schema (infinity_mapping.json:47-56), so
		// it must not be used as a filter. Sections (page.go kind:"section") are
		// stamped compile_kwd="wiki_section", so this filter returns pages only.
		Filter: map[string]interface{}{
			"compile_kwd":   compileKWDWikiPage,
			"available_int": 1,
		},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"content_with_weight^2", "title_tks^5", "content_ltks"},
				MatchingText: text,
				TopN:         topN,
			},
		},
	}
	res, err := s.engine.Search(ctx, req)
	if err != nil || res == nil || len(res.Chunks) == 0 {
		return SearchResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}}, nil
	}
	out := SearchResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}}
	seenDoc := map[string]bool{}
	for _, row := range res.Chunks {
		// Engine raw rows carry the chunk id under "id" (shimmed to _id) and the
		// dataset under "kb_id". Normalize explicitly to the agent chunk shape
		// (chunk_id/dataset_id) so a stable id and KB scope are never lost.
		id := firstString(row["id"])
		datasetID := firstString(row["kb_id"])
		content := firstString(row["content_with_weight"])
		if id == "" && content == "" {
			continue
		}
		docID := firstString(row["doc_id"])
		chunk := map[string]interface{}{
			"chunk_id": id, "content_with_weight": content,
			"doc_id": docID, "docnm_kwd": firstString(row["docnm_kwd"]),
			"dataset_id":    datasetID,
			"wiki_slug_kwd": firstString(row["slug_kwd"]),
		}
		if src := stringArray(row["source_chunk_ids"]); len(src) > 0 {
			chunk["source_chunk_ids"] = src
		}
		out.Chunks = append(out.Chunks, chunk)
		if docID != "" && !seenDoc[docID] {
			seenDoc[docID] = true
			out.DocAggs = append(out.DocAggs, map[string]interface{}{"doc_id": docID, "doc_name": firstString(row["docnm_kwd"])})
		}
	}
	return out, nil
}

func (s *engineWikiService) BackfillChunks(ctx context.Context, tenantID string, datasetIDs []string, chunkIDs []string) ([]map[string]interface{}, error) {
	if s.engine == nil || tenantID == "" || len(chunkIDs) == 0 {
		return nil, nil
	}
	const maxBackfill = 16
	seen := map[string]bool{}
	ids := make([]string, 0, len(chunkIDs))
	for _, id := range chunkIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if len(ids) >= maxBackfill {
			break
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		raw, err := s.engine.GetChunk(ctx, tenantIndexName(tenantID), id, datasetIDs)
		if err != nil {
			continue
		}
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		// GetChunk rows also carry id/kb_id (ES shims id to _id; source["id"]
		// set in chunk.go:1991). Normalize to the agent chunk shape.
		out = append(out, map[string]interface{}{
			"chunk_id": firstString(m["id"]), "content_with_weight": firstString(m["content_with_weight"]),
			"doc_id": firstString(m["doc_id"]), "docnm_kwd": firstString(m["docnm_kwd"]),
			"dataset_id": firstString(m["kb_id"]),
		})
	}
	return out, nil
}

// firstString returns the first string of a possibly array-shaped engine field
// value (the document engine surfaces keyword fields as arrays), matching the
// firstStringValue helper used by the artifact service.
func firstString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []string:
		if len(t) > 0 {
			return t[0]
		}
	case []interface{}:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func stringArray(v interface{}) []string {
	if raw, ok := v.([]string); ok {
		return raw
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
