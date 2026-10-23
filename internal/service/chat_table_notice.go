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
	"strings"
	"sync"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// sqlUnavailableNotice is appended to a vector-search answer when the chat
// includes a table dataset that has no field_map, so the answer is not read
// as the result of table SQL retrieval.
const sqlUnavailableNotice = "Note: SQL retrieval is unavailable for the table dataset(s) %s (no field_map), so this answer was produced by vector search and may be incomplete for aggregate or exact-match questions."

// tableSQLWarned remembers the datasets already warned about so the log line
// fires once per dataset instead of once per chat request.
var tableSQLWarned sync.Map

// tableDatasetsWithoutFieldMap returns the names of table datasets whose
// parser config carries no usable field_map. It checks the dataset state only,
// never the engine, so datasets that do have a field_map are never reported.
func tableDatasetsWithoutFieldMap(kbs []*entity.Knowledgebase) []string {
	var names []string
	for _, kb := range kbs {
		if kb == nil || kb.ParserID != "table" {
			continue
		}
		if fm, ok := kb.ParserConfig["field_map"].(map[string]interface{}); ok && len(fm) > 0 {
			continue
		}
		name := kb.Name
		if name == "" {
			name = kb.ID
		}
		names = append(names, name)
	}
	return names
}

// warnTableSQLUnavailable logs once per dataset that SQL retrieval cannot run.
func warnTableSQLUnavailable(kbs []*entity.Knowledgebase) {
	for _, kb := range kbs {
		if kb == nil || len(tableDatasetsWithoutFieldMap([]*entity.Knowledgebase{kb})) == 0 {
			continue
		}
		if _, seen := tableSQLWarned.LoadOrStore(kb.ID, struct{}{}); seen {
			continue
		}
		common.Warn("table dataset has no field_map; SQL retrieval unavailable, chat answers via vector search",
			zap.String("kb_id", kb.ID), zap.String("kb_name", kb.Name))
	}
}

// appendSQLUnavailableNotice adds the notice to answer when names is non-empty.
func appendSQLUnavailableNotice(answer string, names []string) string {
	if len(names) == 0 || answer == "" {
		return answer
	}
	return answer + "\n\n" + strings.Replace(sqlUnavailableNotice, "%s", strings.Join(names, ", "), 1)
}
