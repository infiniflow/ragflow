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
)

func (d *DatasetService) AggregateTags(datasetIDs []string, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	if len(datasetIDs) == 0 {
		return nil, common.CodeDataError, errors.New("Lack of dataset_ids in query parameters")
	}
	if d.docEngine == nil {
		return nil, common.CodeServerError, errors.New("Document engine is not initialized")
	}

	datasetIDsByTenant := make(map[string][]string)
	for _, rawID := range datasetIDs {
		rawID = strings.TrimSpace(rawID)
		if rawID == "" {
			continue
		}
		datasetID, err := normalizeDatasetID(rawID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		if !d.kbDAO.Accessible(datasetID, userID) {
			return nil, common.CodeDataError, fmt.Errorf("No authorization for dataset '%s'", datasetID)
		}
		kb, err := d.kbDAO.GetByID(datasetID)
		if err != nil {
			if dao.IsNotFoundErr(err) {
				return nil, common.CodeDataError, fmt.Errorf("Invalid Dataset ID '%s'", datasetID)
			}
			return nil, common.CodeServerError, errors.New("Database operation failed")
		}
		if kb.DocNum <= 0 {
			continue
		}
		datasetIDsByTenant[kb.TenantID] = append(datasetIDsByTenant[kb.TenantID], datasetID)
	}

	const pageSize = 10000
	merged := make(map[string]int)
	for tenantID, kbIDs := range datasetIDsByTenant {
		for offset := 0; ; offset += pageSize {
			searchResp, err := d.docEngine.Search(context.Background(), &enginetypes.SearchRequest{
				IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
				KbIDs:        kbIDs,
				Offset:       offset,
				Limit:        pageSize,
				SelectFields: []string{"tag_kwd"},
			})
			if err != nil {
				return nil, common.CodeServerError, fmt.Errorf("failed to aggregate tags: %w", err)
			}
			for _, agg := range d.docEngine.GetAggregation(searchResp.Chunks, "tag_kwd") {
				tag, _ := agg["key"].(string)
				if tag == "" {
					continue
				}
				switch count := agg["count"].(type) {
				case int:
					merged[tag] += count
				case int32:
					merged[tag] += int(count)
				case int64:
					merged[tag] += int(count)
				case float64:
					merged[tag] += int(count)
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

func (d *DatasetService) ListTags(datasetID, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New("Lack of \"Dataset ID\"")
	}
	normalizedID, err := normalizeDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	datasetID = normalizedID
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}
	if d.docEngine == nil {
		return nil, common.CodeServerError, errors.New("Document engine is not initialized")
	}
	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil || kb == nil {
		return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
	}
	indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exists, err := d.docEngine.ChunkStoreExists(ctx, indexName, datasetID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to inspect chunk store: %w", err)
	}
	if !exists {
		return []map[string]interface{}{}, common.CodeSuccess, nil
	}
	const pageSize = 10000
	counts := make(map[string]int)
	for offset := 0; ; offset += pageSize {
		if err = ctx.Err(); err != nil {
			return nil, common.CodeServerError, fmt.Errorf("list tags timeout or canceled: %w", err)
		}
		searchResp, err := d.docEngine.Search(ctx, &enginetypes.SearchRequest{
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

func (d *DatasetService) RenameTag(datasetID, userID, fromTag, toTag string) (map[string]interface{}, common.ErrorCode, error) {
	fromTag = strings.TrimSpace(fromTag)
	toTag = strings.TrimSpace(toTag)
	datasetID, err := normalizeDatasetID(datasetID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	if strings.TrimSpace(datasetID) == "" {
		return nil, common.CodeDataError, errors.New("Lack of \"Dataset ID\"")
	}
	if !d.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, errors.New("No authorization.")
	}
	if d.docEngine == nil {
		return nil, common.CodeServerError, errors.New("Document engine is not initialized")
	}
	kb, err := d.kbDAO.GetByID(datasetID)
	if err != nil || kb == nil {
		return nil, common.CodeDataError, errors.New("Invalid Dataset ID")
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
	err = d.docEngine.UpdateChunks(context.Background(), condition, newValue, indexName, datasetID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to rename tag: %w", err)
	}
	return map[string]interface{}{
		"from": fromTag,
		"to":   toTag,
	}, common.CodeSuccess, nil
}
