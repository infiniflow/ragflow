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

package indexdoc

import (
	"encoding/json"
	"sort"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

// ProjectTableChunks reports what this run indexed from spreadsheet rows: the
// document's derived profile, and the document-level column values those rows
// contribute to metadata. Both are nil when the run indexed no table rows.
//
// It reads the row source information the index boundary is about to drop,
// because that is the only place the column identity and the roles in force are
// recorded together. Deriving either of them from parser_config afterwards is
// guesswork: a canvas with several TableChunker nodes, or a document whose roles
// changed since these rows were written, would be attributed to whichever
// configuration entry a map walk happens to reach first.
//
// Only rows that actually carry structured values add columns to the profile, so
// a field_map never advertises a column no chunk_data holds.
func ProjectTableChunks(chunks []map[string]any, engineName string) (*entity.TableProfile, map[string]any) {
	profile := &entity.TableProfile{
		Engine:  engineName,
		Columns: []entity.TableColumn{},
	}
	columns := map[string]entity.TableColumn{}
	values := map[string]map[string]struct{}{}

	for _, ck := range chunks {
		if ck["table_row_source"] == nil {
			continue
		}
		raw, err := json.Marshal(ck["table_row_source"])
		if err != nil {
			continue
		}
		var src schema.TableRowSource
		if err := json.Unmarshal(raw, &src); err != nil {
			continue
		}
		data, _ := ck["chunk_data"].(map[string]any)

		for _, col := range src.Columns {
			value, written := data[col.DataKey].(string)
			if !written {
				continue
			}
			// The profile is a cross-sheet union keyed by data_key, so a header position
			// is not meaningful here: two sheets can put the same header at different
			// ones. Keep the identity, drop the position.
			col.Index = 0
			columns[col.DataKey] = col
			if src.Mode != entity.TableModeManual || !aggregates(col.Key, src.Roles) {
				continue
			}
			if value == "" {
				continue
			}
			if _, ok := values[col.Key]; !ok {
				values[col.Key] = map[string]struct{}{}
			}
			values[col.Key][value] = struct{}{}
		}
	}

	if len(columns) == 0 && len(values) == 0 {
		return nil, nil
	}
	for _, col := range columns {
		profile.Columns = append(profile.Columns, col)
	}
	if len(values) == 0 {
		return profile, nil
	}
	out := make(map[string]any, len(values))
	for key, set := range values {
		deduped := make([]string, 0, len(set))
		for v := range set {
			deduped = append(deduped, v)
		}
		sort.Strings(deduped)
		out[key] = deduped
	}
	return profile, out
}

// aggregates reports whether a column's values belong in document metadata:
// manual mode and an explicitly configured metadata or both role. A manual
// column with no entry is written to chunk_data as "both" but contributes no
// document value, which is what keeps a wide table from turning its whole
// content into document metadata.
func aggregates(key string, roles map[string]string) bool {
	switch roles[key] {
	case entity.TableRoleMetadata, entity.TableRoleBoth:
		return true
	}
	return false
}
