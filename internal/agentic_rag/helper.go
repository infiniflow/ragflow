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
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
)

// resolveDatasetScope keeps model-provided dataset ids within the
// conversation's server-bound scope. An omitted request uses the full bound
// scope; an explicit request must be a subset of it.
func resolveDatasetScope(bound, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return bound, nil
	}

	allowed := make(map[string]struct{}, len(bound))
	for _, id := range bound {
		allowed[id] = struct{}{}
	}
	for _, id := range requested {
		if _, ok := allowed[id]; !ok {
			return nil, fmt.Errorf("dataset_id %q is outside the conversation's bound scope", id)
		}
	}
	return requested, nil
}

// clampFloat01 clamps a float into [0, 1], used for similarity weights the model
// may supply out of range.
func clampFloat01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// grepChunksSelectFields is the ES _source fields the retrieval tools request.
// doc_id, page_num_int and chunk_order_int are the mandated reading-order
// identifiers; content_with_weight and the other fields feed the tool output.
// The regexp query matches content_with_weight, so it must stay in the list.
var grepChunksSelectFields = []string{
	"content_with_weight",
	"doc_id",
	"docnm_kwd",
	"kb_id",
	"page_num_int",
	"chunk_order_int",
}

// grepChunksSortFields is the reading-order sort used by grep_chunks and
// list_chunks: by document, then page, then chunk within a page.
var grepChunksSortFields = []string{"doc_id", "page_num_int", "chunk_order_int"}

// readingOrderLess orders chunks by document, then page, then chunk index —
// matching the doc_id / page_num_int / chunk_order_int sort the engine applies.
func readingOrderLess(a, b runtime.RetrievalChunk) bool {
	if a.DocumentID != b.DocumentID {
		return a.DocumentID < b.DocumentID
	}
	if a.PageNum != b.PageNum {
		return a.PageNum < b.PageNum
	}
	return a.ChunkIndex < b.ChunkIndex
}

// searchResultsXMLEmpty returns an empty semantic-search result set in the same
// XML shape as formatSearchResultsXML.
func searchResultsXMLEmpty() string {
	return `<search_results count="0">
</search_results>`
}

// formatSearchResultsXML serialises semantic-retrieval chunks into the compact
// XML shape consumed by the model, matching grep_chunks' tag vocabulary so the
// agent sees one consistent output convention across all retrieval tools.
// Terminology is uniform: dataset_id is the owning dataset, doc_id is the
// owning document, doc_title is the document name.
func formatSearchResultsXML(chunks []runtime.RetrievalChunk) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<search_results count=\"%d\">\n", len(chunks)))
	for i, c := range chunks {
		b.WriteString(fmt.Sprintf(
			"<chunk rank=\"%d\" chunk_id=\"%s\" doc_id=\"%s\" page_num=\"%d\" chunk_index=\"%d\" dataset_id=\"%s\" doc_title=\"%s\" score=\"%.3f\">\n",
			i+1,
			xmlEscape(c.ID), xmlEscape(c.DocumentID), c.PageNum, c.ChunkIndex,
			xmlEscape(c.DatasetID), xmlEscape(c.DocumentName), c.Score,
		))
		if c.Content != "" {
			b.WriteString(fmt.Sprintf("<content>%s</content>\n", xmlEscape(c.Content)))
		}
		b.WriteString("</chunk>\n")
	}
	b.WriteString("</search_results>")
	return b.String()
}

// formatChunksXML serialises the ordered chunks of a single document into the
// compact XML shape consumed by the model, including pagination metadata so the
// model knows when to request the next page. Terminology is uniform with the
// other retrieval tools: dataset_id (dataset), doc_id (document).
func formatChunksXML(docID string, chunks []runtime.RetrievalChunk, limit, offset int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<chunks doc_id=\"%s\" offset=\"%d\" limit=\"%d\" fetched=\"%d\">\n",
		xmlEscape(docID), offset, limit, len(chunks)))
	for _, c := range chunks {
		b.WriteString(fmt.Sprintf(
			"<chunk chunk_id=\"%s\" doc_id=\"%s\" page_num=\"%d\" chunk_index=\"%d\" dataset_id=\"%s\" doc_title=\"%s\">\n",
			xmlEscape(c.ID), xmlEscape(c.DocumentID), c.PageNum, c.ChunkIndex,
			xmlEscape(c.DatasetID), xmlEscape(c.DocumentName)))
		if c.Content != "" {
			b.WriteString(fmt.Sprintf("<content>%s</content>\n", xmlEscape(c.Content)))
		}
		b.WriteString("</chunk>\n")
	}
	if len(chunks) == limit && len(chunks) > 0 {
		b.WriteString(fmt.Sprintf("<pagination next_offset=\"%d\" remaining=\"true\" />\n", offset+limit))
	}
	b.WriteString("</chunks>")
	return b.String()
}
