package dataset

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
)

// tagFileIDFromTagsMap pulls tag_file_id out of a raw "tags" config value
// (i.e. parser_config["tags"] in the legacy flat form, or
// parser_config["Extractor:<name>"]["tags"] in the Go pipeline scoped form),
// accepting either a map[string]any or an entity.JSONMap. Returns "" when the
// value is not a tags map or has no tag_file_id. In the Go backend the
// authoritative tag vocabulary lives in this tag source file (parsed by the
// extractor in extractor_tag.go). Note: the Go tag extractor (matchAndTagChunk)
// DOES write tag_kwd onto chunks at parse time, but the selectable-tag list is
// sourced from this vocabulary, not from chunk usage. Returns "" when unset.
func tagFileIDFromTagsMap(raw any) string {
	if raw == nil {
		return ""
	}
	var tags map[string]any
	switch v := raw.(type) {
	case map[string]any:
		tags = v
	case entity.JSONMap:
		tags = map[string]any(v)
	default:
		return ""
	}
	if v, ok := tags["tag_file_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// extractTagFileID returns the tag source file ID declared in a dataset's
// parser_config. The tags config lives in two possible shapes depending on how
// the dataset was persisted:
//
//   - Legacy / flat form: parser_config["tags"]["tag_file_id"].
//   - Go pipeline scoped form: parser_config["Extractor:<name>"]["tags"]
//     ["tag_file_id"] (e.g. "Extractor:AutoExtractDefault"). In this form the
//     component config is keyed by "<Component>:<InstanceName>" and the
//     sub-feature maps (tags/keywords/questions/summary/metadata) sit at the
//     top level of that value — mirroring how NewExtractorComponent reads its
//     params.
//
// The Go backend sources the authoritative tag vocabulary from this tag source
// file (parsed by the extractor in extractor_tag.go). Note: the Go tag extractor
// (matchAndTagChunk) DOES write tag_kwd onto chunks at parse time, but the
// selectable-tag list is taken from this vocabulary, not from chunk usage.
// Returns "" when unset.
func extractTagFileID(parserConfig entity.JSONMap) string {
	if parserConfig == nil {
		return ""
	}
	// 1. Legacy / flat form.
	if v := tagFileIDFromTagsMap(parserConfig["tags"]); v != "" {
		return v
	}
	// 2. Go pipeline scoped form: any key shaped "<Component>:<Instance>" whose
	//    component is Extractor.
	for key, raw := range parserConfig {
		if key == "Extractor" || strings.HasPrefix(key, "Extractor:") {
			extractorMap, ok := raw.(map[string]any)
			if !ok {
				// entity.JSONMap is an alias of map[string]any, so the assertion
				// above already covers it; this guard handles any other wrapper.
				if jm, ok2 := raw.(entity.JSONMap); ok2 {
					extractorMap = map[string]any(jm)
				} else {
					continue
				}
			}
			// The tags config sits at the top level of the Extractor node value,
			// i.e. parser_config["Extractor:<name>"]["tags"], NOT at the node
			// value itself. Pass that nested "tags" map to tagFileIDFromTagsMap.
			if v := tagFileIDFromTagsMap(extractorMap["tags"]); v != "" {
				return v
			}
		}
	}
	return ""
}

func (d *DatasetService) AggregateTags(ctx context.Context, datasetIDs []string, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	if len(datasetIDs) == 0 {
		return nil, common.CodeDataError, errors.New("Lack of dataset_ids in query parameters")
	}
	// merged holds the selectable-tag vocabulary sourced exclusively from each
	// dataset's configured tag source file (parser_config.tags.tag_file_id). The
	// count for a tag is the number of times it appears in that source file. The
	// Go tag extractor (matchAndTagChunk) does write tag_kwd onto chunks at parse
	// time, but by requirement the count reflects only the tag-file occurrence
	// count, so there is intentionally no chunk-level aggregation here. The Go
	// backend has no Python-style tag-library datasets.
	loader := d.tagVocabularyLoader
	if loader == nil {
		loader = component.TagVocabularyFromTagFileID
	}
	merged := make(map[string]int)
	// The handler accepts repeated IDs, and distinct raw IDs can normalize to
	// the same dataset (hyphenated vs. compact form). Track the normalized IDs
	// so each dataset is loaded and merged exactly once — otherwise a repeated
	// dataset (e.g. dataset_ids=A,A) would load the same vocabulary twice and
	// double every count.
	seen := make(map[string]struct{}, len(datasetIDs))
	for _, rawID := range datasetIDs {
		rawID = strings.TrimSpace(rawID)
		if rawID == "" {
			continue
		}
		datasetID, err := normalizeDatasetID(rawID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		if _, dup := seen[datasetID]; dup {
			continue
		}
		seen[datasetID] = struct{}{}
		if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
			return nil, common.CodeDataError, fmt.Errorf("No authorization for dataset '%s'", datasetID)
		}
		kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
		if err != nil {
			if dao.IsNotFoundErr(err) {
				return nil, common.CodeDataError, fmt.Errorf("Invalid Dataset ID '%s'", datasetID)
			}
			return nil, common.CodeServerError, errors.New("Database operation failed")
		}

		// Include the dataset's tag-source vocabulary. The count for each tag is
		// the number of times it occurs in the tag source file. The file is
		// resolved against the dataset's own tenant: tag_file_id is user-writable,
		// so a foreign file ID must not resolve (IDOR, CWE-639).
		if tagFileID := extractTagFileID(kb.ParserConfig); tagFileID != "" {
			counts, vErr := loader(ctx, tagFileID, kb.TenantID)
			if vErr != nil {
				return nil, common.CodeServerError,
					fmt.Errorf("load tag vocabulary for dataset %q: %w", datasetID, vErr)
			}
			for tag, c := range counts {
				merged[tag] += c
			}
		}
	}
	result := make([]map[string]interface{}, 0, len(merged))
	for tag, count := range merged {
		result = append(result, map[string]interface{}{
			"value": tag,
			"count": count,
		})
	}
	return result, common.CodeSuccess, nil
}

func (d *DatasetService) ListTags(ctx context.Context, datasetID, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New("lack of \"Dataset ID\"")
	}
	normalizedID, err := normalizeDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	datasetID = normalizedID
	if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
		return nil, common.CodeDataError, errors.New("no authorization")
	}
	if d.docEngine == nil {
		return nil, common.CodeServerError, errors.New("document engine is not initialized")
	}
	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil || kb == nil {
		return nil, common.CodeDataError, errors.New("invalid Dataset ID")
	}
	indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
	newCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	exists, err := d.docEngine.ChunkStoreExists(newCtx, indexName, datasetID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to inspect chunk store: %w", err)
	}
	if !exists {
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}
	const pageSize = 10000
	counts := make(map[string]int)
	for offset := 0; ; offset += pageSize {
		if err = newCtx.Err(); err != nil {
			return nil, common.CodeServerError, fmt.Errorf("list tags timeout or canceled: %w", err)
		}
		searchResp, err := d.docEngine.Search(newCtx, &enginetypes.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{datasetID},
			Offset:       offset,
			Limit:        pageSize,
			SelectFields: []string{"tag_kwd"},
		})
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to list tags: %w", err)
		}
		for _, agg := range d.docEngine.GetAggregation(searchResp.Chunks, "tag_kwd") {
			tag, _ := agg["key"].(string)
			if tag == "" {
				continue
			}
			switch count := agg["count"].(type) {
			case int:
				counts[tag] += count
			case int32:
				counts[tag] += int(count)
			case int64:
				counts[tag] += int(count)
			case float64:
				counts[tag] += int(count)
			}
		}
		chunkCount := len(searchResp.Chunks)
		if chunkCount == 0 || chunkCount < pageSize {
			break
		}
		if searchResp.Total > 0 && int64(offset+chunkCount) >= searchResp.Total {
			break
		}
	}
	if len(counts) == 0 {
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}
	tags := make([]string, 0, len(counts))
	for tag := range counts {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		if counts[tags[i]] != counts[tags[j]] {
			return counts[tags[i]] > counts[tags[j]]
		}
		return tags[i] < tags[j]
	})
	result := make([]map[string]interface{}, 0, len(tags))
	for _, tag := range tags {
		result = append(result, map[string]interface{}{
			"key":   tag,
			"count": counts[tag],
		})
	}
	return result, common.CodeSuccess, nil
}

func (d *DatasetService) RenameTag(ctx context.Context, datasetID, userID, fromTag, toTag string) (map[string]interface{}, common.ErrorCode, error) {
	fromTag = strings.TrimSpace(fromTag)
	toTag = strings.TrimSpace(toTag)
	datasetID, err := normalizeDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	if strings.TrimSpace(datasetID) == "" {
		return nil, common.CodeDataError, errors.New("lack of \"Dataset ID\"")
	}
	if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
		return nil, common.CodeDataError, errors.New("no authorization")
	}
	if d.docEngine == nil {
		return nil, common.CodeServerError, errors.New("document engine is not initialized")
	}
	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil || kb == nil {
		return nil, common.CodeDataError, errors.New("invalid Dataset ID")
	}
	indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
	condition := map[string]interface{}{
		"tag_kwd": fromTag,
		"kb_id":   datasetID,
	}
	newValue := map[string]interface{}{
		"remove": map[string]interface{}{
			"tag_kwd": fromTag,
		},
		"add": map[string]interface{}{
			"tag_kwd": toTag,
		},
	}
	err = d.docEngine.UpdateChunks(ctx, condition, newValue, indexName, datasetID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to rename tag: %w", err)
	}
	return map[string]interface{}{
		"from": fromTag,
		"to":   toTag,
	}, common.CodeSuccess, nil
}
