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
		IndexNames:   []string{indexName},
		KbIDs:        []string{kbID},
		DocIDs:       docIDs,
		Page:         1,
		Size:         size,
		KeywordOnly:  true,
	}

	result, err := s.docEngine.Search(context.Background(), searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	searchResp, ok := result.(*types.SearchResponse)
	if !ok {
		return nil, fmt.Errorf("invalid search response type")
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
		IndexNames:   []string{indexName},
		KbIDs:        kbIDs,
		Page:         1,
		Size:         size,
		KeywordOnly:  true,
	}

	result, err := s.docEngine.Search(context.Background(), searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	searchResp, ok := result.(*types.SearchResponse)
	if !ok {
		return nil, fmt.Errorf("invalid search response type")
	}

	return &SearchMetadataResult{
		IndexName: indexName,
		Chunks:    searchResp.Chunks,
	}, nil
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
		metaFields = ParseLengthPrefixedJSON(v)
		if metaFields == nil {
			if err := json.Unmarshal(v, &metaFields); err != nil {
				return make(map[string]interface{}), nil
			}
		}
	default:
		return make(map[string]interface{}), nil
	}

	return metaFields, nil
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
