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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// The two thresholds the model workspace and the publish gate share.
//
// They are not preferences: below the blocker line the model cannot answer
// enough of what it was declared to answer, which is a statement about the
// model, not about this compile. Between the two lines the corpus is the more
// likely explanation (a dataset that has only been half compiled), so the
// publisher is allowed to overrule it.
const (
	CompetencyBlockerCoverage = 0.60
	CompetencyWarningCoverage = 0.80
)

// The kinds of element a competency question can require. A question is written
// as a list of these, and each one is checked against the measured graph — which
// is why the answer is a count and not a judgement.
const (
	competencyClass    = "class"
	competencyProperty = "property"
	competencyRelation = "relation"
)

// OntologyDriftField is one declared attribute as the corpus filled it.
type OntologyDriftField struct {
	Type string `json:"type"`
	// Assertions is how many instances of the owning class carry a value, and
	// Instances is how many instances the class has. The ratio is what the drift
	// panel shows; the two raw numbers are kept because a ratio over four
	// instances is not a finding.
	Assertions int     `json:"assertions"`
	Instances  int     `json:"instances"`
	FillRate   float64 `json:"fill_rate"`
	// Required mirrors the declaration, and Missing is how many instances lack a
	// value the model says they must have.
	Required bool `json:"required,omitempty"`
	Missing  int  `json:"missing,omitempty"`
	// InheritedFrom names the ancestor that declares the attribute, empty when
	// the class declares it itself. A drift reader who is about to widen a
	// declaration needs to edit the declaring class, not the one on screen.
	InheritedFrom string `json:"inherited_from,omitempty"`
}

// OntologyDriftClass is one class as observed against the declaration.
type OntologyDriftClass struct {
	Type        string `json:"type"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	// Entities is the instance count the compile produced.
	Entities int `json:"entities"`
	// Declared is false for a class the corpus produced that the model does not
	// name. That is the "补漏" case: the only drift finding that can be fixed by
	// declaring something.
	Declared bool `json:"declared"`
	// Fields is the attribute ledger. It is populated for every class, declared
	// or not, because "declared but never filled" and "observed but not
	// declared" are read side by side.
	Fields []OntologyDriftField `json:"fields,omitempty"`
}

// OntologyDriftProperty is one declared relation that the corpus never exercised.
type OntologyDriftProperty struct {
	Type   string `json:"type"`
	Label  string `json:"label,omitempty"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// OntologyDrift is the data-authorised half of the ledger: what the corpus says
// about the declaration. It deliberately carries no suggestion — a suggestion is
// the requirement half's job — only the two directions of the comparison.
type OntologyDrift struct {
	// UndeclaredClasses are observed, not declared. These are the only findings
	// that can be resolved by writing a declaration.
	UndeclaredClasses []OntologyDriftClass `json:"undeclared_classes"`
	// UnusedClasses are declared and never observed. A declaration the corpus
	// never exercised is either a missing extractor instruction or a class that
	// is not needed; the number cannot tell which.
	UnusedClasses []OntologyDriftClass `json:"unused_classes"`
	// UnusedProperties are declared relations with zero assertions.
	UnusedProperties []OntologyDriftProperty `json:"unused_properties"`
	// Classes is the full declared ledger with fill rates, so the panel can show
	// "field due_date 61% filled" without a second request per class.
	Classes []OntologyDriftClass `json:"classes"`
	// Totals and the quality counters travel with the drift because a reader
	// who sees "3 undeclared classes" has to know whether the scan covered the
	// whole dataset.
	TotalEntities    int  `json:"total_entities"`
	TotalRelations   int  `json:"total_relations"`
	DroppedRelations int  `json:"dropped_relations"`
	UntypedEntities  int  `json:"untyped_entities"`
	Truncated        bool `json:"truncated"`
}

// CompetencyElementResult is one required element of a question, checked.
type CompetencyElementResult struct {
	Kind string `json:"kind"`
	// Name is the element as written in the model: a class name, `class.field`
	// for a property, `from->to` for a relation.
	Name string `json:"name"`
	// Required is false for an element written with a trailing `?`. An optional
	// element that is missing lowers nothing; it is recorded so the panel can
	// show the whole question.
	Required bool `json:"required"`
	// Satisfied means the element exists AND the corpus exercised it. A declared
	// class with zero instances is not satisfied: the question would return
	// nothing, which is precisely what "can this model answer it" has to catch.
	Satisfied bool `json:"satisfied"`
	// Observed is the count behind the verdict: instances for a class, assertions
	// for a property or a relation. Zero with Declared true is a corpus gap.
	Observed int `json:"observed"`
	// Declared separates "the model never mentions this" from "the model
	// mentions it and the corpus produced nothing". The two need different fixes
	// and the panel cannot guess which one it is looking at.
	Declared bool `json:"declared"`
	// Reason is a stable machine code, not prose: `undeclared`, `not_extracted`,
	// `untyped`. The UI translates it.
	Reason string `json:"reason,omitempty"`
}

// CompetencyQuestionCoverage is one question's answerability.
type CompetencyQuestionCoverage struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	// Required / Satisfied count required elements only.
	Required  int `json:"required"`
	Satisfied int `json:"satisfied"`
	// Answerable is all required elements satisfied.
	Answerable bool                      `json:"answerable"`
	Elements   []CompetencyElementResult `json:"elements"`
}

// OntologyInsightPayload is what one measurement says. Drift and questions live
// together because they are folds of the same scan and any reader that shows one
// is one tab away from the other.
type OntologyInsightPayload struct {
	Drift     OntologyDrift                `json:"drift"`
	Questions []CompetencyQuestionCoverage `json:"questions"`
	// Required / Satisfied aggregate over every question's required elements.
	Required  int `json:"required"`
	Satisfied int `json:"satisfied"`
}

// Coverage is the ratio the publish gate thresholds against.
//
// No questions at all is a pass: a model that has not promised anything cannot
// fail to deliver. Questions that declare no required elements is NOT a pass —
// the denominator is empty because the question was never traced, and reporting
// 100% would let an untraced promise count as a measured one.
func (p OntologyInsightPayload) Coverage() float64 {
	if p.Required == 0 {
		if len(p.Questions) == 0 {
			return 1
		}
		return 0
	}
	return float64(p.Satisfied) / float64(p.Required)
}

// OntologyInsightView is one measurement as the workspace reads it.
type OntologyInsightView struct {
	TemplateID string `json:"template_id"`
	DatasetID  string `json:"dataset_id"`
	// Version is the model version the rows were compiled under, and AsOf when
	// the scan ran. Together they answer "what is this number about".
	Version int   `json:"version"`
	AsOf    int64 `json:"as_of"`
	// Stale reports that the model has been published since. The numbers are not
	// wrong — the corpus really looks like this — but comparing them against the
	// current draft is.
	Stale bool `json:"stale"`
	// DraftPending reports unsaved-to-a-version edits. It is the answer to "I
	// changed the declaration and nothing moved": compilation reads the published
	// snapshot, so a draft edit takes effect only after it is published and the
	// documents are compiled again.
	DraftPending bool                   `json:"draft_pending"`
	Payload      OntologyInsightPayload `json:"payload"`
}

// CompetencyQuestion is one declarative capability of the model.
type CompetencyQuestion struct {
	ID       string
	Question string
	Elements []CompetencyElement
}

// CompetencyElement is one parsed `kind:name` clause.
type CompetencyElement struct {
	Kind     string
	Name     string
	Required bool
}

// parseCompetencyQuestions reads the model's questions.
//
// The shape is one field per question inside the `competency` section:
//
//	competency:
//	  fields:
//	    - type: CQ-1
//	      question: 未结合同的供应商是谁？
//	      elements: class:contract | relation:contract->vendor | ?class:person
//
// Elements are a compact string rather than a nested array because the template
// config is a flat field list, and a trailing `?` is the whole optional syntax.
// The `type` key is what makes the version diff match questions by name instead
// of by index, so a question inserted in the middle does not rewrite the list.
func parseCompetencyQuestions(cfg map[string]interface{}) []CompetencyQuestion {
	section := jsonMapAt(cfg, "competency")
	fields := jsonListAt(section, "fields")
	out := make([]CompetencyQuestion, 0, len(fields))
	for _, raw := range fields {
		field, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id := strings.TrimSpace(jsonStringAt(field, "type"))
		question := strings.TrimSpace(jsonStringAt(field, "question"))
		if id == "" && question == "" {
			continue
		}
		if id == "" {
			id = question
		}
		out = append(out, CompetencyQuestion{
			ID:       id,
			Question: question,
			Elements: parseCompetencyElements(jsonStringAt(field, "elements")),
		})
	}
	return out
}

// parseCompetencyElements splits the element clause. Unknown kinds are dropped
// rather than reported: a typo in the model should not make every question look
// unanswerable, and the element list is visible on screen for the author to fix.
//
// Both `|` and a newline separate elements: the editor's field is a textarea, so
// one element per line is the natural way to write a long question, and treating
// that newline as part of the element name would silently drop every element
// after the first.
func parseCompetencyElements(raw string) []CompetencyElement {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '|' || r == '\n' || r == '\r'
	})
	out := make([]CompetencyElement, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		required := true
		if strings.HasPrefix(part, "?") {
			required = false
			part = strings.TrimSpace(part[1:])
		}
		kind, name, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		kind = strings.ToLower(strings.TrimSpace(kind))
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		switch kind {
		case competencyClass, competencyProperty, competencyRelation:
		default:
			continue
		}
		out = append(out, CompetencyElement{Kind: kind, Name: name, Required: required})
	}
	return out
}

// buildOntologyDrift folds the measured graph into the drift ledger.
func buildOntologyDrift(graph *OntologyGraph) OntologyDrift {
	drift := OntologyDrift{
		UndeclaredClasses: make([]OntologyDriftClass, 0),
		UnusedClasses:     make([]OntologyDriftClass, 0),
		UnusedProperties:  make([]OntologyDriftProperty, 0),
		Classes:           make([]OntologyDriftClass, 0, len(graph.Classes)),
		TotalEntities:     graph.TotalEntities,
		TotalRelations:    graph.TotalRelations,
		DroppedRelations:  graph.Quality.DroppedRelations,
		UntypedEntities:   graph.Quality.UntypedEntities,
		Truncated:         graph.CountsTruncated,
	}
	for _, class := range graph.Classes {
		node := OntologyDriftClass{
			Type:        class.Type,
			Label:       class.Label,
			Description: class.Description,
			Entities:    class.Entities,
			Declared:    class.Declared,
			Fields:      buildDriftFields(class),
		}
		switch {
		case !class.Declared && class.Entities > 0:
			drift.UndeclaredClasses = append(drift.UndeclaredClasses, node)
		case class.Declared && class.Entities == 0:
			drift.UnusedClasses = append(drift.UnusedClasses, node)
		}
		if class.Declared {
			drift.Classes = append(drift.Classes, node)
		}
	}
	for _, property := range graph.Properties {
		if !property.Declared || property.Relations > 0 {
			continue
		}
		drift.UnusedProperties = append(drift.UnusedProperties, OntologyDriftProperty{
			Type:   property.Type,
			Label:  property.Label,
			Source: property.Source,
			Target: property.Target,
		})
	}
	sort.Slice(drift.UndeclaredClasses, func(i, j int) bool {
		return drift.UndeclaredClasses[i].Entities > drift.UndeclaredClasses[j].Entities
	})
	return drift
}

// buildDriftFields turns a class's attributes into fill rates.
func buildDriftFields(class OntologyClassNode) []OntologyDriftField {
	if len(class.Attributes) == 0 {
		return nil
	}
	out := make([]OntologyDriftField, 0, len(class.Attributes))
	for _, attribute := range class.Attributes {
		rate := float64(0)
		if class.Entities > 0 {
			rate = float64(attribute.Assertions) / float64(class.Entities)
		}
		out = append(out, OntologyDriftField{
			Type:          attribute.Type,
			Assertions:    attribute.Assertions,
			Instances:     class.Entities,
			FillRate:      rate,
			Required:      attribute.Required,
			Missing:       attribute.Missing,
			InheritedFrom: attribute.InheritedFrom,
		})
	}
	return out
}

// buildCompetencyCoverage checks every question against the measured graph.
//
// The check is deliberately about the corpus and not only about the
// declaration: a class that is declared and has zero instances answers nothing,
// and reporting it as satisfied would make the coverage number a restatement of
// the template instead of a measurement of it.
func buildCompetencyCoverage(questions []CompetencyQuestion, graph *OntologyGraph, declared declaredOntology) ([]CompetencyQuestionCoverage, int, int) {
	classes := make(map[string]OntologyClassNode, len(graph.Classes))
	for _, class := range graph.Classes {
		classes[class.Type] = class
	}
	// Keyed by "source->target" rather than by a struct: the same spelling the
	// model uses in a question, so one reader can check both sides by eye.
	edges := make(map[string]OntologyPropertyEdge, len(graph.Properties))
	for _, property := range graph.Properties {
		key := property.Source + "->" + property.Target
		if _, ok := edges[key]; !ok {
			edges[key] = property
		}
	}

	out := make([]CompetencyQuestionCoverage, 0, len(questions))
	totalRequired, totalSatisfied := 0, 0
	for _, question := range questions {
		coverage := CompetencyQuestionCoverage{
			ID:       question.ID,
			Question: question.Question,
			Elements: make([]CompetencyElementResult, 0, len(question.Elements)),
		}
		for _, element := range question.Elements {
			result := checkCompetencyElement(element, classes, edges, declared)
			coverage.Elements = append(coverage.Elements, result)
			if !element.Required {
				continue
			}
			coverage.Required++
			totalRequired++
			if result.Satisfied {
				coverage.Satisfied++
				totalSatisfied++
			}
		}
		// A question that declares no required element is not answerable: there is
		// nothing to satisfy, so 0 == 0 would report a promise the model has not
		// made. The elements are derived from the question text by the editor
		// (competency.ts: traceCompetencyElements), so an empty list means the
		// derivation never ran — a finding, not a pass.
		coverage.Answerable = coverage.Required > 0 && coverage.Required == coverage.Satisfied
		out = append(out, coverage)
	}
	return out, totalRequired, totalSatisfied
}

// checkCompetencyElement resolves one `kind:name` clause against the graph.
//
// `declared` is the model as authored (the working config), while the graph is
// the model the stored rows were compiled under. They differ while a draft edit
// is unpublished, and the difference is the advice the reader needs: an element
// the draft declares but the rows never produced is "declared, no rows" — go
// recompile — not "not declared", which would send the author back to the canvas
// to add what is already there.
func checkCompetencyElement(
	element CompetencyElement,
	classes map[string]OntologyClassNode,
	edges map[string]OntologyPropertyEdge,
	declared declaredOntology,
) CompetencyElementResult {
	result := CompetencyElementResult{
		Kind:     element.Kind,
		Name:     element.Name,
		Required: element.Required,
	}
	switch element.Kind {
	case competencyClass:
		class, ok := classes[element.Name]
		if !ok {
			if _, isDeclared := declared.classes[element.Name]; isDeclared {
				result.Declared = true
				result.Reason = "not_extracted"
				return result
			}
			result.Reason = "undeclared"
			return result
		}
		result.Declared = true
		result.Observed = class.Entities
		result.Satisfied = class.Entities > 0
		if !result.Satisfied {
			result.Reason = "not_extracted"
		}
		return result
	case competencyProperty:
		owner, name, ok := strings.Cut(element.Name, ".")
		if !ok {
			result.Reason = "undeclared"
			return result
		}
		owner = strings.TrimSpace(owner)
		name = strings.TrimSpace(name)
		if class, found := classes[owner]; found {
			for _, attribute := range class.Attributes {
				if attribute.Type != name {
					continue
				}
				result.Declared = true
				result.Observed = attribute.Assertions
				result.Satisfied = attribute.Assertions > 0
				if !result.Satisfied {
					result.Reason = "not_extracted"
				}
				return result
			}
		}
		if property, isDeclared := declared.props[name]; isDeclared && containsStr(property.domains, owner) {
			result.Declared = true
			result.Reason = "not_extracted"
			return result
		}
		result.Reason = "undeclared"
		return result
	case competencyRelation:
		source, target, ok := strings.Cut(element.Name, "->")
		if !ok {
			result.Reason = "undeclared"
			return result
		}
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if edge, found := edges[source+"->"+target]; found {
			result.Declared = true
			result.Observed = edge.Relations
			result.Satisfied = edge.Relations > 0
			if !result.Satisfied {
				result.Reason = "not_extracted"
			}
			return result
		}
		if competencyElementDeclared(element, declared) {
			result.Declared = true
			result.Reason = "not_extracted"
			return result
		}
		result.Reason = "undeclared"
		return result
	}
	result.Reason = "undeclared"
	return result
}

// compiledTemplateConfig returns the config the corpus was actually extracted
// against.
//
// A published model compiles from its immutable snapshot, not from the working
// config (newKnowledgeCompilerTemplateResolver does the same resolution). The
// measurement has to read the same one: comparing rows produced under v5 against
// a draft that has since deleted a class would report every one of that class's
// rows as "in the corpus, not declared", and the author would go looking for a
// drift that is only their own unsaved edit.
func compiledTemplateConfig(ctx context.Context, tenantID string, template *entity.CompilationTemplate) (map[string]interface{}, error) {
	config := map[string]interface{}(template.Config)
	if template.Version <= 0 {
		// Nothing has ever been published: the template still compiles from the
		// config it carries, so that is the config to measure against.
		return config, nil
	}
	snapshot, err := dao.NewCompilationTemplateVersionDAO().
		GetByVersion(ctx, dao.DB, tenantID, template.ID, template.Version)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}(snapshot.Config), nil
}

// templateDraftPending reports edits that have not been published as a version.
//
// It is read on every measurement and on every open, so the panel can say "this
// edit is not live yet" instead of letting an author conclude that changing the
// declaration does nothing.
func templateDraftPending(ctx context.Context, tenantID string, template *entity.CompilationTemplate) bool {
	if template == nil || template.Version <= 0 {
		return false
	}
	snapshot, err := dao.NewCompilationTemplateVersionDAO().
		GetByVersion(ctx, dao.DB, tenantID, template.ID, template.Version)
	if err != nil {
		return false
	}
	working, errWorking := json.Marshal(map[string]interface{}(template.Config))
	published, errPublished := json.Marshal(map[string]interface{}(snapshot.Config))
	if errWorking != nil || errPublished != nil {
		return false
	}
	return !bytes.Equal(working, published)
}

// RefreshInsight measures one dataset against one model and stores the result.
//
// This is the only path that scans, and it is explicit: the workspace reads the
// stored measurement and offers a refresh button, so opening a tab never costs
// a full dataset scan.
func (s *DatasetArtifactService) RefreshInsight(ctx context.Context, tenantID, datasetID, templateID string) (*OntologyInsightView, error) {
	templateID = strings.TrimSpace(templateID)
	datasetID = strings.TrimSpace(datasetID)
	if tenantID == "" || datasetID == "" || templateID == "" {
		return nil, errors.New("tenant, dataset and template are all required")
	}
	templateDAO := dao.NewCompilationTemplateDAO()
	template, err := templateDAO.GetTemplate(ctx, dao.DB, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	cfg, err := compiledTemplateConfig(ctx, tenantID, template)
	if err != nil {
		return nil, err
	}
	filter := datasetBucketFilter(string(template.Kind), templateID)
	graph, err := s.buildOntologyGraph(ctx, tenantID, datasetID, filter, cfg)
	if err != nil {
		return nil, err
	}

	payload := OntologyInsightPayload{Drift: buildOntologyDrift(graph)}
	// Questions come from the working config while the graph stays on the
	// published snapshot: a requirement is a statement about the model as
	// authored, and the publish gate judges exactly that (it reads
	// template.Config). Measuring the snapshot instead would make every new
	// question unmeasurable until the version that contains it is published —
	// and publishing is what the coverage number is needed for.
	working := map[string]interface{}(template.Config)
	payload.Questions, payload.Required, payload.Satisfied = buildCompetencyCoverage(
		parseCompetencyQuestions(working), graph, parseDeclaredOntology(working),
	)

	stored, err := jsonMapOf(payload)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	insight := &entity.OntologyInsight{
		ID:         common.GenerateUUID(),
		TenantID:   tenantID,
		TemplateID: templateID,
		DatasetID:  datasetID,
		Version:    template.Version,
		AsOf:       now,
		Payload:    stored,
	}
	if err := dao.NewOntologyInsightDAO().Upsert(ctx, dao.DB, insight); err != nil {
		return nil, err
	}
	return &OntologyInsightView{
		TemplateID:   templateID,
		DatasetID:    datasetID,
		Version:      template.Version,
		AsOf:         now,
		DraftPending: templateDraftPending(ctx, tenantID, template),
		Payload:      payload,
	}, nil
}

// GetInsight returns the stored measurement. A missing one is not an error the
// caller has to translate: it is the state before the first compile, and the
// workspace shows "not measured yet" from it.
func (s *DatasetArtifactService) GetInsight(ctx context.Context, tenantID, datasetID, templateID string) (*OntologyInsightView, error) {
	insight, err := dao.NewOntologyInsightDAO().Get(ctx, dao.DB, tenantID, templateID, datasetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	payload, err := decodeInsightPayload(insight.Payload)
	if err != nil {
		return nil, err
	}
	current, err := dao.NewCompilationTemplateDAO().GetTemplate(ctx, dao.DB, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	return &OntologyInsightView{
		TemplateID: insight.TemplateID,
		DatasetID:  insight.DatasetID,
		Version:    insight.Version,
		AsOf:       insight.AsOf,
		// A retired or unpublished model still has its old measurements; only a
		// newer version makes them describe a different model than the one on
		// screen.
		Stale:        current.Version != insight.Version,
		DraftPending: templateDraftPending(ctx, tenantID, current),
		Payload:      payload,
	}, nil
}

// jsonMapOf round-trips a typed payload through JSON into the column shape. It
// is the one place the storage representation is decided, so a new field on the
// payload needs no schema change.
func jsonMapOf(value interface{}) (entity.JSONMap, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	out := entity.JSONMap{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decodeInsightPayload rebuilds the typed payload from its column shape.
func decodeInsightPayload(stored entity.JSONMap) (OntologyInsightPayload, error) {
	var payload OntologyInsightPayload
	if len(stored) == 0 {
		return payload, nil
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return payload, err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

// appendMeasuredCoverageIssues adds the requirement-authorised findings to a
// publish check from the stored measurements.
//
// It is separate from modelPublishCheck because it is the only part of the gate
// that depends on the corpus: the declaration checks run on the template alone
// and must stay answerable before anything is compiled. When nothing has been
// measured yet the gate says so instead of guessing — a model published against
// three datasets has to be checked against those three, and "no data" is a
// finding, not a pass.
func appendMeasuredCoverageIssues(ctx context.Context, db *gorm.DB, tenantID string, check *ModelPublishCheck, config map[string]interface{}) error {
	questions := parseCompetencyQuestions(config)
	if len(questions) == 0 {
		return nil
	}
	// A question with no required element changes nothing about the gate: the
	// denominator is empty, so no coverage number can move. That is the one
	// thing a declared question must not be able to do, and it is always a
	// tracing failure rather than a modelling decision (the editor derives the
	// elements from the question text). Reported before the measurement loop so
	// an unmeasured model also says it.
	if untraced := questionsWithoutRequiredElements(questions); len(untraced) > 0 {
		check.Warnings = append(check.Warnings, ModelPublishIssue{
			Code:     "cq_without_elements",
			Category: ModelIssueCategoryRequirement,
			Severity: ModelIssueWarning,
			Message:  "competency questions declare no required element, so nothing measures whether the model can answer them",
			Subjects: untraced,
		})
	}
	insights, err := dao.NewOntologyInsightDAO().ListByTemplate(ctx, db, tenantID, check.TemplateID)
	if err != nil {
		return err
	}
	if len(insights) == 0 {
		check.Warnings = append(check.Warnings, ModelPublishIssue{
			Code:     "cq_unmeasured",
			Category: ModelIssueCategoryRequirement,
			Severity: ModelIssueWarning,
			Message:  "competency questions are declared but no dataset has been measured against this model yet",
		})
		return nil
	}
	for _, insight := range insights {
		payload, err := decodeInsightPayload(insight.Payload)
		if err != nil {
			return err
		}
		if payload.Required == 0 {
			continue
		}
		coverage := payload.Coverage()
		severity := ""
		switch {
		case coverage < CompetencyBlockerCoverage:
			severity = ModelIssueBlocker
		case coverage < CompetencyWarningCoverage:
			severity = ModelIssueWarning
		default:
			continue
		}
		check.Blockers, check.Warnings = appendIssue(check, ModelPublishIssue{
			Code:     "cq_coverage_low",
			Category: ModelIssueCategoryRequirement,
			Severity: severity,
			Message:  "the model answers " + coveragePercent(coverage) + " of its required capability elements in this dataset",
			Subjects: unansweredQuestionIDs(payload.Questions),
		})
	}
	return nil
}

// appendIssue files one finding under its severity, keeping the two lists the
// only place severity is read.
func appendIssue(check *ModelPublishCheck, issue ModelPublishIssue) ([]ModelPublishIssue, []ModelPublishIssue) {
	if issue.Severity == ModelIssueBlocker {
		return append(check.Blockers, issue), check.Warnings
	}
	return check.Blockers, append(check.Warnings, issue)
}

// questionsWithoutRequiredElements lists the questions whose element list
// carries no required clause. They are the ones a coverage number cannot
// measure, so the finding names them instead of a ratio.
func questionsWithoutRequiredElements(questions []CompetencyQuestion) []string {
	out := make([]string, 0, len(questions))
	for _, question := range questions {
		hasRequired := false
		for _, element := range question.Elements {
			if element.Required {
				hasRequired = true
				break
			}
		}
		if !hasRequired {
			out = append(out, question.ID)
		}
	}
	return out
}

// unansweredQuestionIDs lists the questions a measurement could not answer, so
// the finding points at the questions rather than at the dataset.
func unansweredQuestionIDs(questions []CompetencyQuestionCoverage) []string {
	out := make([]string, 0, len(questions))
	for _, question := range questions {
		if !question.Answerable {
			out = append(out, question.ID)
		}
	}
	return out
}

// coveragePercent renders a ratio for a finding message. It is fixed at whole
// percents because the message is read, not computed with.
func coveragePercent(ratio float64) string {
	if ratio <= 0 {
		return "0%"
	}
	if ratio >= 1 {
		return "100%"
	}
	return strconv.Itoa(int(ratio*100+0.5)) + "%"
}

// ModelIssueCategoryRequirement groups findings that come from the requirement
// authorisation, next to the declaration categories.
const ModelIssueCategoryRequirement = "requirement"

// competencyIssueCodeUnresolved is the declaration-only finding: a question
// requires an element the model never declares, so no compile can ever satisfy
// it.
const competencyIssueCodeUnresolved = "cq_unresolved_element"

// buildCompetencyDeclarationIssues reports required elements the model does not
// declare. It reads the template alone, which is why it belongs with the other
// declaration checks.
func buildCompetencyDeclarationIssues(config map[string]interface{}) []ModelPublishIssue {
	declared := parseDeclaredOntology(config)
	questions := parseCompetencyQuestions(config)
	issues := make([]ModelPublishIssue, 0)
	for _, question := range questions {
		for _, element := range question.Elements {
			if !element.Required {
				continue
			}
			if competencyElementDeclared(element, declared) {
				continue
			}
			issues = append(issues, ModelPublishIssue{
				Code:     competencyIssueCodeUnresolved,
				Category: ModelIssueCategoryRequirement,
				Severity: ModelIssueBlocker,
				Message:  "competency question " + question.ID + " requires " + element.Kind + " " + element.Name + ", which the model does not declare",
				Subjects: []string{question.ID, element.Name},
			})
		}
	}
	return issues
}

// competencyElementDeclared reports whether the declaration names the element.
// It does not look at the corpus: that is the measured half's job.
func competencyElementDeclared(element CompetencyElement, declared declaredOntology) bool {
	switch element.Kind {
	case competencyClass:
		_, ok := declared.classes[element.Name]
		return ok
	case competencyProperty:
		owner, name, found := strings.Cut(element.Name, ".")
		if !found {
			return false
		}
		property, ok := declared.props[strings.TrimSpace(name)]
		return ok && containsStr(property.domains, strings.TrimSpace(owner))
	case competencyRelation:
		source, target, found := strings.Cut(element.Name, "->")
		if !found {
			return false
		}
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		for _, property := range declared.props {
			if property.isDatatype() {
				continue
			}
			if containsStr(property.domains, source) && containsStr(property.ranges, target) {
				return true
			}
		}
		return false
	}
	return false
}
