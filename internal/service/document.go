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
	"regexp"
	"sort"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/model"
	"ragflow/internal/server"
)

// DocumentService document service
type DocumentService struct {
	documentDAO *dao.DocumentDAO
	kbDAO       *dao.KnowledgebaseDAO
	docEngine   engine.DocEngine
	engineType  server.EngineType
	metadataSvc *MetadataService
}

// NewDocumentService create document service
func NewDocumentService() *DocumentService {
	cfg := server.GetConfig()
	return &DocumentService{
		documentDAO: dao.NewDocumentDAO(),
		kbDAO:       dao.NewKnowledgebaseDAO(),
		docEngine:   engine.Get(),
		engineType:  cfg.DocEngine.Type,
		metadataSvc: NewMetadataService(),
	}
}

// CreateDocumentRequest create document request
type CreateDocumentRequest struct {
	Name      string `json:"name" binding:"required"`
	KbID      string `json:"kb_id" binding:"required"`
	ParserID  string `json:"parser_id" binding:"required"`
	CreatedBy string `json:"created_by" binding:"required"`
	Type      string `json:"type"`
	Source    string `json:"source"`
}

// UpdateDocumentRequest update document request
type UpdateDocumentRequest struct {
	Name        *string  `json:"name"`
	Run         *string  `json:"run"`
	TokenNum    *int64   `json:"token_num"`
	ChunkNum    *int64   `json:"chunk_num"`
	Progress    *float64 `json:"progress"`
	ProgressMsg *string  `json:"progress_msg"`
}

// DocumentResponse document response
type DocumentResponse struct {
	ID              string  `json:"id"`
	Name            *string `json:"name,omitempty"`
	KbID            string  `json:"kb_id"`
	ParserID        string  `json:"parser_id"`
	PipelineID      *string `json:"pipeline_id,omitempty"`
	Type            string  `json:"type"`
	SourceType      string  `json:"source_type"`
	CreatedBy       string  `json:"created_by"`
	Location        *string `json:"location,omitempty"`
	Size            int64   `json:"size"`
	TokenNum        int64   `json:"token_num"`
	ChunkNum        int64   `json:"chunk_num"`
	Progress        float64 `json:"progress"`
	ProgressMsg     *string `json:"progress_msg,omitempty"`
	ProcessDuration float64 `json:"process_duration"`
	Suffix          string  `json:"suffix"`
	Run             *string `json:"run,omitempty"`
	Status          *string `json:"status,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// CreateDocument create document
func (s *DocumentService) CreateDocument(req *CreateDocumentRequest) (*model.Document, error) {
	document := &model.Document{
		Name:       &req.Name,
		KbID:       req.KbID,
		ParserID:   req.ParserID,
		CreatedBy:  req.CreatedBy,
		Type:       req.Type,
		SourceType: req.Source,
		Suffix:     ".doc",
		Status:     func() *string { s := "0"; return &s }(),
	}

	if err := s.documentDAO.Create(document); err != nil {
		return nil, fmt.Errorf("failed to create document: %w", err)
	}

	return document, nil
}

// GetDocumentByID get document by ID
func (s *DocumentService) GetDocumentByID(id string) (*DocumentResponse, error) {
	document, err := s.documentDAO.GetByID(id)
	if err != nil {
		return nil, err
	}

	return s.toResponse(document), nil
}

// UpdateDocument update document
func (s *DocumentService) UpdateDocument(id string, req *UpdateDocumentRequest) error {
	document, err := s.documentDAO.GetByID(id)
	if err != nil {
		return err
	}

	if req.Name != nil {
		document.Name = req.Name
	}
	if req.Run != nil {
		document.Run = req.Run
	}
	if req.TokenNum != nil {
		document.TokenNum = *req.TokenNum
	}
	if req.ChunkNum != nil {
		document.ChunkNum = *req.ChunkNum
	}
	if req.Progress != nil {
		document.Progress = *req.Progress
	}
	if req.ProgressMsg != nil {
		document.ProgressMsg = req.ProgressMsg
	}

	return s.documentDAO.Update(document)
}

// DeleteDocument delete document
func (s *DocumentService) DeleteDocument(id string) error {
	return s.documentDAO.Delete(id)
}

// ListDocuments list documents
func (s *DocumentService) ListDocuments(page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.List(offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

// ListDocumentsByKBID list documents by knowledge base ID
func (s *DocumentService) ListDocumentsByKBID(kbID string, page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.ListByKBID(kbID, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

// GetDocumentsByAuthorID get documents by author ID
func (s *DocumentService) GetDocumentsByAuthorID(authorID, page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.GetByAuthorID(fmt.Sprintf("%d", authorID), offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

// toResponse convert model.Document to DocumentResponse
func (s *DocumentService) toResponse(doc *model.Document) *DocumentResponse {
	createdAt := ""
	if doc.CreateTime != nil {
		// Check if timestamp is in milliseconds (13 digits) or seconds (10 digits)
		var ts int64
		if *doc.CreateTime > 1000000000000 {
			// Milliseconds - convert to seconds
			ts = *doc.CreateTime / 1000
		} else {
			ts = *doc.CreateTime
		}
		createdAt = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	}
	updatedAt := ""
	if doc.UpdateTime != nil {
		updatedAt = time.Unix(*doc.UpdateTime, 0).Format("2006-01-02 15:04:05")
	}
	return &DocumentResponse{
		ID:              doc.ID,
		Name:            doc.Name,
		KbID:            doc.KbID,
		ParserID:        doc.ParserID,
		PipelineID:      doc.PipelineID,
		Type:            doc.Type,
		SourceType:      doc.SourceType,
		CreatedBy:       doc.CreatedBy,
		Location:        doc.Location,
		Size:            doc.Size,
		TokenNum:        doc.TokenNum,
		ChunkNum:        doc.ChunkNum,
		Progress:        doc.Progress,
		ProgressMsg:     doc.ProgressMsg,
		ProcessDuration: doc.ProcessDuration,
		Suffix:          doc.Suffix,
		Run:             doc.Run,
		Status:          doc.Status,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}

// GetMetadataSummaryRequest request for metadata summary
type GetMetadataSummaryRequest struct {
	KBID   string   `json:"kb_id" binding:"required"`
	DocIDs []string `json:"doc_ids"`
}

// GetMetadataSummaryResponse response for metadata summary
type GetMetadataSummaryResponse struct {
	Summary map[string]interface{} `json:"summary"`
}

// GetMetadataSummary get metadata summary for documents
func (s *DocumentService) GetMetadataSummary(kbID string, docIDs []string) (map[string]interface{}, error) {
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(kbID)
	if err != nil {
		return nil, err
	}

	searchResult, err := s.metadataSvc.SearchMetadata(kbID, tenantID, docIDs, 1000)
	if err != nil {
		return nil, err
	}

	// Aggregate metadata from results
	return aggregateMetadata(searchResult.Chunks), nil
}

// GetDocumentMetadataByID get metadata for a specific document
func (s *DocumentService) GetDocumentMetadataByID(docID string) (map[string]interface{}, error) {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	tenantID, err := s.metadataSvc.GetTenantIDByKBID(doc.KbID)
	if err != nil {
		return nil, err
	}

	searchResult, err := s.metadataSvc.SearchMetadata(doc.KbID, tenantID, []string{docID}, 1)
	if err != nil {
		return nil, err
	}

	// Return metadata if found
	if len(searchResult.Chunks) > 0 {
		chunk := searchResult.Chunks[0]
		return ExtractMetaFields(chunk)
	}

	return make(map[string]interface{}), nil
}

// GetMetadataByKBs get metadata for knowledge bases
func (s *DocumentService) GetMetadataByKBs(kbIDs []string) (map[string]interface{}, error) {
	if len(kbIDs) == 0 {
		return make(map[string]interface{}), nil
	}

	searchResult, err := s.metadataSvc.SearchMetadataByKBs(kbIDs, 10000)
	if err != nil {
		return nil, err
	}

	flattenedMeta := make(map[string]map[string][]string)
	numChunks := len(searchResult.Chunks)

	var allMetaFields []map[string]interface{}
	if numChunks > 1 && len(searchResult.Chunks) > 0 {
		firstChunk := searchResult.Chunks[0]
		if metaFieldsVal := firstChunk["meta_fields"]; metaFieldsVal != nil {
			if v, ok := metaFieldsVal.([]byte); ok {
				allMetaFields = ParseAllLengthPrefixedJSON(v)
			}
		}
	}

	for idx, chunk := range searchResult.Chunks {
		docID, ok := ExtractDocumentID(chunk)
		if !ok {
			continue
		}

		var metaFields map[string]interface{}
		var metaFieldsVal interface{}

		if len(allMetaFields) > 0 && idx < len(allMetaFields) {
			// Use pre-parsed meta_fields from concatenated data
			metaFields = allMetaFields[idx]
		} else {
			// Normal case - get from chunk
			metaFieldsVal = chunk["meta_fields"]
			if metaFieldsVal != nil {
				switch v := metaFieldsVal.(type) {
				case string:
					if err := json.Unmarshal([]byte(v), &metaFields); err != nil {
						continue
					}
				case []byte:
					// Try direct JSON parse first
					if err := json.Unmarshal(v, &metaFields); err != nil {
						// Try to parse as concatenated JSON objects
						metaFields = ParseLengthPrefixedJSON(v)
					}
				case map[string]interface{}:
					metaFields = v
				default:
					continue
				}
			}
		}

		if metaFields == nil {
			continue
		}

		// Process each metadata field
		for fieldName, fieldValue := range metaFields {
			if fieldName == "kb_id" || fieldName == "id" {
				continue
			}

			if _, ok := flattenedMeta[fieldName]; !ok {
				flattenedMeta[fieldName] = make(map[string][]string)
			}

			// Handle list and single values
			var values []interface{}
			switch v := fieldValue.(type) {
			case []interface{}:
				values = v
			default:
				values = []interface{}{v}
			}

			for _, val := range values {
				if val == nil {
					continue
				}
				strVal := fmt.Sprintf("%v", val)
				flattenedMeta[fieldName][strVal] = append(flattenedMeta[fieldName][strVal], docID)
			}
		}
	}

	// Convert to map[string]interface{} for return
	var metaResult map[string]interface{} = make(map[string]interface{})
	for k, v := range flattenedMeta {
		metaResult[k] = v
	}

	return metaResult, nil
}

// valueInfo holds count and order of first appearance
type valueInfo struct {
	count     int
	firstOrder int
}

// aggregateMetadata aggregates metadata from search results
func aggregateMetadata(chunks []map[string]interface{}) map[string]interface{} {
	// summary: map[fieldName]map[value]valueInfo
	summary := make(map[string]map[string]valueInfo)
	typeCounter := make(map[string]map[string]int)
	orderCounter := 0

	for _, chunk := range chunks {
		// For metadata table, the actual metadata is in the "meta_fields" JSON field
		// Extract it first
		metaFieldsVal := chunk["meta_fields"]
		if metaFieldsVal == nil {
			continue
		}

		// Parse meta_fields - could be a string (JSON) or a map
		var metaFields map[string]interface{}
		switch v := metaFieldsVal.(type) {
		case string:
			// Parse JSON string
			if err := json.Unmarshal([]byte(v), &metaFields); err != nil {
				continue
			}
		case []byte:
			// Handle byte slice - Infinity returns concatenated JSON objects with length prefixes
			rawBytes := v

			// Try to detect and handle length-prefixed format
			// Format: [4-byte length][JSON][4-byte length][JSON]...
			parsedMetaFields := make(map[string]interface{})
			offset := 0
			for offset < len(rawBytes) {
				// Need at least 4 bytes for length prefix
				if offset+4 > len(rawBytes) {
					break
				}

				// Read 4-byte length (little-endian, not big-endian!)
				length := uint32(rawBytes[offset]) | uint32(rawBytes[offset+1])<<8 |
					uint32(rawBytes[offset+2])<<16 | uint32(rawBytes[offset+3])<<24

				// Check if length looks valid (not too large)
				if length > 10000 || length == 0 {
					// Try to find next '{' from current position
					nextBrace := -1
					for i := offset; i < len(rawBytes) && i < offset+100; i++ {
						if rawBytes[i] == '{' {
							nextBrace = i
							break
						}
					}
					if nextBrace > offset {
						// Skip to the next '{'
						offset = nextBrace
						continue
					}
					break
				}

				// Extract JSON data
				jsonStart := offset + 4
				jsonEnd := jsonStart + int(length)
				if jsonEnd > len(rawBytes) {
					jsonEnd = len(rawBytes)
				}

				jsonBytes := rawBytes[jsonStart:jsonEnd]

				// Try to parse this JSON
				var singleMeta map[string]interface{}
				if err := json.Unmarshal(jsonBytes, &singleMeta); err == nil {
					// Merge metadata from this document
					for k, vv := range singleMeta {
						if existing, ok := parsedMetaFields[k]; ok {
							// Combine values
							if existList, ok := existing.([]interface{}); ok {
								if newList, ok := vv.([]interface{}); ok {
									parsedMetaFields[k] = append(existList, newList...)
								} else {
									parsedMetaFields[k] = append(existList, vv)
								}
							} else {
								parsedMetaFields[k] = []interface{}{existing, vv}
							}
						} else {
							parsedMetaFields[k] = vv
						}
					}
				}

				offset = jsonEnd
			}

			// If we successfully parsed multiple JSON objects, use the merged result
			if len(parsedMetaFields) > 0 {
				metaFields = parsedMetaFields
			} else {
				// Fallback: try the original parsing method
				startIdx := -1
				for i, b := range rawBytes {
					if b == '{' {
						startIdx = i
						break
					}
				}
				if startIdx > 0 {
					strVal := string(rawBytes[startIdx:])
					if err := json.Unmarshal([]byte(strVal), &metaFields); err != nil {
						metaFields = map[string]interface{}{"raw": strVal}
					}
				} else if err := json.Unmarshal(rawBytes, &metaFields); err != nil {
					metaFields = map[string]interface{}{"raw": string(rawBytes)}
				}
			}
		case map[string]interface{}:
			metaFields = v
		default:
			continue
		}

		// Now iterate over the extracted metadata fields
		for k, v := range metaFields {
			// Skip nil values
			if v == nil {
				continue
			}

			// Determine value type
			valueType := getMetaValueType(v)

			// Track type counts
			if valueType != "" {
				if _, ok := typeCounter[k]; !ok {
					typeCounter[k] = make(map[string]int)
				}
				typeCounter[k][valueType] = typeCounter[k][valueType] + 1
			}

			// Aggregate value counts
			values := v
			if v, ok := v.([]interface{}); ok {
				values = v
			} else {
				values = []interface{}{v}
			}

			for _, vv := range values.([]interface{}) {
				if vv == nil {
					continue
				}
				sv := fmt.Sprintf("%v", vv)

				if _, ok := summary[k]; !ok {
					summary[k] = make(map[string]valueInfo)
				}

				if existing, ok := summary[k][sv]; ok {
					// Already exists, just increment count
					existing.count++
					summary[k][sv] = existing
				} else {
					// First time seeing this value - record order
					summary[k][sv] = valueInfo{count: 1, firstOrder: orderCounter}
					orderCounter++
				}
			}
		}
	}

	// Build result with type information and sorted values
	result := make(map[string]interface{})
	for k, v := range summary {
		// Sort by count descending, then by firstOrder ascending (to match Python stable sort)
		// values: [value, count, firstOrder]
		values := make([][3]interface{}, 0, len(v))
		for val, info := range v {
			values = append(values, [3]interface{}{val, info.count, info.firstOrder})
		}
		// Use stable sort - sort by count descending, then by firstOrder
		sort.SliceStable(values, func(i, j int) bool {
			cntI := values[i][1].(int)
			cntJ := values[j][1].(int)
			if cntI != cntJ {
				return cntI > cntJ // count descending
			}
			// If counts equal, use firstOrder ascending (earlier appearance first)
			return values[i][2].(int) < values[j][2].(int)
		})

		// Determine dominant type
		valueType := "string"
		if typeCounts, ok := typeCounter[k]; ok {
			maxCount := 0
			for t, c := range typeCounts {
				if c > maxCount {
					maxCount = c
					valueType = t
				}
			}
		}

		// Convert from [value, count, firstOrder] to [value, count] for output
		outputValues := make([][2]interface{}, len(values))
		for i, val := range values {
			outputValues[i] = [2]interface{}{val[0], val[1]}
		}

		result[k] = map[string]interface{}{
			"type":  valueType,
			"values": outputValues,
		}
	}

	return result
}

// getMetaValueType determines the type of a metadata value
func getMetaValueType(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case []interface{}:
		if len(v) > 0 {
			return "list"
		}
		return ""
	case bool:
		return "string"
	case int, int8, int16, int32, int64:
		return "number"
	case float32, float64:
		return "number"
	case string:
		if isTimeString(v) {
			return "time"
		}
		return "string"
	}
	return "string"
}

// isTimeString checks if a string is an ISO 8601 datetime
func isTimeString(s string) bool {
	matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`, s)
	return matched
}
