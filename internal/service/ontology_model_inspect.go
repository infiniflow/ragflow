/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package service

import (
	"strconv"
	"strings"

	"ragflow/internal/dao"
)

// A compiled row is stored per dataset (the engine queries pin one kb_id), so
// "which datasets does this model reach" is the reverse index in
// dataset_model_ref. It is what the review queue reads across, and why the
// selection below exists: the ABox itself is per dataset and is read one
// dataset at a time (see DatasetArtifactService.GetOntologyClassEntities).
const (
	// InspectDatasetsMax bounds how many datasets one cross-dataset read takes.
	// The read is one engine query per dataset, so an unbounded model would turn
	// a panel open into a scan of the tenant.
	InspectDatasetsMax = 20
)

// ModelInspectScope is how wide a cross-dataset read goes.
type ModelInspectScope string

const (
	// ModelInspectScopeDoc reads one document's rows (chunk-level evidence).
	ModelInspectScopeDoc ModelInspectScope = "doc"
	// ModelInspectScopeDataset reads one dataset's dataset-scope rows, which
	// already fold every document in that dataset — no per-document fan-out.
	ModelInspectScopeDataset ModelInspectScope = "dataset"
	// ModelInspectScopeCorpus reads every dataset that names the model.
	ModelInspectScopeCorpus ModelInspectScope = "corpus"
)

// NormalizeModelInspectScope maps a query value onto a scope. Anything
// unrecognized means "the dataset", which is the narrowest scope that still
// shows compiled rows.
func NormalizeModelInspectScope(raw string) ModelInspectScope {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ModelInspectScopeDoc):
		return ModelInspectScopeDoc
	case string(ModelInspectScopeCorpus), "all", "fleet":
		return ModelInspectScopeCorpus
	default:
		return ModelInspectScopeDataset
	}
}

// ModelInspectDataset is one dataset a cross-dataset read touched.
type ModelInspectDataset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	DocNum int64  `json:"doc_num"`
}

// inspectSelectDatasets picks the datasets to read, in the order the index
// returned them, and reports the ones the cap left out.
func inspectSelectDatasets(bound []*dao.ModelImpactDataset, scope ModelInspectScope, wanted []string) ([]ModelInspectDataset, []string) {
	filter := map[string]bool{}
	for _, id := range wanted {
		if id = strings.TrimSpace(id); id != "" {
			filter[id] = true
		}
	}
	picked := make([]ModelInspectDataset, 0, len(bound))
	skipped := []string{}
	for _, ds := range bound {
		if ds == nil {
			continue
		}
		if len(filter) > 0 && !filter[ds.ID] {
			continue
		}
		if len(picked) >= InspectDatasetsMax {
			// Only a corpus read can overflow: dataset scope is a subset of the
			// cap, and doc scope is one dataset.
			skipped = append(skipped, ds.ID)
			continue
		}
		picked = append(picked, ModelInspectDataset{ID: ds.ID, Name: ds.Name, DocNum: ds.DocNum})
	}
	return picked, skipped
}

// inspectRowInt reads an integer that the engine may return as a number, a
// string, or a single-element list.
func inspectRowInt(row map[string]interface{}, key string) int64 {
	switch v := row[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}
