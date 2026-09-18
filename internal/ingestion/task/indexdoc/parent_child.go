package indexdoc

import (
	"fmt"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// MaterializeParentChunks turns the chunker's transient parent text into
// hidden index rows and links every child row to its parent. The parent ID is
// deliberately xxhash64(mom) without document scope to match Python.
func MaterializeParentChunks(chunks []map[string]any) []map[string]any {
	parentsByID := make(map[string]map[string]any)
	parentIDs := make([]string, 0)

	for _, chunk := range chunks {
		mom := parentText(chunk)
		delete(chunk, "mom")
		delete(chunk, "mom_with_weight")
		if mom == "" {
			continue
		}

		parentID := fmt.Sprintf("%016x", xxhash.Sum64String(mom))
		chunk["mom_id"] = parentID
		if _, exists := parentsByID[parentID]; exists {
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
		parentsByID[parentID] = parent
		parentIDs = append(parentIDs, parentID)
	}

	parents := make([]map[string]any, 0, len(parentIDs))
	for _, parentID := range parentIDs {
		parents = append(parents, parentsByID[parentID])
	}
	return parents
}

func parentText(chunk map[string]any) string {
	for _, field := range []string{"mom", "mom_with_weight"} {
		if value, ok := chunk[field].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
