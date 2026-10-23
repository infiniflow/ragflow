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
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"

	"gorm.io/gorm"
)

// A model is one template compiled by many documents across many datasets,
// while every compiled row is stored per dataset (the engine queries pin one
// kb_id). Inspect therefore answers "what did this model produce?" by fanning
// out over the datasets that name it — the reverse index in dataset_model_ref —
// and merging their dataset-scope rows. There is deliberately no cross-dataset
// row store: entity identity is a per-compilation hash over (template, doc,
// name), so the same name in two datasets is two instances, and merging them
// would invent an identity the data does not have.
const (
	// InspectDatasetsMax bounds how many datasets one inspection reads. The
	// fan-out is one engine query per dataset, so an unbounded model would turn
	// a panel open into a scan of the tenant.
	InspectDatasetsMax = 20
	// inspectInstancesDefaultLimit / Max bound the merged page.
	inspectInstancesDefaultLimit = 50
	inspectInstancesMaxLimit     = 200
)

// ModelInspectScope is how wide an inspection reads.
type ModelInspectScope string

const (
	// ModelInspectScopeDoc reads one document's rows (chunk-level evidence).
	ModelInspectScopeDoc ModelInspectScope = "doc"
	// ModelInspectDataset reads one dataset's dataset-scope rows, which already
	// fold every document in that dataset — no per-document fan-out.
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

// ModelInspectKind is which compiled row kind an inspection answers. The panel's
// two tabs are two kinds, not two filters: instances are entity rows and
// assertions are relation rows.
type ModelInspectKind string

const (
	ModelInspectKindEntity   ModelInspectKind = "entity"
	ModelInspectKindRelation ModelInspectKind = "relation"
)

// NormalizeModelInspectKind maps a request onto the kind it answers.
//
// The panel says it outright, because a tab is what the reader is looking at. A
// named class or property also settles it — class names entities, property names
// relations — so a request that names one is answered even without the kind. A
// request that names neither gets the instances, the tab the panel opens on.
func NormalizeModelInspectKind(raw, class, property string) ModelInspectKind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "relation", "relations", "assertion", "assertions":
		return ModelInspectKindRelation
	case "entity", "entities", "instance", "instances":
		return ModelInspectKindEntity
	}
	if strings.TrimSpace(class) == "" && strings.TrimSpace(property) != "" {
		return ModelInspectKindRelation
	}
	return ModelInspectKindEntity
}

// ModelInspectInput asks what a model produced over its datasets: the instances
// of one class, the assertions of one property, or — with neither named — every
// row of the requested kind.
type ModelInspectInput struct {
	TenantID   string
	TemplateID string
	// Kind is the row kind to answer. Empty is resolved by
	// NormalizeModelInspectKind.
	Kind string
	// Class is a declared class name; Property a declared property name. At most
	// one of them narrows the answer, and an empty one means "every class" /
	// "every property" rather than "nothing": the panel opens with nothing
	// selected, and "what did this model produce at all" is the first question an
	// author asks of it.
	Class      string
	Property   string
	SourceType string
	TargetType string
	// Scope defaults to the dataset.
	Scope ModelInspectScope
	// DatasetIDs narrows the fan-out. Empty means every dataset of the model
	// (still capped by InspectDatasetsMax).
	DatasetIDs []string
	// DocumentID narrows to one document; only meaningful with ModelInspectScopeDoc.
	DocumentID string
	Limit      int
}

// ModelInspectDataset is one dataset the fan-out read.
type ModelInspectDataset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	DocNum int64  `json:"doc_num"`
}

// ModelInspectResult is the merged view. Entities and Relations carry
// dataset_id/dataset_name on every row so the reader can tell which library an
// instance came from.
type ModelInspectResult struct {
	TemplateID string `json:"template_id"`
	Scope      string `json:"scope"`
	// Kind says which of the two row lists is the answer. It is reported because
	// an unfiltered read names neither a class nor a property, so nothing else in
	// the payload would say which list was populated.
	Kind     string `json:"kind"`
	Class    string `json:"class,omitempty"`
	Property string `json:"property,omitempty"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target,omitempty"`
	// ModelVersion is the version in force. A row whose own version is lower was
	// extracted by an earlier one and has not been recompiled since.
	ModelVersion int `json:"model_version"`
	// Refs are the references the datasets were resolved from: the model's group
	// and the template itself. Reported so an empty panel can say what was looked
	// up, instead of leaving the reader to guess why nothing matched.
	Refs []common.ModelRef `json:"refs,omitempty"`
	// Datasets are the datasets actually read, SkippedDatasetIDs those the cap
	// left out — the difference between "no instances" and "did not look" has to
	// be visible, or an empty panel reads as a defect in the model.
	Datasets          []ModelInspectDataset    `json:"datasets"`
	SkippedDatasetIDs []string                 `json:"skipped_dataset_ids,omitempty"`
	Total             int64                    `json:"total"`
	Truncated         bool                     `json:"truncated"`
	Entities          []StructureGraphNode     `json:"entities"`
	Relations         []StructureGraphRelation `json:"relations"`
}

// InspectInstances answers what a model produced, for one class or property —
// or, when the reader has not picked one, everything of the requested kind —
// over the datasets that compile with it.
func (s *CompilationTemplateGovernanceService) InspectInstances(ctx context.Context, db *gorm.DB, in ModelInspectInput) (*ModelInspectResult, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, in.TenantID, in.TemplateID)
	if err != nil {
		return nil, err
	}
	scope := NormalizeModelInspectScope(string(in.Scope))
	kind := NormalizeModelInspectKind(in.Kind, in.Class, in.Property)
	// Both references, not the preferred one: a dataset that names the template
	// directly reaches this model just as much as one that names its group, and
	// resolving only one makes a model in use look unused. Rows are filtered by
	// template id below, so a dataset pulled in through its group contributes
	// only this template's rows.
	refs := modelRefsFor(template)
	bound, err := dao.NewDatasetModelRefDAO().DatasetsByModelRefs(ctx, db, in.TenantID, refs)
	if err != nil {
		return nil, err
	}
	datasets, skipped := inspectSelectDatasets(bound, scope, in.DatasetIDs)

	limit := inspectInstancesDefaultLimit
	if in.Limit > 0 {
		limit = in.Limit
	}
	if limit > inspectInstancesMaxLimit {
		limit = inspectInstancesMaxLimit
	}

	result := &ModelInspectResult{
		TemplateID:        template.ID,
		Scope:             string(scope),
		Kind:              string(kind),
		ModelVersion:      template.Version,
		Refs:              refs,
		Datasets:          datasets,
		SkippedDatasetIDs: skipped,
		Truncated:         len(skipped) > 0,
		Entities:          []StructureGraphNode{},
		Relations:         []StructureGraphRelation{},
	}
	// Only the dimensions actually answered are echoed. A response that named a
	// class while carrying assertions would describe a query that never ran.
	if kind == ModelInspectKindRelation {
		result.Property = strings.TrimSpace(in.Property)
		result.Source = strings.TrimSpace(in.SourceType)
		result.Target = strings.TrimSpace(in.TargetType)
	} else {
		result.Class = strings.TrimSpace(in.Class)
	}

	artifact := NewDatasetArtifactService()
	documentID := inspectDocumentID(scope, in.DocumentID)
	for _, ds := range datasets {
		if kind == ModelInspectKindRelation {
			page, err := artifact.GetOntologyPropertyRelations(ctx, OntologyPropertyRelationsInput{
				TenantID: in.TenantID, DatasetID: ds.ID, Property: result.Property,
				// No property named: the reader asked for the whole page, not for
				// an unnamed one.
				All:        result.Property == "",
				SourceType: result.Source, TargetType: result.Target,
				TemplateID: template.ID, DocumentID: documentID,
				Limit: limit,
			})
			if err != nil {
				return nil, err
			}
			inspectNotePage(result, len(page.Relations), limit)
			for _, edge := range page.Relations {
				result.Relations = append(result.Relations, inspectAnnotateRow(edge, ds))
			}
			continue
		}
		page, err := artifact.GetOntologyClassEntities(ctx, OntologyClassEntitiesInput{
			TenantID: in.TenantID, DatasetID: ds.ID, Class: result.Class,
			All:        result.Class == "",
			TemplateID: template.ID, DocumentID: documentID,
			Limit: limit,
		})
		if err != nil {
			return nil, err
		}
		inspectNotePage(result, len(page.Entities), limit)
		for _, node := range page.Entities {
			result.Entities = append(result.Entities, inspectAnnotateRow(node, ds))
		}
	}

	sortInspectEntities(result.Entities)
	sortInspectRelations(result.Relations)
	if len(result.Entities) > limit {
		result.Entities = result.Entities[:limit]
		result.Truncated = true
	}
	if len(result.Relations) > limit {
		result.Relations = result.Relations[:limit]
		result.Truncated = true
	}
	return result, nil
}

// inspectNotePage folds one dataset's page into the result.
//
// It counts the rows the dataset actually returned rather than the engine's
// match total: a dataset-scope query matches both a document's rows and the
// dataset's folded row for the same name, which the projection then dedups, so
// the engine's total counts rows the reader will never see. A window that came
// back full is the honest signal that there may be more.
func inspectNotePage(result *ModelInspectResult, returned, limit int) {
	result.Total += int64(returned)
	if returned >= limit {
		result.Truncated = true
	}
}

// inspectDocumentID is the document predicate for the requested scope. A
// document id under any other scope would silently narrow the fan-out to rows
// that carry it, which is not what "dataset scope" means.
func inspectDocumentID(scope ModelInspectScope, documentID string) string {
	if scope != ModelInspectScopeDoc {
		return ""
	}
	return strings.TrimSpace(documentID)
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

// inspectAnnotateRow stamps the source dataset onto a merged row, so the reader
// can tell two same-named instances in two libraries apart.
func inspectAnnotateRow(row map[string]interface{}, ds ModelInspectDataset) map[string]interface{} {
	row["dataset_id"] = ds.ID
	row["dataset_name"] = ds.Name
	return row
}

// sortInspectEntities orders the merged page by mention count, then name and
// dataset, so paging is deterministic across datasets.
func sortInspectEntities(rows []StructureGraphNode) {
	sort.SliceStable(rows, func(i, j int) bool {
		return inspectRowOrder(rows[i], rows[j], "name")
	})
}

func sortInspectRelations(rows []StructureGraphRelation) {
	sort.SliceStable(rows, func(i, j int) bool {
		return inspectRowOrder(rows[i], rows[j], "from")
	})
}

func inspectRowOrder(a, b map[string]interface{}, nameKey string) bool {
	am, bm := inspectRowInt(a, "mention_count"), inspectRowInt(b, "mention_count")
	if am != bm {
		return am > bm
	}
	an, bn := inspectRowString(a, nameKey), inspectRowString(b, nameKey)
	if an != bn {
		return an < bn
	}
	return inspectRowString(a, "dataset_id") < inspectRowString(b, "dataset_id")
}

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

func inspectRowString(row map[string]interface{}, key string) string {
	s, _ := row[key].(string)
	return s
}
