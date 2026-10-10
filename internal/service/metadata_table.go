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
	"fmt"
	"sort"

	"ragflow/internal/entity"
)

// TableFieldMap returns the structured columns the given knowledge bases have
// indexed: the union over their enabled documents' derived table profiles, and
// the document IDs that contributed it.
//
// Each document's indexed columns are recorded in its own metadata record when
// a table run publishes (entity.TableProfile), so the knowledge
// base view is computed from the documents that actually hold rows. Nothing is
// stored a second time per knowledge base, which is why a document that fails
// to publish, is disabled, or was indexed under another engine simply does not
// appear here rather than needing a cleanup pass.
//
// Missing or incomplete profiles do not contribute columns. Malformed profile
// JSON and metadata-store failures are returned, so callers do not interpret
// a publisher or infrastructure failure as a knowledge base with no columns.
//
// Whether the result may be used for SQL is the caller's decision
// (SupportsStructuredTableSQL): a read-only view of indexed fields is useful on
// engines that cannot query them.
func (s *MetadataService) TableFieldMap(ctx context.Context, kbIDs []string) (map[string]interface{}, []string, error) {
	if len(kbIDs) == 0 || s.docEngine == nil {
		return nil, nil, nil
	}
	engineName := s.docEngine.GetType()

	docIDsByKB, err := s.documentDAO.ListEnabledIDsByKBIDs(ctx, s.db, kbIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list enabled documents: %w", err)
	}

	fields := make(map[string]interface{})
	var firstErr error
	contributed := make([]string, 0)
	for _, kbID := range kbIDs {
		docIDs := docIDsByKB[kbID]
		if len(docIDs) == 0 {
			continue
		}
		tenantID, err := s.GetTenantIDByKBID(ctx, kbID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		result, err := s.SearchMetadata(ctx, kbID, tenantID, docIDs, len(docIDs))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, record := range result.MetadataRecords {
			docID := extractDocID(record)
			if docID == "" {
				continue
			}
			metaFields, err := ExtractMetaFields(record)
			if err != nil {
				continue
			}
			profile, ok, err := entity.DecodeTableProfile(metaFields[entity.TableProfileMetadataField])
			if err != nil {
				// A record that is present but unparseable is a publisher bug;
				// say so rather than dropping the document in silence.
				if firstErr == nil {
					firstErr = fmt.Errorf("decode table profile of document %s: %w", docID, err)
				}
				continue
			}
			if !ok || profile.Engine != engineName {
				continue
			}
			for dataKey, name := range profile.FieldMap() {
				if known, exists := fields[dataKey]; exists && known != name {
					if firstErr == nil {
						firstErr = fmt.Errorf("document %s maps column %q to %q, another document maps it to %q",
							docID, dataKey, name, known)
					}
					continue
				}
				fields[dataKey] = name
			}
			contributed = append(contributed, docID)
		}
	}

	if len(fields) == 0 {
		return nil, nil, firstErr
	}
	sort.Strings(contributed)
	return fields, contributed, firstErr
}
