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
	"strconv"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// KBDocIDsMap maps a KB ID to its document IDs.
// Example: {"kb1": ["doc1", "doc2"], "kb2": ["doc3"]}
type KBDocIDsMap map[string][]string

// DocMetaMap maps a document ID to its metadata fields.
// Example: {"doc1": {"author": "Zhang San", "date": "2024-01-01"}}
type DocMetaMap map[string]map[string]interface{}

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
	return dao.GetTenantIDByKBID(kbID)
}

// GetTenantIDByKBIDs retrieves tenant ID from the first knowledge base ID in the list
func (s *MetadataService) GetTenantIDByKBIDs(kbIDs []string) (string, error) {
	if len(kbIDs) == 0 {
		return "", fmt.Errorf("no kb_ids provided")
	}
	return dao.GetTenantIDByKBID(kbIDs[0])
}

// SearchMetadataResponse holds the result of a metadata search
type SearchMetadataResponse struct {
	IndexName       string
	MetadataRecords []map[string]interface{}
}

// SearchMetadata searches the metadata index with the given parameters
func (s *MetadataService) SearchMetadata(kbID, tenantID string, docIDs []string, size int) (*SearchMetadataResponse, error) {
	searchReq := &types.SearchMetadataRequest{
		TenantID: tenantID,
		Offset:   0,
		Limit:    size,
		Filter: map[string]interface{}{
			"id":    docIDs,
			"kb_id": kbID,
		},
	}

	searchResult, err := s.docEngine.SearchMetadata(context.Background(), searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return &SearchMetadataResponse{
		IndexName:       BuildMetadataIndexName(tenantID),
		MetadataRecords: searchResult.MetadataRecords,
	}, nil
}

// SearchMetadataByKBs searches the metadata index for multiple knowledge bases
func (s *MetadataService) SearchMetadataByKBs(kbIDs []string, size int) (*SearchMetadataResponse, error) {
	if len(kbIDs) == 0 {
		return &SearchMetadataResponse{MetadataRecords: []map[string]interface{}{}}, nil
	}

	tenantID, err := s.GetTenantIDByKBIDs(kbIDs)
	if err != nil {
		return nil, err
	}

	searchReq := &types.SearchMetadataRequest{
		TenantID: tenantID,
		Offset:   0,
		Limit:    size,
		Filter: map[string]interface{}{
			"kb_id": kbIDs,
		},
	}

	searchResult, err := s.docEngine.SearchMetadata(context.Background(), searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return &SearchMetadataResponse{
		IndexName:       BuildMetadataIndexName(tenantID),
		MetadataRecords: searchResult.MetadataRecords,
	}, nil
}

// GetFlattedMetaByKBs returns flattened metadata in the format:
// {field_name: {value: [doc_ids]}}
func (s *MetadataService) GetFlattedMetaByKBs(kbIDs []string) (common.MetaData, error) {
	if len(kbIDs) == 0 {
		return make(common.MetaData), nil
	}

	// Get metadata for all docs in KBs (use large limit like Python's 10000)
	result, err := s.SearchMetadataByKBs(kbIDs, 10000)
	if err != nil {
		return nil, err
	}

	flattedMeta := make(common.MetaData)

	for _, chunk := range result.MetadataRecords {
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
				flattedMeta[fieldName] = make(common.MetaValueDocs)
			}

			valueMap := flattedMeta[fieldName]

			// Handle string, number (float64/int), and list of string/number
			switch v := fieldValue.(type) {
			case string:
				// Single string value (including time strings)
				if v != "" {
					if _, exists := valueMap[v]; !exists {
						valueMap[v] = []string{docID}
					} else {
						valueMap[v] = appendDocID(valueMap[v], docID)
					}
				}
			case float64:
				// Numeric value - convert to string (matching Python's str())
				strVal := strconv.FormatFloat(v, 'f', -1, 64)
				if _, exists := valueMap[strVal]; !exists {
					valueMap[strVal] = []string{docID}
				} else {
					valueMap[strVal] = appendDocID(valueMap[strVal], docID)
				}
			case int:
				// Integer value - convert to string
				strVal := fmt.Sprintf("%d", v)
				if _, exists := valueMap[strVal]; !exists {
					valueMap[strVal] = []string{docID}
				} else {
					valueMap[strVal] = appendDocID(valueMap[strVal], docID)
				}
			case []interface{}:
				// List of values (string, number, or time)
				for _, item := range v {
					switch itemVal := item.(type) {
					case string:
						if itemVal != "" {
							if _, exists := valueMap[itemVal]; !exists {
								valueMap[itemVal] = []string{docID}
							} else {
								valueMap[itemVal] = appendDocID(valueMap[itemVal], docID)
							}
						}
					case float64:
						strVal := strconv.FormatFloat(itemVal, 'f', -1, 64)
						if _, exists := valueMap[strVal]; !exists {
							valueMap[strVal] = []string{docID}
						} else {
							valueMap[strVal] = appendDocID(valueMap[strVal], docID)
						}
					case int:
						strVal := fmt.Sprintf("%d", itemVal)
						if _, exists := valueMap[strVal]; !exists {
							valueMap[strVal] = []string{docID}
						} else {
							valueMap[strVal] = appendDocID(valueMap[strVal], docID)
						}
					}
				}
			}
		}
	}

	return flattedMeta, nil
}

// CollectDocIDsByKB collects unique (kb_id, doc_id) pairs from chunks.
func CollectDocIDsByKB(chunks []map[string]interface{}) KBDocIDsMap {
	seen := make(map[string]struct{})
	result := make(KBDocIDsMap)
	for _, chunk := range chunks {
		kbID := extractKBID(chunk)
		docID := extractDocID(chunk)
		if kbID == "" || docID == "" {
			continue
		}
		key := kbID + ":" + docID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result[kbID] = append(result[kbID], docID)
	}
	return result
}

// ConvertSearchResultToDocMeta converts SearchMetadataResult chunks into a DocMetaMap.
// Pure function, no dependencies.
func ConvertSearchResultToDocMeta(chunks []map[string]interface{}) DocMetaMap {
	metaByDoc := make(DocMetaMap)
	for _, metaChunk := range chunks {
		docID := extractDocID(metaChunk)
		if docID == "" {
			continue
		}
		metaFields, err := ExtractMetaFields(metaChunk)
		if err != nil || len(metaFields) == 0 {
			continue
		}
		metaByDoc[docID] = metaFields
	}
	return metaByDoc
}

// FetchDocMetaByKB fetches document metadata from ES for each KB.
func (s *MetadataService) FetchDocMetaByKB(docIDsByKB KBDocIDsMap, tenantID string) DocMetaMap {
	metaByDoc := make(DocMetaMap)
	for kbID, docIDs := range docIDsByKB {
		result, err := s.SearchMetadata(kbID, tenantID, docIDs, len(docIDs))
		if err != nil {
			continue
		}
		for docID, meta := range ConvertSearchResultToDocMeta(result.MetadataRecords) {
			metaByDoc[docID] = meta
		}
	}
	return metaByDoc
}

// AttachDocMetaToChunks attaches document metadata to matching chunks in-place.
func AttachDocMetaToChunks(chunks []map[string]interface{}, metaByDoc DocMetaMap, metadataFields []string) {
	filter := make(map[string]struct{}, len(metadataFields))
	for _, f := range metadataFields {
		filter[f] = struct{}{}
	}
	for _, chunk := range chunks {
		docID := extractDocID(chunk)
		if docID == "" {
			continue
		}
		meta, ok := metaByDoc[docID]
		if !ok {
			continue
		}
		if len(filter) > 0 {
			filtered := make(map[string]interface{}, len(filter))
			for k, v := range meta {
				if _, ok := filter[k]; ok {
					filtered[k] = v
				}
			}
			if len(filtered) > 0 {
				chunk["document_metadata"] = filtered
			}
		} else {
			chunk["document_metadata"] = meta
		}
	}
}

// EnrichChunksWithDocMetadata attaches document metadata to each chunk in-place.
// Combines CollectDocIDsByKB, FetchDocMetaByKB, and AttachDocMetaToChunks.
func (s *MetadataService) EnrichChunksWithDocMetadata(chunks []map[string]interface{}, tenantID string, metadataFields []string) {
	if len(chunks) == 0 || s.docEngine == nil {
		return
	}
	docIDsByKB := CollectDocIDsByKB(chunks)
	if len(docIDsByKB) == 0 {
		return
	}
	metaByDoc := s.FetchDocMetaByKB(docIDsByKB, tenantID)
	if len(metaByDoc) == 0 {
		return
	}
	AttachDocMetaToChunks(chunks, metaByDoc, metadataFields)
}

// extractKBID extracts the KB ID from a chunk, checking common field names.
func extractKBID(chunk map[string]interface{}) string {
	if id, ok := chunk["kb_id"].(string); ok && id != "" {
		return id
	}
	if id, ok := chunk["dataset_id"].(string); ok && id != "" {
		return id
	}
	return ""
}

// extractDocID extracts the document ID from a chunk, checking both id and doc_id.
func extractDocID(chunk map[string]interface{}) string {
	if id, ok := chunk["id"].(string); ok {
		return id
	}
	if id, ok := chunk["doc_id"].(string); ok {
		return id
	}
	return ""
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
