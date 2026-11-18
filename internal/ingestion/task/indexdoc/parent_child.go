package indexdoc

import (
	"fmt"
	"strings"

	"github.com/cespare/xxhash/v2"

	"ragflow/internal/common"
)

// MaterializeParentChunks turns the chunker's transient parent text into
// hidden index rows and links every child row to its parent. A parent ID is
// scoped to its dataset and document so equal parent text from separate
// documents or datasets cannot overwrite each other in a shared index.
//
// Every parent row also receives a chunk_order_int from the range reserved for
// parents (common.ParentChunkOrderBase + k, where k follows the order in which
// the parents are discovered, i.e. the reading order of the first child
// referencing each one), while source chunks stay in [0, ParentChunkOrderBase).
// That keeps every source chunk — the ones just ingested and the ones a user
// adds later through AddChunk — ahead of every parent on every engine:
// Elasticsearch already placed parents last because the field was simply absent
// on the parent row (a missing sort key counts as greatest), but Infinity cannot
// express "absent": an unwritten column falls back to the mapping default 0
// (conf/infinity_mapping.json), which tied every parent with the first source
// chunk, so the management chunk list listed parents first and sliced that tie
// group differently on every page (LIMIT/OFFSET).
//
// Documents whose chunks carry no chunk_order_int at all are left alone: the
// parent rows keep the field absent so the Elasticsearch placement is preserved
// and no ordering is invented for pipelines that never numbered their chunks.
func MaterializeParentChunks(datasetID string, chunks []map[string]any) []map[string]any {
	seen := make(map[string]struct{})
	parents := make([]map[string]any, 0)
	order := common.ParentChunkOrderBase
	ordered := chunksOrdered(chunks)

	for _, chunk := range chunks {
		mom := parentText(chunk)
		delete(chunk, "mom")
		delete(chunk, "mom_with_weight")
		if mom == "" {
			continue
		}

		docID, _ := chunk["doc_id"].(string)
		if docID == "" {
			continue
		}
		parentID := parentChunkID(datasetID, docID, mom)
		chunk["mom_id"] = parentID
		if _, exists := seen[parentID]; exists {
			continue
		}

		parent := map[string]any{
			"id":                  parentID,
			"content_with_weight": mom,
			"available_int":       0,
		}
		for _, field := range []string{
			"doc_id", "docnm_kwd", "position_int", "create_timestamp_flt",
			"page_num_int", "top_int",
		} {
			if value, exists := chunk[field]; exists {
				parent[field] = value
			}
		}
		if ordered {
			parent["chunk_order_int"] = order
			order++
		}
		seen[parentID] = struct{}{}
		parents = append(parents, parent)
	}
	return parents
}

// chunksOrdered reports whether the document's source chunks are numbered at
// all. The tokenizer numbers every surviving chunk; a pipeline without one
// leaves the field unset, and those documents keep their previous placement
// instead of getting invented order values.
func chunksOrdered(chunks []map[string]any) bool {
	for _, chunk := range chunks {
		if _, ok := common.GetInt(chunk["chunk_order_int"]); ok {
			return true
		}
	}
	return false
}

// parentChunkID returns the deterministic parent row identifier. NUL
// separators make the dataset, document, and parent-text boundaries
// unambiguous.
func parentChunkID(datasetID, docID, mom string) string {
	hasher := xxhash.New()
	_, _ = hasher.WriteString(datasetID)
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.WriteString(docID)
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.WriteString(mom)
	return fmt.Sprintf("%016x", hasher.Sum64())
}

func parentText(chunk map[string]any) string {
	for _, field := range []string{"mom", "mom_with_weight"} {
		if value, ok := chunk[field].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
