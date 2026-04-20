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
	"context"
	"encoding/json"
	"fmt"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// MetadataService provides common metadata operations
type MetadataService struct {
	kbDAO     *dao.KnowledgebaseDAO
	docEngine engine.DocEngine
}

// NewMetadataService creates a new metadata service
func NewMetadataService() *MetadataService {
	return &MetadataService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		docEngine: engine.Get(),
	}
}

// BuildMetadataIndexName constructs the metadata index name for a tenant
func BuildMetadataIndexName(tenantID string) string {
	return fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
}

// GetTenantIDByKBID retrieves tenant ID from knowledge base ID
func (s *MetadataService) GetTenantIDByKBID(kbID string) (string, error) {
	kb, err := s.kbDAO.GetByID(kbID)
	if err != nil {
		return "", fmt.Errorf("knowledgebase not found: %w", err)
	}
	return kb.TenantID, nil
}

// GetTenantIDByKBIDs retrieves tenant ID from the first knowledge base ID in the list
func (s *MetadataService) GetTenantIDByKBIDs(kbIDs []string) (string, error) {
	if len(kbIDs) == 0 {
		return "", fmt.Errorf("no kb_ids provided")
	}
	kb, err := s.kbDAO.GetByID(kbIDs[0])
	if err != nil {
		return "", fmt.Errorf("knowledgebase not found: %w", err)
	}
	return kb.TenantID, nil
}

// SearchMetadataResult holds the result of a metadata search
type SearchMetadataResult struct {
	IndexName string
	Chunks    []map[string]interface{}
}

// SearchMetadata searches the metadata index with the given parameters
func (s *MetadataService) SearchMetadata(kbID, tenantID string, docIDs []string, size int) (*SearchMetadataResult, error) {
	indexName := BuildMetadataIndexName(tenantID)

	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{kbID},
		DocIDs:     docIDs,
		Offset:     0,
		Limit:      size,
	}

	searchResp, err := s.docEngine.Search(context.Background(), searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return &SearchMetadataResult{
		IndexName: indexName,
		Chunks:    searchResp.Chunks,
	}, nil
}

// SearchMetadataByKBs searches the metadata index for multiple knowledge bases
func (s *MetadataService) SearchMetadataByKBs(kbIDs []string, size int) (*SearchMetadataResult, error) {
	if len(kbIDs) == 0 {
		return &SearchMetadataResult{Chunks: []map[string]interface{}{}}, nil
	}

	tenantID, err := s.GetTenantIDByKBIDs(kbIDs)
	if err != nil {
		return nil, err
	}

	indexName := BuildMetadataIndexName(tenantID)

	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      kbIDs,
		Offset:     0,
		Limit:      size,
	}

	searchResp, err := s.docEngine.Search(context.Background(), searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return &SearchMetadataResult{
		IndexName: indexName,
		Chunks:    searchResp.Chunks,
	}, nil
}

// GetFlattedMetaByKBs returns flattened metadata in the format:
// {field_name: {value: [doc_ids]}}
// This corresponds to api/db/services/doc_metadata_service.py:get_flatted_meta_by_kbs()
func (s *MetadataService) GetFlattedMetaByKBs(kbIDs []string) (map[string]interface{}, error) {
	if len(kbIDs) == 0 {
		return make(map[string]interface{}), nil
	}

	// Get metadata for all docs in KBs (use large limit like Python's 10000)
	result, err := s.SearchMetadataByKBs(kbIDs, 10000)
	if err != nil {
		return nil, err
	}

	// Build flattened metadata: {field: {value: [doc_ids]}}
	flattedMeta := make(map[string]interface{})

	for _, chunk := range result.Chunks {
		// Extract doc_id from chunk
		docID := ""
		if id, ok := chunk["id"].(string); ok {
			docID = id
		} else if id, ok := chunk["doc_id"].(string); ok {
			docID = id
		}

		if docID == "" {
			continue
		}

		// Extract metadata fields
		metaFields, err := ExtractMetaFields(chunk)
		if err != nil || len(metaFields) == 0 {
			continue
		}

		// Flatten each field
		for fieldName, fieldValue := range metaFields {
			if fieldValue == nil {
				continue
			}

			// Initialize field map if not exists
			if _, exists := flattedMeta[fieldName]; !exists {
				flattedMeta[fieldName] = make(map[string]interface{})
			}

			valueMap, ok := flattedMeta[fieldName].(map[string]interface{})
			if !ok {
				continue
			}

			// Handle different value types
			switch v := fieldValue.(type) {
			case string:
				// Single string value
				if v == "" {
					continue
				}
				if _, exists := valueMap[v]; !exists {
					valueMap[v] = []string{docID}
				} else {
					valueMap[v] = appendDocID(valueMap[v], docID)
				}
			case []interface{}:
				// List of values
				for _, item := range v {
					if itemStr, ok := item.(string); ok && itemStr != "" {
						if _, exists := valueMap[itemStr]; !exists {
							valueMap[itemStr] = []string{docID}
						} else {
							valueMap[itemStr] = appendDocID(valueMap[itemStr], docID)
						}
					}
				}
			case map[string]interface{}:
				// Nested object - flatten it
				for nestedKey, nestedValue := range v {
					if nestedStr, ok := nestedValue.(string); ok && nestedStr != "" {
						fullKey := fieldName + "." + nestedKey
						if _, exists := flattedMeta[fullKey]; !exists {
							flattedMeta[fullKey] = make(map[string]interface{})
						}
						if subMap, ok := flattedMeta[fullKey].(map[string]interface{}); ok {
							if _, exists := subMap[nestedStr]; !exists {
								subMap[nestedStr] = []string{docID}
							} else {
								if ids, ok := subMap[nestedStr].([]string); ok {
									subMap[nestedStr] = append(ids, docID)
								}
							}
						}
					}
				}
			}
		}
	}

	return flattedMeta, nil
}

// ExtractDocumentID extracts the document ID from a chunk
func ExtractDocumentID(chunk map[string]interface{}) (string, bool) {
	docID, ok := chunk["id"].(string)
	return docID, ok
}

// ExtractMetaFields extracts meta_fields from a chunk, handling different types
func ExtractMetaFields(chunk map[string]interface{}) (map[string]interface{}, error) {
	metaFieldsVal := chunk["meta_fields"]
	if metaFieldsVal == nil {
		return make(map[string]interface{}), nil
	}

	var metaFields map[string]interface{}
	switch v := metaFieldsVal.(type) {
	case map[string]interface{}:
		metaFields = v
	case string:
		if err := json.Unmarshal([]byte(v), &metaFields); err != nil {
			return make(map[string]interface{}), nil
		}
	case []byte:
		// Try length-prefixed format (Infinity stores data this way)
		allResults := ParseAllLengthPrefixedJSON(v)
		if len(allResults) > 0 {
			// Merge all JSON objects - when same key appears with different values, collect all
			metaFields = make(map[string]interface{})
			for _, result := range allResults {
				for k, val := range result {
					if existing, exists := metaFields[k]; exists {
						// Key already exists - merge values
						metaFields[k] = mergeFieldValues(existing, val)
					} else {
						metaFields[k] = val
					}
				}
			}
		} else if err := json.Unmarshal(v, &metaFields); err != nil {
			return make(map[string]interface{}), nil
		}
	default:
		return make(map[string]interface{}), nil
	}

	return metaFields, nil
}

// mergeFieldValues merges two field values when the same key appears multiple times
// If both are arrays, append all elements. If one is array and other is string, append string to array.
// Returns []interface{} with all merged values (flattened).
func mergeFieldValues(existing, new interface{}) []interface{} {
	result := []interface{}{}

	var addValue func(v interface{})
	addValue = func(v interface{}) {
		if v == nil {
			return
		}
		switch val := v.(type) {
		case string:
			if val != "" {
				result = append(result, val)
			}
		case []interface{}:
			for _, item := range val {
				addValue(item)
			}
		}
	}

	addValue(existing)
	addValue(new)

	return result
}

// appendDocID appends a docID to an existing value that may be []string or []interface{}
func appendDocID(existing interface{}, docID string) []string {
	result := []string{docID}
	if existing == nil {
		return result
	}
	switch v := existing.(type) {
	case []string:
		return append(v, docID)
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return append(result, v)
	}
	return result
}

// ParseLengthPrefixedJSON parses Infinity's length-prefixed JSON format
// Format: [4-byte length (little-endian)][JSON][4-byte length][JSON]...
// Returns the FIRST valid JSON object found
func ParseLengthPrefixedJSON(data []byte) map[string]interface{} {
	if len(data) < 4 {
		return nil
	}

	// Try to find the first valid JSON object by skipping length prefixes
	offset := 0
	for offset < len(data) {
		// Skip non-'{' bytes
		for offset < len(data) && data[offset] != '{' {
			offset++
		}
		if offset >= len(data) {
			break
		}

		// Try to parse JSON from current position
		var result map[string]interface{}
		err := json.Unmarshal(data[offset:], &result)
		if err == nil {
			return result
		}

		// Move forward to try next position
		offset++
	}
	return nil
}

// ParseAllLengthPrefixedJSON parses Infinity's length-prefixed JSON format
// and returns ALL JSON objects found (for cases where multiple rows are concatenated)
// Format: [4-byte length (little-endian)][JSON][4-byte length][JSON]...
func ParseAllLengthPrefixedJSON(data []byte) []map[string]interface{} {
	if len(data) < 4 {
		return nil
	}

	var results []map[string]interface{}
	offset := 0

	// Use length prefix to extract each JSON
	for offset+4 <= len(data) {
		// Read 4-byte length (little-endian)
		length := uint32(data[offset]) | uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24

		// Check if length looks reasonable
		if length == 0 || offset+4+int(length) > len(data) {
			// Length invalid, try to find next '{'
			nextBrace := -1
			for i := offset + 4; i < len(data) && i < offset+104; i++ {
				if data[i] == '{' {
					nextBrace = i
					break
				}
			}
			if nextBrace > offset {
				offset = nextBrace
				continue
			}
			break
		}

		// Extract JSON bytes (skip the 4-byte length prefix)
		jsonStart := offset + 4
		jsonEnd := jsonStart + int(length)
		jsonBytes := data[jsonStart:jsonEnd]

		var result map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &result); err == nil {
			results = append(results, result)
			offset = jsonEnd
			continue
		} else {
			// Try to find next '{'
			nextBrace := -1
			for i := offset + 4; i < len(data) && i < offset+104; i++ {
				if data[i] == '{' {
					nextBrace = i
					break
				}
			}
			if nextBrace > offset {
				offset = nextBrace
				continue
			}
			break
		}
	}
	return results
}
