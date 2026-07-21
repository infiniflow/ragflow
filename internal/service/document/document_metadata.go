package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/service"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

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
	return aggregateMetadata(searchResult.MetadataRecords), nil
}

// SetDocumentMetadata sets metadata for a document in the document engine
func (s *DocumentService) SetDocumentMetadata(docID string, meta map[string]interface{}) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	if err := s.docEngine.UpdateMetadata(context.Background(), docID, doc.KbID, meta, tenantID); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// DeleteDocumentMetadata deletes metadata keys for a document in the document engine
func (s *DocumentService) DeleteDocumentMetadata(docID string, keys []string) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	// Delete metadata using the document engine
	err = s.docEngine.DeleteMetadataKeys(nil, docID, doc.KbID, keys, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	return nil
}

// DeleteDocumentAllMetadata deletes all metadata for a document in the document engine
func (s *DocumentService) DeleteDocumentAllMetadata(docID string) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	// Build condition to match the document
	condition := map[string]interface{}{
		"id":    docID,
		"kb_id": doc.KbID,
	}

	// Delete entire document metadata
	_, err = s.docEngine.DeleteMetadata(nil, condition, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete document metadata: %w", err)
	}

	return nil
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
	if len(searchResult.MetadataRecords) > 0 {
		metadata := searchResult.MetadataRecords[0]
		return service.ExtractMetaFields(metadata)
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
	numMetadata := len(searchResult.MetadataRecords)

	var allMetaFields []map[string]interface{}
	if numMetadata > 1 && len(searchResult.MetadataRecords) > 0 {
		firstMetadata := searchResult.MetadataRecords[0]
		if metaFieldsVal := firstMetadata["meta_fields"]; metaFieldsVal != nil {
			if v, ok := metaFieldsVal.([]byte); ok {
				allMetaFields = service.ParseAllLengthPrefixedJSON(v)
			}
		}
	}

	for idx, metadata := range searchResult.MetadataRecords {
		docID, ok := service.ExtractDocumentID(metadata)
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
			metaFieldsVal = metadata["meta_fields"]
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
						metaFields = service.ParseLengthPrefixedJSON(v)
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

			// Aggregate value counts. Flatten nested arrays so malformed values do
			// not surface in the UI as the literal string "[]".
			values := flattenMetadataSummaryValues(v)
			for _, vv := range values {
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
			"type":   valueType,
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

func flattenMetadataSummaryValues(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, flattenMetadataSummaryValues(item)...)
		}
		return result
	case []string:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	case nil:
		return nil
	default:
		return []interface{}{typed}
	}
}

// isTimeString checks if a string is an ISO 8601 datetime
func isTimeString(s string) bool {
	matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`, s)
	return matched
}

func (s *DocumentService) replaceDocumentMetadata(docID string, meta map[string]any) error {
	if s.docEngine == nil || s.metadataSvc == nil {
		return nil
	}
	if err := s.DeleteDocumentAllMetadata(docID); err != nil {
		return err
	}
	return s.SetDocumentMetadata(docID, map[string]interface{}(meta))
}

func (s *DocumentService) patchDocumentMetadata(docID string, before, after map[string]interface{}) error {
	if s.docEngine == nil || s.metadataSvc == nil {
		return nil
	}

	deleteKeys := make([]string, 0)
	for key := range before {
		if _, ok := after[key]; !ok {
			deleteKeys = append(deleteKeys, key)
		}
	}
	if len(deleteKeys) > 0 {
		if err := s.DeleteDocumentMetadata(docID, deleteKeys); err != nil {
			return err
		}
	}

	updateFields := make(map[string]interface{})
	for key, value := range after {
		if !reflect.DeepEqual(before[key], value) {
			updateFields[key] = value
		}
	}
	if len(updateFields) == 0 {
		return nil
	}
	return s.SetDocumentMetadata(docID, updateFields)
}

// BatchUpdateDocumentMetadatas implements the shared logic for
// PATCH /datasets/:dataset_id/documents/metadatas  and
// POST  /datasets/:dataset_id/metadata/update.
func (s *DocumentService) BatchUpdateDocumentMetadatas(
	datasetID string,
	selector *DocumentMetadataSelector,
	updates []DocumentMetadataUpdate,
	deletes []DocumentMetadataDelete,
) (*BatchUpdateDocumentMetadatasResponse, common.ErrorCode, error) {
	if selector == nil {
		selector = &DocumentMetadataSelector{}
	}
	if code, err := validateBatchUpdateDocumentMetadatasRequest(selector, updates, deletes); err != nil {
		return nil, code, err
	}

	// Resolve which document IDs to target.
	targetDocIDs := make(map[string]struct{})

	if len(selector.DocumentIDs) > 0 {
		// Validate that supplied IDs actually belong to this dataset.
		allRows, err := s.documentDAO.GetAllDocIDsByKBIDs([]string{datasetID})
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to list dataset documents: %w", err)
		}
		kbDocIDSet := make(map[string]struct{}, len(allRows))
		for _, row := range allRows {
			kbDocIDSet[row["id"]] = struct{}{}
		}
		var invalidIDs []string
		for _, id := range selector.DocumentIDs {
			if _, ok := kbDocIDSet[id]; !ok {
				invalidIDs = append(invalidIDs, id)
			}
		}
		if len(invalidIDs) > 0 {
			return nil, common.CodeDataError, fmt.Errorf("these documents do not belong to dataset %s: %s",
				datasetID, strings.Join(invalidIDs, ", "))
		}
		for _, id := range selector.DocumentIDs {
			targetDocIDs[id] = struct{}{}
		}
	}

	// Apply metadata_condition filter.
	if len(selector.MetadataCondition) > 0 {
		flattedMeta, err := s.metadataSvc.GetFlattedMetaByKBs([]string{datasetID})
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to get flattened metadata: %w", err)
		}

		// ParseAndConvert mirrors Python convert_conditions: conditions arrive as
		// {name, comparison_operator, value}, the operator is normalised, and the
		// (possibly non-string) value is preserved. MetaFilter then matches against
		// the common.MetaData returned by GetFlattedMetaByKBs.
		filterInput := common.ParseAndConvert(selector.MetadataCondition)
		filteredIDs := common.MetaFilter(flattedMeta, filterInput)

		filteredSet := make(map[string]struct{}, len(filteredIDs))
		for _, id := range filteredIDs {
			filteredSet[id] = struct{}{}
		}

		if len(targetDocIDs) > 0 {
			// Intersect with the document_ids restriction.
			for id := range targetDocIDs {
				if _, ok := filteredSet[id]; !ok {
					delete(targetDocIDs, id)
				}
			}
		} else {
			targetDocIDs = filteredSet
		}

		// Early-exit when conditions given but nothing matched.
		rawConds, _ := selector.MetadataCondition["conditions"]
		if rawConds != nil && len(targetDocIDs) == 0 {
			return &BatchUpdateDocumentMetadatasResponse{Updated: 0, MatchedDocs: 0}, common.CodeSuccess, nil
		}
	}

	ids := make([]string, 0, len(targetDocIDs))
	for id := range targetDocIDs {
		ids = append(ids, id)
	}

	// Apply updates and deletes per document using Python's batch_update_metadata
	// semantics instead of a simple merge-then-delete.
	updated := 0
	for _, docID := range ids {
		currentMeta, err := s.GetDocumentMetadataByID(docID)
		if err != nil {
			common.Warn("BatchUpdateDocumentMetadata: get metadata failed",
				zap.String("docID", docID), zap.Error(err))
			continue
		}

		meta := cloneDocumentMetadata(currentMeta)
		originalMeta := cloneDocumentMetadata(meta)

		changed := applyDocumentMetadataUpdates(meta, updates)
		if applyDocumentMetadataDeletes(meta, deletes) {
			changed = true
		}

		if !changed || reflect.DeepEqual(originalMeta, meta) {
			continue
		}

		if err := s.patchDocumentMetadata(docID, originalMeta, meta); err != nil {
			common.Warn("BatchUpdateDocumentMetadata: patch metadata failed",
				zap.String("docID", docID), zap.Error(err))
			continue
		}
		updated++
	}

	return &BatchUpdateDocumentMetadatasResponse{Updated: updated, MatchedDocs: len(ids)}, common.CodeSuccess, nil
}

func validateBatchUpdateDocumentMetadatasRequest(
	selector *DocumentMetadataSelector,
	updates []DocumentMetadataUpdate,
	deletes []DocumentMetadataDelete,
) (common.ErrorCode, error) {
	for _, upd := range updates {
		if strings.TrimSpace(upd.Key) == "" || upd.Value == nil {
			return common.CodeDataError, errors.New("Each update requires key and value.")
		}
	}
	for _, del := range deletes {
		if strings.TrimSpace(del.Key) == "" {
			return common.CodeDataError, errors.New("Each delete requires key.")
		}
	}
	if selector != nil && selector.MetadataCondition != nil {
		if _, ok := selector.MetadataCondition["conditions"]; !ok && len(selector.MetadataCondition) > 0 {
			return common.CodeDataError, errors.New("metadata_condition must be an object.")
		}
	}
	return common.CodeSuccess, nil
}

func cloneDocumentMetadata(meta map[string]interface{}) map[string]interface{} {
	if meta == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(meta))
	for k, v := range meta {
		cloned[k] = cloneDocumentMetadataValue(v)
	}
	return cloned
}

func cloneDocumentMetadataValue(v interface{}) interface{} {
	switch typed := v.(type) {
	case []interface{}:
		cp := make([]interface{}, len(typed))
		copy(cp, typed)
		return cp
	case []string:
		cp := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			cp = append(cp, item)
		}
		return cp
	default:
		return typed
	}
}

func applyDocumentMetadataUpdates(meta map[string]interface{}, updates []DocumentMetadataUpdate) bool {
	changed := false
	for _, upd := range updates {
		key := strings.TrimSpace(upd.Key)
		if key == "" {
			continue
		}
		normalizedValue := normalizeDocumentMetadataUpdateValue(upd.Value, upd.ValueType)
		matchProvided := upd.Match != nil && !(fmt.Sprintf("%v", upd.Match) == "")
		current, exists := meta[key]
		if !exists {
			if matchProvided {
				continue
			}
			if listVal, ok := toMetadataInterfaceSlice(normalizedValue); ok {
				meta[key] = dedupeDocumentMetadataList(listVal)
			} else {
				meta[key] = normalizedValue
			}
			changed = true
			continue
		}

		if curList, ok := toMetadataInterfaceSlice(current); ok {
			if !matchProvided {
				newList := append([]interface{}{}, curList...)
				if appendList, ok := toMetadataInterfaceSlice(normalizedValue); ok {
					newList = append(newList, appendList...)
				} else {
					newList = append(newList, normalizedValue)
				}
				newList = dedupeDocumentMetadataList(newList)
				if !reflect.DeepEqual(curList, newList) {
					meta[key] = newList
					changed = true
				}
				continue
			}

			replaced := false
			newList := make([]interface{}, 0, len(curList))
			for _, item := range curList {
				if documentMetadataValuesEqual(item, upd.Match) {
					if replacementList, ok := toMetadataInterfaceSlice(normalizedValue); ok {
						newList = append(newList, replacementList...)
					} else {
						newList = append(newList, normalizedValue)
					}
					replaced = true
				} else {
					newList = append(newList, item)
				}
			}
			newList = dedupeDocumentMetadataList(newList)
			if replaced && !reflect.DeepEqual(curList, newList) {
				meta[key] = newList
				changed = true
			}
			continue
		}

		if !matchProvided {
			if !reflect.DeepEqual(current, normalizedValue) {
				meta[key] = normalizedValue
				changed = true
			}
			continue
		}
		if documentMetadataValuesEqual(current, upd.Match) && !reflect.DeepEqual(current, normalizedValue) {
			meta[key] = normalizedValue
			changed = true
		}
	}
	return changed
}

func applyDocumentMetadataDeletes(meta map[string]interface{}, deletes []DocumentMetadataDelete) bool {
	changed := false
	for _, del := range deletes {
		key := strings.TrimSpace(del.Key)
		current, exists := meta[key]
		if key == "" || !exists {
			continue
		}

		if curList, ok := toMetadataInterfaceSlice(current); ok {
			if del.Value == nil {
				delete(meta, key)
				changed = true
				continue
			}
			newList := make([]interface{}, 0, len(curList))
			for _, item := range curList {
				if !documentMetadataValuesEqual(item, del.Value) {
					newList = append(newList, item)
				}
			}
			if len(newList) != len(curList) {
				if len(newList) == 0 {
					delete(meta, key)
				} else {
					meta[key] = newList
				}
				changed = true
			}
			continue
		}

		if del.Value == nil || documentMetadataValuesEqual(current, del.Value) {
			delete(meta, key)
			changed = true
		}
	}
	return changed
}

func toMetadataInterfaceSlice(v interface{}) ([]interface{}, bool) {
	switch typed := v.(type) {
	case []interface{}:
		cp := make([]interface{}, len(typed))
		copy(cp, typed)
		return cp, true
	case []string:
		cp := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			cp = append(cp, item)
		}
		return cp, true
	default:
		return nil, false
	}
}

func dedupeDocumentMetadataList(items []interface{}) []interface{} {
	result := make([]interface{}, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := fmt.Sprintf("%T:%v", item, item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func documentMetadataValuesEqual(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func normalizeDocumentMetadataUpdateValue(value interface{}, valueType string) interface{} {
	switch strings.ToLower(strings.TrimSpace(valueType)) {
	case "list":
		if list, ok := normalizeMetadataListValue(value); ok {
			return list
		}
		return []interface{}{}
	case "number":
		scalar, ok := firstScalarMetadataValue(value)
		if !ok {
			return value
		}
		switch typed := scalar.(type) {
		case float64, float32, int, int8, int16, int32, int64:
			return typed
		case json.Number:
			if i, err := typed.Int64(); err == nil {
				return i
			}
			if f, err := typed.Float64(); err == nil {
				return f
			}
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				return ""
			}
			if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				return i
			}
			if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
				return f
			}
			return trimmed
		}
		return scalar
	case "string", "time":
		if scalar, ok := firstScalarMetadataValue(value); ok {
			return fmt.Sprintf("%v", scalar)
		}
		return ""
	default:
		return value
	}
}

func normalizeMetadataListValue(value interface{}) ([]interface{}, bool) {
	switch typed := value.(type) {
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if nested, ok := normalizeMetadataListValue(item); ok {
				result = append(result, nested...)
				continue
			}
			if item != nil {
				result = append(result, item)
			}
		}
		return result, true
	case []string:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result, true
	default:
		return nil, false
	}
}

func firstScalarMetadataValue(value interface{}) (interface{}, bool) {
	if list, ok := normalizeMetadataListValue(value); ok {
		for _, item := range list {
			if item != nil {
				return item, true
			}
		}
		return nil, false
	}
	if value == nil {
		return nil, false
	}
	return value, true
}
