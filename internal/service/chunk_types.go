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

package service

import (
	"encoding/json"
	"fmt"

	"ragflow/internal/common"
)

// RetrievalTestRequest retrieval test request
type RetrievalTestRequest struct {
	Datasets               common.StringSlice     `json:"dataset_ids" binding:"required"` // string or []string
	Question               string                 `json:"question"`
	Page                   *int                   `json:"page,omitempty"`
	Size                   *int                   `json:"size,omitempty"`
	RerankCandidatesCount  *int                   `json:"rerank_candidates_count,omitempty"`
	DocIDs                 []string               `json:"doc_ids,omitempty"`
	UseKG                  *bool                  `json:"use_kg,omitempty"`
	TopK                   *int                   `json:"top_k,omitempty"`
	CrossLanguages         []string               `json:"cross_languages,omitempty"`
	SearchID               *string                `json:"search_id,omitempty"`
	Filter                 map[string]interface{} `json:"meta_data_filter,omitempty"`
	TenantRerankID         *string                `json:"tenant_rerank_id,omitempty"`
	RerankID               *string                `json:"rerank_id,omitempty"`
	Keyword                *bool                  `json:"keyword,omitempty"`
	SimilarityThreshold    *float64               `json:"similarity_threshold,omitempty"`
	VectorSimilarityWeight *float64               `json:"vector_similarity_weight,omitempty"`
}

// RetrievalTestResponse retrieval test response
type RetrievalTestResponse struct {
	Chunks  []map[string]interface{} `json:"chunks"`
	DocAggs []map[string]interface{} `json:"doc_aggs"`
	Labels  *map[string]float64      `json:"labels"`
	Total   int64                    `json:"total"`
}

// GetChunkRequest request for getting a chunk by ID
type GetChunkRequest struct {
	ChunkID string `json:"chunk_id"`
}

// GetChunkResponse response for getting a chunk
type GetChunkResponse struct {
	Chunk map[string]interface{} `json:"chunk"`
}

// ParseFileRequest is the request body for reparsing documents in a dataset.
type ParseFileRequest struct {
	DocumentIDs []string `json:"document_ids"`
}

// AddChunkRequest request for adding a chunk
type AddChunkRequest struct {
	DatasetID         string      `json:"dataset_id"`
	DocumentID        string      `json:"document_id"`
	Content           string      `json:"content"`
	ImportantKeywords []string    `json:"important_keywords,omitempty"`
	Questions         []string    `json:"questions,omitempty"`
	TagKwd            []string    `json:"tag_kwd,omitempty"`
	TagFeas           interface{} `json:"tag_feas,omitempty"`
	ImageBase64       *string     `json:"image_base64,omitempty"`
}

// AddChunkResponse response for adding a chunk
type AddChunkResponse struct {
	Chunk map[string]interface{} `json:"chunk"`
}

// ErrorCoder exposes an application error code alongside an error string.
type ErrorCoder interface {
	error
	Code() common.ErrorCode
}

type StopParsingRequest struct {
	DocumentIDs []string `json:"document_ids"`
}

type StopParsingResponse struct {
	Data    map[string]interface{}
	Message string
}

func CheckDuplicateIDs(docList []string, idType string) ([]string, []string) {
	uniqueDocIDs := make([]string, 0)
	duplicateMessages := make([]string, 0)
	idCount := make(map[string]int)

	for _, docID := range docList {
		idCount[docID] += 1
	}

	for id, count := range idCount {
		if count > 1 {
			duplicateMessages = append(duplicateMessages, fmt.Sprintf("Duplicate %s ids: %s", idType, id))
		}
		uniqueDocIDs = append(uniqueDocIDs, id)
	}
	return uniqueDocIDs, duplicateMessages
}

func IndexName(uid string) string {
	return fmt.Sprintf("ragflow_%s", uid)
}

// ListChunksRequest request for listing chunks
type ListChunksRequest struct {
	DatasetID    string   `json:"dataset_id,omitempty"`
	DocID        string   `json:"doc_id" binding:"required"`
	ChunkIDs     []string `json:"chunk_ids,omitempty"`
	Page         *int     `json:"page,omitempty"`
	Size         *int     `json:"size,omitempty"`
	Keywords     string   `json:"keywords,omitempty"`
	AvailableInt *int     `json:"available_int,omitempty"`
}

// ListChunksResponse response for listing chunks
type ListChunksResponse struct {
	Chunks []map[string]interface{} `json:"chunks"`
	Doc    map[string]interface{}   `json:"doc"`
	Total  int64                    `json:"total"`
}

// UpdateChunkRequest request for updating a chunk
type UpdateChunkRequest struct {
	DatasetID    string        `json:"dataset_id"`
	DocumentID   string        `json:"document_id"`
	ChunkID      string        `json:"chunk_id"`
	Content      *string       `json:"content,omitempty"`
	ImportantKwd []string      `json:"important_keywords,omitempty"`
	Questions    []string      `json:"questions,omitempty"`
	Available    *bool         `json:"available,omitempty"`
	Positions    []interface{} `json:"positions,omitempty"`
	TagKwd       []string      `json:"tag_kwd,omitempty"`
	TagFeas      interface{}   `json:"tag_feas,omitempty"`
}

// RemoveChunksRequest request for removing chunks
type RemoveChunksRequest struct {
	DocID     string   `json:"doc_id"`
	ChunkIDs  []string `json:"chunk_ids,omitempty"`
	DeleteAll bool     `json:"delete_all,omitempty"`
}

// SourcedChunk is a typed, normalized view over a retrieval result chunk.
// It decouples the ask pipeline (KbPrompt, ChunksFormat) from the raw
// map[string]interface{} that flows through the retrieval engine.
type SourcedChunk struct {
	ID               string                 // chunk_id or id
	Content          string                 // content_with_weight or content
	DocID            string                 // doc_id or document_id
	DocName          string                 // docnm_kwd or document_name
	DatasetID        string                 // kb_id or dataset_id
	ImageID          string                 // image_id or img_id
	Positions        string                 // positions or position_int
	URL              string                 // url
	Similarity       float64                // similarity score
	VectorSimilarity float64                // vector_similarity score
	TermSimilarity   float64                // term_similarity score
	DocType          string                 // doc_type_kwd or doc_type
	DocumentMetadata map[string]interface{} // document_metadata
}

// NewSourcedChunks normalizes raw retrieval chunks into typed SourcedChunk values.
// It handles the key aliases used by different engine backends (ES, Infinity).
func NewSourcedChunks(raw []map[string]interface{}) []SourcedChunk {
	out := make([]SourcedChunk, 0, len(raw))
	for _, ck := range raw {
		if ck == nil {
			continue
		}
		out = append(out, SourcedChunk{
			ID:               getStr(ck, "chunk_id", "id"),
			Content:          getStr(ck, "content_with_weight", "content"),
			DocID:            getStr(ck, "doc_id", "document_id"),
			DocName:          getStr(ck, "docnm_kwd", "document_name"),
			DatasetID:        getStr(ck, "kb_id", "dataset_id"),
			ImageID:          getStr(ck, "image_id", "img_id"),
			Positions:        getStr(ck, "positions", "position_int"),
			URL:              getStr(ck, "url"),
			Similarity:       getFloat(ck, "similarity"),
			VectorSimilarity: getFloat(ck, "vector_similarity"),
			TermSimilarity:   getFloat(ck, "term_similarity"),
			DocType:          getStr(ck, "doc_type_kwd", "doc_type"),
			DocumentMetadata: getMap(ck, "document_metadata"),
		})
	}
	return out
}

// getStr tries each key in order and returns the first non-empty string value.
// The first key is the primary name; subsequent keys are fallback aliases
// used by different engine backends (e.g. "content_with_weight" vs "content").
func getStr(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// getFloat extracts a float64 value from the map, handling the various
// numeric types that different JSON decoders and engine drivers may produce
// (float64, float32, json.Number, int, int64).
func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch f := v.(type) {
		case float64:
			return f
		case float32:
			return float64(f)
		case json.Number:
			if n, err := f.Float64(); err == nil {
				return n
			}
		case int:
			return float64(f)
		case int64:
			return float64(f)
		}
	}
	return 0
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]interface{}); ok {
			// Return a shallow copy so callers cannot mutate the original chunk data.
			out := make(map[string]interface{}, len(mm))
			for k, val := range mm {
				out[k] = val
			}
			return out
		}
	}
	return nil
}
