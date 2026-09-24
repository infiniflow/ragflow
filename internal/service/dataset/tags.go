package dataset

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/ingestion/component"
)

func (d *DatasetService) AggregateTags(ctx context.Context, datasetIDs []string, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	if len(datasetIDs) == 0 {
		return nil, common.CodeDataError, errors.New("Lack of dataset_ids in query parameters")
	}
	// merged holds the selectable-tag vocabulary sourced exclusively from each
	// dataset's configured tag source file (parser_config.tags.tag_file_id). The
	// count for a tag is the number of times it appears in that source file.
	// The Go backend has no Python-style tag-library datasets, so there is no
	// chunk-level aggregation.
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
		if tagFileID := component.TagFileIDFromParserConfig(map[string]any(kb.ParserConfig)); tagFileID != "" {
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
