package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"ragflow/internal/service/datasetversion"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

// GetMetadataSummary get metadata summary for documents
func (s *DocumentService) GetMetadataSummary(ctx context.Context, kbID string, docIDs []string) (map[string]interface{}, error) {
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(ctx, kbID)
	if err != nil {
		return nil, err
	}

	searchResult, err := s.metadataSvc.SearchMetadata(ctx, kbID, tenantID, docIDs, 1000)
	if err != nil {
		return nil, err
	}

	// Aggregate metadata from results
	return aggregateMetadata(searchResult.MetadataRecords), nil
}

// SetDocumentMetadata sets metadata for a document in the document engine
func (s *DocumentService) SetDocumentMetadata(ctx context.Context, docID string, meta map[string]interface{}) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(ctx, doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	// Ensure the metadata store exists before writing (service-layer
	// create-on-first-write logic; the engine layer assumes it exists).
	if err := s.metadataSvc.EnsureMetadataStore(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to ensure metadata store: %w", err)
	}

	meta = splitCombinedDocumentMetadataValues(meta)
	if err = s.docEngine.UpdateMetadata(ctx, docID, doc.KbID, meta, tenantID); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}
	// The store keeps only the current values, so the history is the only place a
	// restore can read them back from. A history that cannot be recorded fails the
	// call: a row naming values nobody stored would replay an empty map.
	if err := recordDocumentMetadataState(ctx, dao.DB, docID, doc.KbID, meta); err != nil {
		return fmt.Errorf("failed to record the metadata history: %w", err)
	}

	return nil
}

// DeleteDocumentMetadata deletes metadata keys for a document in the document engine
func (s *DocumentService) DeleteDocumentMetadata(ctx context.Context, docID string, keys []string) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(ctx, doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	// Delete metadata using the document engine
	err = s.docEngine.DeleteMetadataKeys(ctx, docID, doc.KbID, keys, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}
	// The delete left a smaller map behind; that map is the state a restore to a
	// later version has to put back, so it is recorded as the document's newest
	// one. A read failure keeps the delete successful and leaves the history
	// without the row rather than guessing at what remained.
	remaining, readErr := s.GetDocumentMetadataByID(ctx, docID)
	if readErr != nil {
		common.Warn("Document: failed to read the metadata left after a delete",
			zap.String("document_id", docID), zap.Error(readErr))
		return nil
	}
	if err := recordDocumentMetadataState(ctx, dao.DB, docID, doc.KbID, remaining); err != nil {
		return fmt.Errorf("failed to record the metadata history: %w", err)
	}

	return nil
}

// DeleteDocumentAllMetadata deletes all metadata for a document in the document engine
func (s *DocumentService) DeleteDocumentAllMetadata(ctx context.Context, docID string) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(ctx, doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	// Build condition to match the document
	condition := map[string]interface{}{
		"id":    docID,
		"kb_id": doc.KbID,
	}

	// Delete entire document metadata
	_, err = s.docEngine.DeleteMetadata(ctx, condition, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete document metadata: %w", err)
	}
	// Nothing is recorded: this is only called while the document itself is being
	// removed, and the delete version already says the document is gone. A state
	// row here would name a document that no longer exists - and, once its id is
	// reused, describe the next document with the previous one's values.
	return nil
}

// GetDocumentMetadataByID get metadata for a specific document
func (s *DocumentService) GetDocumentMetadataByID(ctx context.Context, docID string) (map[string]interface{}, error) {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	tenantID, err := s.metadataSvc.GetTenantIDByKBID(ctx, doc.KbID)
	if err != nil {
		return nil, err
	}

	searchResult, err := s.metadataSvc.SearchMetadata(ctx, doc.KbID, tenantID, []string{docID}, 1)
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

// GetDocumentMetadataByIDs returns metadata for a batch of documents in one
// dataset, sharing the tenant and index lookups across all of them.
func (s *DocumentService) GetDocumentMetadataByIDs(ctx context.Context, kbID string, docIDs []string) (map[string]map[string]interface{}, error) {
	out := make(map[string]map[string]interface{}, len(docIDs))
	if len(docIDs) == 0 {
		return out, nil
	}
	// Retry the whole tenant-lookup + index-search unit once: a transient
	// failure in either step would otherwise clear metadata for every doc in
	// the batch.
	search := func() (*service.SearchMetadataResponse, error) {
		tenantID, err := s.metadataSvc.GetTenantIDByKBID(ctx, kbID)
		if err != nil {
			return nil, err
		}
		return s.metadataSvc.SearchMetadata(ctx, kbID, tenantID, docIDs, len(docIDs))
	}
	result, err := search()
	if err != nil {
		result, err = search()
		if err != nil {
			return out, err
		}
	}
	seen := make(map[string]bool, len(result.MetadataRecords))
	for _, record := range result.MetadataRecords {
		docID, ok := service.ExtractDocumentID(record)
		if !ok || docID == "" || seen[docID] {
			continue
		}
		seen[docID] = true
		fields, err := service.ExtractMetaFields(record)
		if err != nil {
			fields = map[string]interface{}{}
		}
		out[docID] = fields
	}
	return out, nil
}

// GetMetadataByKBs get metadata for knowledge bases
func (s *DocumentService) GetMetadataByKBs(ctx context.Context, kbIDs []string) (map[string]interface{}, error) {
	if len(kbIDs) == 0 {
		return make(map[string]interface{}), nil
	}

	searchResult, err := s.metadataSvc.SearchMetadataByKBs(ctx, kbIDs, 10000)
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

// documentMetadataScope resolves the document and the tenant whose metadata
// store holds its values.
func (s *DocumentService) documentMetadataScope(ctx context.Context, docID string) (*entity.Document, string, error) {
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil {
		return nil, "", fmt.Errorf("document not found: %w", err)
	}
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(ctx, doc.KbID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get tenant ID: %w", err)
	}
	return doc, tenantID, nil
}

// clearDocumentMetadataStore drops every value a document holds.
func (s *DocumentService) clearDocumentMetadataStore(ctx context.Context, doc *entity.Document, tenantID string) error {
	if _, err := s.docEngine.DeleteMetadata(ctx,
		map[string]interface{}{"id": doc.ID, "kb_id": doc.KbID}, tenantID); err != nil {
		return fmt.Errorf("failed to delete document metadata: %w", err)
	}
	return nil
}

// writeDocumentMetadataStore writes the values a document is left with. An
// empty map writes nothing: the callers clear the row first when they mean to
// remove every key, so a write of nothing has nothing to do.
func (s *DocumentService) writeDocumentMetadataStore(ctx context.Context, doc *entity.Document, tenantID string, meta map[string]interface{}) error {
	if len(meta) == 0 {
		return nil
	}
	if err := s.docEngine.UpdateMetadata(ctx, doc.ID, doc.KbID,
		splitCombinedDocumentMetadataValues(meta), tenantID); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}
	return nil
}

// writeDocumentMetadataSnapshot replaces a document's stored metadata with the
// given values and reports the dataset the document belongs to.
//
// It is the store half of a metadata write: no history row is appended, so a
// caller whose write has several steps records the state once, when it is
// final. The values are replaced as a whole - a snapshot says what the document
// holds, so a key it does not mention is a key the document does not have. A
// service without a document engine answers with no dataset and writes nothing,
// which is what the unit tests of the callers run against.
func (s *DocumentService) writeDocumentMetadataSnapshot(ctx context.Context, docID string, meta map[string]interface{}) (string, error) {
	if s.docEngine == nil || s.metadataSvc == nil {
		return "", nil
	}
	doc, tenantID, err := s.documentMetadataScope(ctx, docID)
	if err != nil {
		return "", err
	}
	if err := s.clearDocumentMetadataStore(ctx, doc, tenantID); err != nil {
		return "", err
	}
	if len(meta) == 0 {
		return doc.KbID, nil
	}
	// The store is created on first write, which is why the clear (which only
	// removes rows) comes first: a document nobody wrote metadata for has no
	// store yet, and clearing before creating it is the order the engine expects.
	if err := s.metadataSvc.EnsureMetadataStore(ctx, tenantID); err != nil {
		return "", fmt.Errorf("failed to ensure metadata store: %w", err)
	}
	if err := s.writeDocumentMetadataStore(ctx, doc, tenantID, meta); err != nil {
		return "", err
	}
	return doc.KbID, nil
}

func (s *DocumentService) replaceDocumentMetadata(ctx context.Context, docID string, meta map[string]any) error {
	// The state itself is recorded by the caller under the version that records
	// the change (see UpdateDatasetDocument): one record, keyed by that number.
	_, err := s.writeDocumentMetadataSnapshot(ctx, docID, meta)
	return err
}

// patchDocumentMetadata writes the new values and reports the document's display
// name, which the caller puts in the dataset timeline's change set. The name
// comes from the read the write already needs, so reporting it costs no extra
// lookup; a document that is not found answers with no name rather than failing
// a write that the caller has already decided to make.
func (s *DocumentService) patchDocumentMetadata(ctx context.Context, docID string, before, after map[string]interface{}) (string, error) {
	if s.docEngine == nil || s.metadataSvc == nil {
		return "", nil
	}

	deleteKeys := make([]string, 0)
	for key := range before {
		if _, ok := after[key]; !ok {
			deleteKeys = append(deleteKeys, key)
		}
	}

	// Check if anything actually changed.
	changed := false
	for key, value := range after {
		if !reflect.DeepEqual(before[key], value) {
			changed = true
			break
		}
	}
	if !changed && len(deleteKeys) == 0 {
		return "", nil
	}

	doc, tenantID, err := s.documentMetadataScope(ctx, docID)
	if err != nil {
		return "", err
	}
	if len(after) == 0 && len(deleteKeys) > 0 {
		if err := s.docEngine.DeleteMetadataKeys(ctx, docID, doc.KbID, deleteKeys, tenantID); err != nil {
			return "", fmt.Errorf("failed to delete metadata: %w", err)
		}
	}
	if len(after) > 0 {
		if err := s.metadataSvc.EnsureMetadataStore(ctx, tenantID); err != nil {
			return "", fmt.Errorf("failed to ensure metadata store: %w", err)
		}
	}
	// Send the complete 'after' map — UpdateMetadata replaces the stored
	// meta_fields wholesale on every engine, so a write carrying only the
	// delta would drop untouched keys. The delete above stays a separate step
	// for the empty-'after' case: writeDocumentMetadataStore skips empty maps,
	// so removing the last keys is the delete's job.
	if err := s.writeDocumentMetadataStore(ctx, doc, tenantID, after); err != nil {
		return "", err
	}
	// The values are recorded by the caller under the version that records the
	// change (see BatchUpdateDocumentMetadatas): one record, keyed by its number.
	return documentNameOf(doc), nil
}

// BatchUpdateDocumentMetadatas implements the shared logic for
// PATCH /datasets/:dataset_id/documents/metadatas  and
// POST  /datasets/:dataset_id/metadata/update.
func (s *DocumentService) BatchUpdateDocumentMetadatas(
	ctx context.Context,
	datasetID string,
	selector *MetadataSelector,
	updates []MetadataUpdate,
	deletes []MetadataDelete,
) (*BatchUpdateMetadatasResponse, common.ErrorCode, error) {
	if selector == nil {
		selector = &MetadataSelector{}
	}
	if code, err := validateBatchUpdateDocumentMetadatasRequest(selector, updates, deletes); err != nil {
		return nil, code, err
	}

	// Resolve which document IDs to target.
	targetDocIDs := make(map[string]struct{})

	if len(selector.DocumentIDs) > 0 {
		// Validate that supplied IDs actually belong to this dataset.
		allRows, err := s.documentDAO.GetAllDocIDsByKBIDs(ctx, dao.DB, []string{datasetID})
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
		flattedMeta, err := s.metadataSvc.GetFlattedMetaByKBs(ctx, []string{datasetID})
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to get flattened metadata: %w", err)
		}

		// ParseAndConvert mirrors Python convert_conditions: conditions arrive as
		// {name, comparison_operator, value}, the operator is normalized, and the
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
			return &BatchUpdateMetadatasResponse{Updated: 0, MatchedDocs: 0}, common.CodeSuccess, nil
		}
	}

	ids := make([]string, 0, len(targetDocIDs))
	for id := range targetDocIDs {
		ids = append(ids, id)
	}

	// Apply updates and deletes per document using Python's batch_update_metadata
	// semantics instead of a simple merge-then-delete.
	updated := 0
	failed := 0
	// The documents whose values moved, gathered for the dataset timeline and
	// recorded once at the end: one batch update is one moment, so it appends one
	// version rather than a row per document it touched.
	metadataChanges := make([]entity.DatasetVersionChange, 0, len(ids))
	// The values each document came to hold, kept beside the change set: the
	// version row says who moved, and this is what a replay of that version puts
	// back when a later edit lands in the same millisecond.
	metaByDocument := make(map[string]map[string]interface{}, len(ids))
	for _, docID := range ids {
		currentMeta, err := s.GetDocumentMetadataByID(ctx, docID)
		if err != nil {
			common.Warn("BatchUpdateDocumentMetadata: get metadata failed",
				zap.String("docID", docID), zap.Error(err))
			failed++
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

		docName, patchErr := s.patchDocumentMetadata(ctx, docID, originalMeta, meta)
		if patchErr != nil {
			common.Warn("BatchUpdateDocumentMetadata: patch metadata failed",
				zap.String("docID", docID), zap.Error(patchErr))
			failed++
			continue
		}
		updated++
		metaByDocument[docID] = meta
		metadataChanges = append(metadataChanges, entity.DatasetVersionChange{
			DocID:       docID,
			Entity:      entity.DatasetVersionChangeEntityDocument,
			Action:      entity.DatasetVersionChangeActionUpdate,
			Name:        docName,
			MetaChanges: metadataFieldChanges(originalMeta, meta),
		})
	}

	// An edit of the values is a version of its own: the metadata store is not
	// rebuilt by a restore, so the timeline is what says whose metadata moved and
	// when - the same reason the single-document path records one. A batch that
	// moved nothing records nothing: a version marks a moment, and a save that
	// repeated the previous values is not one.
	if err := recordMetadataVersionChanges(ctx, datasetID, metadataChanges, metaByDocument); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to record the metadata history: %w", err)
	}
	if failed > 0 {
		return nil, common.CodeServerError, fmt.Errorf("failed to save metadata for %d of %d documents", failed, len(ids))
	}

	return &BatchUpdateMetadatasResponse{Updated: updated, MatchedDocs: len(ids)}, common.CodeSuccess, nil
}

// recordMetadataVersionChanges appends the dataset timeline row that names the
// documents a batch metadata update moved.
//
// It is its own function so the writer can be exercised without a metadata
// store: the row is the observable half of a change whose values live in a store
// the unit tests do not carry. An empty set is not a moment at all, but a state
// that cannot be stored is an error: a row naming values nobody holds would
// replay an empty map over the document.
func recordMetadataVersionChanges(ctx context.Context, datasetID string, changes []entity.DatasetVersionChange, metaByDocument map[string]map[string]interface{}) error {
	if len(changes) == 0 {
		return nil
	}
	version, err := datasetversion.RecordContentVersionChangesOwnTx(
		ctx, datasetID, entity.DatasetVersionKindDocumentMetadata, changes)
	if err != nil {
		return err
	}
	// The values a replay of this version puts back, recorded by number so a
	// later edit in the same millisecond cannot displace them.
	for _, change := range changes {
		if meta, ok := metaByDocument[change.DocID]; ok {
			if err := recordDocumentVersionMetadata(ctx, dao.DB, version, change.DocID, datasetID, meta); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBatchUpdateDocumentMetadatasRequest(
	selector *MetadataSelector,
	updates []MetadataUpdate,
	deletes []MetadataDelete,
) (common.ErrorCode, error) {
	for _, upd := range updates {
		if strings.TrimSpace(upd.Key) == "" || upd.Value == nil {
			return common.CodeDataError, errors.New("each update requires key and value")
		}
	}
	for _, del := range deletes {
		if strings.TrimSpace(del.Key) == "" {
			return common.CodeDataError, errors.New("each delete requires key")
		}
	}
	if selector != nil && selector.MetadataCondition != nil {
		if _, ok := selector.MetadataCondition["conditions"]; !ok && len(selector.MetadataCondition) > 0 {
			return common.CodeDataError, errors.New("metadata_condition must be an object")
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

func splitCombinedDocumentMetadataValues(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return meta
	}
	out := make(map[string]any, len(meta))
	for key, value := range meta {
		switch typed := value.(type) {
		case []interface{}:
			out[key] = splitCombinedDocumentMetadataList(typed)
		case []string:
			items := make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
			out[key] = splitCombinedDocumentMetadataList(items)
		default:
			out[key] = value
		}
	}
	return out
}

var combinedDocumentMetadataValueSplitter = regexp.MustCompile(`[、,，;；|]+`)

func splitCombinedDocumentMetadataList(items []any) []any {
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			out = append(out, item)
			continue
		}
		parts := combinedDocumentMetadataValueSplitter.Split(strings.TrimSpace(text), -1)
		added := false
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, part)
			added = true
		}
		if !added {
			out = append(out, item)
		}
	}
	return dedupeDocumentMetadataList(out)
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

func applyDocumentMetadataUpdates(meta map[string]interface{}, updates []MetadataUpdate) bool {
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
				switch strings.ToLower(strings.TrimSpace(upd.ValueType)) {
				case "string", "number", "time":
					meta[key] = normalizedValue
					changed = true
					continue
				}
			}
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

func applyDocumentMetadataDeletes(meta map[string]interface{}, deletes []MetadataDelete) bool {
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
		return []interface{}{value}
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
