package dataset

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/ingestion/component"
)

func (d *DatasetService) AggregateTags(ctx context.Context, datasetIDs []string, userID string) ([]map[string]interface{}, common.ErrorCode, error) {
	if len(datasetIDs) == 0 {
		return nil, common.CodeDataError, errors.New("Lack of dataset_ids in query parameters")
	}
	// merged holds the selectable-tag vocabulary sourced from the tag source
	// files (parser_config.tags.tag_file_id) declared by the dataset's
	// documents. A document copies the dataset config at upload and the
	// document parser dialog may then override it, and that document-level copy
	// is the one the extractor reads at parse time (see
	// ingestion/task/pipeline_executor.go) — so it, not the dataset row, is what
	// describes the tags actually in effect. A dataset-level value no document
	// carries has not been applied to anything yet and is deliberately not
	// reported: this endpoint answers "in effect", not "configured", so a
	// dataset configured after its documents were uploaded reads empty until
	// those documents are re-uploaded or configured individually. The count for
	// a tag is the number of times it appears in that source file. The Go
	// backend has no Python-style tag-library datasets, so there is no
	// chunk-level aggregation.
	loader := d.tagVocabularyLoader
	if loader == nil {
		loader = component.TagVocabularyFromTagFileID
	}
	merged := make(map[string]int)
	// The handler accepts repeated IDs, and distinct raw IDs can normalize to
	// the same dataset (hyphenated vs. compact form). Normalize up front so
	// each dataset is authorized, loaded and merged exactly once — otherwise a
	// repeated dataset (e.g. dataset_ids=A,A) would load the same vocabulary
	// twice and double every count.
	seen := make(map[string]struct{}, len(datasetIDs))
	orderedIDs := make([]string, 0, len(datasetIDs))
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
		orderedIDs = append(orderedIDs, datasetID)
	}

	// Authorize and load every dataset before reading any document config, so
	// nothing belonging to an inaccessible dataset is fetched.
	type authorizedDataset struct {
		id     string
		tenant string
	}
	authorized := make([]authorizedDataset, 0, len(orderedIDs))
	for _, datasetID := range orderedIDs {
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
		authorized = append(authorized, authorizedDataset{
			id:     datasetID,
			tenant: kb.TenantID,
		})
	}

	docConfigs, err := d.documentDAO.ListParserConfigsByKBIDs(ctx, dao.DB, orderedIDs)
	if err != nil {
		return nil, common.CodeServerError, errors.New("Database operation failed")
	}

	// Every source below comes from a document — that is the copy the extractor
	// reads, so only it can describe tags in effect. A source file is scoped to
	// its owning tenant and is normally referenced by every document seeded from
	// the dataset, so load each (tenant, file) once or its counts add up again.
	loaded := make(map[string]struct{})
	for _, ds := range authorized {
		sources := make(map[string]struct{}, len(docConfigs[ds.id]))
		for _, docConfig := range docConfigs[ds.id] {
			if id := component.TagFileIDFromParserConfig(map[string]any(docConfig)); id != "" {
				sources[id] = struct{}{}
			}
		}
		// Sorted so the vocabulary and any load error are deterministic when a
		// dataset carries more than one source file.
		ids := make([]string, 0, len(sources))
		for id := range sources {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		for _, tagFileID := range ids {
			key := ds.tenant + "\x00" + tagFileID
			if _, dup := loaded[key]; dup {
				continue
			}
			loaded[key] = struct{}{}
			// The file is resolved against the dataset's own tenant:
			// tag_file_id is user-writable, so a foreign file ID must not
			// resolve (IDOR, CWE-639).
			counts, vErr := loader(ctx, tagFileID, ds.tenant)
			if vErr != nil {
				// Nothing clears a stale reference when a tag file is deleted, and
				// a document's parser_config is never validated against the file
				// table, so an unresolvable id is skipped instead of letting it
				// discard every other dataset in the request. A database or
				// storage failure still fails loudly: it says nothing about this
				// id and may be environment-wide.
				if component.IsTagSourceNotFound(vErr) {
					common.Warn(fmt.Sprintf("tag_vocab: skipping unresolvable tag source %q for dataset %q: %v",
						tagFileID, ds.id, vErr))
					continue
				}
				return nil, common.CodeServerError,
					fmt.Errorf("load tag vocabulary for dataset %q: %w", ds.id, vErr)
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
