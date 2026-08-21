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
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/agent/tool"
)

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
func readingOrderLess(a, b tool.RetrievalChunk) bool {
	if a.DocumentID != b.DocumentID {
		return a.DocumentID < b.DocumentID
	}
	if a.PageNum != b.PageNum {
		return a.PageNum < b.PageNum
	}
	return a.ChunkIndex < b.ChunkIndex
}

// The small helpers below are local copies of unexported utility functions in
// internal/agent/tool that the agentic RAG tools depend on. They are kept here
// so this package does not need to export/relocate the canvas tool package's
// internals.

// stringFromMap extracts a string value from a raw map by key.
func stringFromMap(raw map[string]any, key string) string {
	if v, ok := raw[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// firstStringFromMap returns the first non-empty string among the given keys.
func firstStringFromMap(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringFromMap(raw, key); value != "" {
			return value
		}
	}
	return ""
}

// intFromMap extracts an int value from a raw map by key, tolerating the
// numeric types ES decodes JSON numbers as (float64) as well as int/int64.
func intFromMap(raw map[string]any, key string) int {
	v, ok := raw[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

// jsonChunksEmpty returns an empty chunk list in the same JSON shape as
// marshalSearchResult.
func jsonChunksEmpty() string {
	return `{"chunks":[]}`
}

// marshalSearchResult serialises retrieval chunks into the compact JSON shape
// consumed by the model. (Retained for callers that expect JSON output; the
// agentic retrieval tools emit XML via formatSearchResultsXML.)
func marshalSearchResult(chunks []tool.RetrievalChunk) string {
	type outChunk struct {
		ID         string  `json:"id"`
		Content    string  `json:"content"`
		DocumentID string  `json:"doc_id"`
		DatasetID  string  `json:"dataset_id"`
		DocName    string  `json:"docnm_kwd"`
		Score      float64 `json:"similarity"`
	}
	out := make([]outChunk, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, outChunk{
			ID: c.ID, Content: c.Content, DocumentID: c.DocumentID,
			DatasetID: c.DatasetID, DocName: c.DocumentName, Score: c.Score,
		})
	}
	b, err := json.Marshal(map[string]interface{}{"chunks": out})
	if err != nil {
		return jsonChunksEmpty()
	}
	return string(b)
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
func formatSearchResultsXML(chunks []tool.RetrievalChunk) string {
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
func formatChunksXML(docID string, chunks []tool.RetrievalChunk, limit, offset int) string {
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
