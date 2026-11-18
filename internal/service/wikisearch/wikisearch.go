// Package wikisearch defines the compiled-wiki search service contract. It is a
// dependency-light leaf package so that both the agent tool layer
// (internal/agent/tool) and any concrete engine-backed implementation can depend
// on it without an import cycle — mirroring the internal/service/nav pattern.
//
// The interface abstracts two independent concerns:
//
//   - QueryPages: hybrid search over compiled wiki/artifact pages for a query,
//     returning rendered page content + slug/title + stable doc aggregation.
//   - BackfillChunks: fetch the ORIGINAL source chunks a compiled page was built
//     from, by chunk id, scoped to tenant + datasets. This is how the harness P7
//     evidence expansion gets the raw evidence (the general retrieval Search API
//     cannot fetch by chunk id).
//   - AvailableFor: whether a dataset actually has wiki artifacts the tool may
//     search, so the production runner only selects the wiki path when the bound
//     KBs carry the artifact (Python's compilation_available gate).
//
// The concrete implementation lives in the same package
// (engineWikiService, see engine_service.go) and is backed by the document
// engine. It is installed at server bootstrap via SetService.
package wikisearch

import "context"

// SearchResult is the normalized wiki_query return shape (mirrors Python's
// {"answer":"", "chunks":[...], "doc_aggs":[...]}).
//
// Each chunk is a map with at least: chunk_id, content_with_weight, doc_id,
// docnm_kwd, dataset_id, and (for a compiled page) wiki_slug_kwd.
type SearchResult struct {
	Chunks  []map[string]interface{}
	DocAggs []map[string]interface{}
}

// Service is the single read entrypoint the wiki_query agent tool and the
// production runner use.
type Service interface {
	// AvailableFor reports whether any of the given datasets carries searchable
	// wiki/artifact pages (Python's compilation_available gate). Returning false
	// means the wiki tool should not be selected for those datasets.
	AvailableFor(ctx context.Context, tenantID string, datasetIDs []string) bool
	// QueryPages hybrid-searches compiled wiki/artifact pages across the given
	// datasets and returns rendered page chunks. Returns an empty result (never
	// an error) when nothing matches or the backend is unavailable, so the agent
	// can fall back to hybrid search.
	QueryPages(ctx context.Context, tenantID string, datasetIDs []string, query, keywords string, topN int) (SearchResult, error)
	// BackfillChunks fetches original source chunks by their ids, scoped to the
	// tenant and datasets. It returns only chunks it could resolve, in the given
	// order, deduped by id; unresolvable ids are skipped (never an error). Empty
	// when the backend is unavailable or none resolve.
	BackfillChunks(ctx context.Context, tenantID string, datasetIDs []string, chunkIDs []string) ([]map[string]interface{}, error)
}
