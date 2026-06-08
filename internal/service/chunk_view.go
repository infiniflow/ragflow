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

import "encoding/json"

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
