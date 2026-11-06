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
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/mindmap"
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
	"ragflow/internal/ingestion/component/knowledge_compiler/tree"
	"ragflow/internal/ingestion/component/knowledge_compiler/wiki"
	"ragflow/internal/ingestion/knowledge_compile"
)

// The trial's bounds. A trial compiles real documents with an LLM, so every
// dimension is capped: the point is to show the direction a model change moves
// the extracted knowledge, and a sample says that far more cheaply than the
// corpus does.
const (
	// modelTrialDefaultDatasets and modelTrialDefaultDocuments are what a
	// trial runs when the caller asks for nothing in particular.
	modelTrialDefaultDatasets  = 1
	modelTrialDefaultDocuments = 2
	// modelTrialMaxDocuments caps the whole trial, not each dataset: a caller
	// cannot turn a click into an unbounded bill.
	modelTrialMaxDocuments = 6
	// modelTrialChunkLimit caps the source text one document contributes, so a
	// single huge document cannot dominate the run's cost.
	modelTrialChunkLimit = 400
	// modelTrialChunkPageSize is the engine page size while collecting chunks.
	modelTrialChunkPageSize = 1000
)

// ModelTrialAssertion is one piece of knowledge a compile run produced.
//
// Identity, not storage: a product carries engine ids, embeddings and source
// chunk pointers, all of which differ between two runs of the same input, so
// comparing them would report every row as changed. What a reviewer needs is
// whether the *knowledge* is different.
type ModelTrialAssertion struct {
	// Kind is the product kind: entity, relation, or the producing variant's
	// own kind for pages and summaries.
	Kind string `json:"kind"`
	// Type is the declared type — an entity type, a relation's property.
	Type string `json:"type,omitempty"`
	// Name identifies the assertion: an entity name, or "from -> to" for a
	// relation.
	Name string `json:"name"`
	// Key is the stable identity the two runs are compared on.
	Key string `json:"key"`
}

// ModelTrialDocument is one document's trial result.
type ModelTrialDocument struct {
	DatasetID    string `json:"dataset_id"`
	DatasetName  string `json:"dataset_name"`
	DocumentID   string `json:"document_id"`
	DocumentName string `json:"document_name"`
	// Chunks is how much source text went into the run.
	Chunks int `json:"chunks"`
	// Added and Removed are the assertions the draft would gain and lose
	// against what the corpus holds now.
	Added   []ModelTrialAssertion `json:"added"`
	Removed []ModelTrialAssertion `json:"removed"`
	// Unchanged is how many assertions both sides agree on, so a reader can
	// tell "small diff, small document" from "small diff, agreement".
	Unchanged int `json:"unchanged"`
	// Error reports why this document produced no comparison. It is per
	// document on purpose: one unparseable document must not throw away the
	// evidence the others produced.
	Error string `json:"error,omitempty"`
}

// ModelTrial is the pre-publish rehearsal: the draft model run over real
// documents, compared with what the corpus holds under the published one.
//
// It is deliberately separate from ModelPublishCheck. The check is free,
// automatic and about the model's self-consistency; a trial costs an LLM pass
// per document and is about the world, which is why it is an action somebody
// chooses rather than a step the publish button takes for them.
type ModelTrial struct {
	TemplateID string `json:"template_id"`
	Kind       string `json:"kind"`
	// CurrentVersion is the version the corpus was compiled with, and
	// DraftVersion the one a publish would create.
	CurrentVersion int                   `json:"current_version"`
	DraftVersion   int                   `json:"draft_version"`
	Documents      []*ModelTrialDocument `json:"documents"`
	Added          int                   `json:"added"`
	Removed        int                   `json:"removed"`
	// Runs is how many compiles the trial actually performed, so the cost of
	// the answer is visible in it.
	Runs int `json:"runs"`
	// Truncated is true when a cap cut the sample short: the trial then
	// describes a sample, not the corpus.
	Truncated bool `json:"truncated"`
	// Notes explain what was sampled and what was left out.
	Notes []string `json:"notes"`
}

// ModelTrialOptions bounds a trial. Zero values take the defaults.
type ModelTrialOptions struct {
	Datasets  int
	Documents int
}

// Trial runs the draft model over sample documents of the datasets that
// compile against it and reports what would change.
//
// The baseline is what the corpus holds — the compiled rows already stored for
// those documents — not a second run of the published config. That is both
// cheaper (one LLM pass instead of two) and truer to the question being asked:
// a reviewer is deciding what publishing changes in the knowledge base, and the
// knowledge base is what it changes.
func (s *CompilationTemplateGovernanceService) Trial(ctx context.Context, db *gorm.DB, tenantID, templateID string, opts ModelTrialOptions) (*ModelTrial, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	if template.Version <= 0 {
		return nil, fmt.Errorf("model %s has never been published, so a trial has nothing to compare against; its first publish has no baseline", templateID)
	}
	variant, err := kccommon.KindToVariant(template.Kind)
	if err != nil {
		return nil, err
	}

	trial := &ModelTrial{
		TemplateID:     template.ID,
		Kind:           template.Kind,
		CurrentVersion: template.Version,
		DraftVersion:   template.Version + 1,
		Documents:      make([]*ModelTrialDocument, 0),
		Notes:          make([]string, 0),
	}

	datasets, err := s.datasetsForTrial(ctx, db, tenantID, template, opts.Datasets)
	if err != nil {
		return nil, err
	}
	if len(datasets) == 0 {
		trial.Notes = append(trial.Notes, "no dataset compiles against this model, so there is nothing to run the draft over")
		return trial, nil
	}

	documents := modelTrialDocuments(opts)
	sampled := 0
	for _, dataset := range datasets {
		kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, dataset.ID)
		if err != nil {
			trial.Notes = append(trial.Notes, fmt.Sprintf("dataset %s could not be read: %v", dataset.Name, err))
			continue
		}
		docs, _, err := dao.NewDocumentDAO().GetByKBID(ctx, db, kb.ID)
		if err != nil {
			trial.Notes = append(trial.Notes, fmt.Sprintf("documents of dataset %s could not be listed: %v", dataset.Name, err))
			continue
		}
		if len(docs) > documents {
			trial.Truncated = true
			trial.Notes = append(trial.Notes, fmt.Sprintf("dataset %s has %d documents; %d were sampled", dataset.Name, len(docs), documents))
		}
		for _, doc := range docs {
			if sampled >= modelTrialMaxDocuments {
				trial.Truncated = true
				break
			}
			sampled++
			result := runModelTrialDocument(ctx, db, tenantID, template, variant, kb, doc)
			trial.Documents = append(trial.Documents, result)
			trial.Added += len(result.Added)
			trial.Removed += len(result.Removed)
			if result.Error == "" {
				trial.Runs++
			}
		}
		if sampled >= modelTrialMaxDocuments {
			break
		}
	}
	trial.Notes = append(trial.Notes,
		fmt.Sprintf("the draft was compiled over %d document(s) with %d chunk(s) of source text at most each", trial.Runs, modelTrialChunkLimit),
		"a trial compiles a sample, not the corpus: publishing does not recompile anything by itself")
	return trial, nil
}

// datasetsForTrial picks the datasets the draft is tried on.
func (s *CompilationTemplateGovernanceService) datasetsForTrial(ctx context.Context, db *gorm.DB, tenantID string, template *entity.CompilationTemplate, limit int) ([]*dao.ModelImpactDataset, error) {
	if limit <= 0 {
		limit = modelTrialDefaultDatasets
	}
	all, err := dao.NewDatasetModelRefDAO().DatasetsByModel(ctx, db, tenantID, modelRefOf(template))
	if err != nil {
		return nil, err
	}
	if len(all) > limit {
		return all[:limit], nil
	}
	return all, nil
}

func modelTrialDocuments(opts ModelTrialOptions) int {
	if opts.Documents <= 0 {
		return modelTrialDefaultDocuments
	}
	return opts.Documents
}

// runModelTrialDocument compiles one document with the draft model and compares
// the result with the compiled rows the corpus already holds for it.
func runModelTrialDocument(ctx context.Context, db *gorm.DB, tenantID string, template *entity.CompilationTemplate, variant kccommon.Variant, kb *entity.Knowledgebase, doc *entity.Document) *ModelTrialDocument {
	result := &ModelTrialDocument{
		DatasetID:    kb.ID,
		DatasetName:  kb.Name,
		DocumentID:   doc.ID,
		DocumentName: stringOrEmpty(doc.Name),
		Added:        make([]ModelTrialAssertion, 0),
		Removed:      make([]ModelTrialAssertion, 0),
	}
	chunks, err := loadTrialChunks(ctx, tenantID, kb.ID, doc.ID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if len(chunks) == 0 {
		result.Error = "this document has no parsed content to compile; it has to be parsed first"
		return result
	}
	result.Chunks = len(chunks)

	nodeParams := compilerNodeParams(kb.ParserConfig)
	param, err := trialParam(template, variant, nodeParams)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	deps, err := kccommon.ResolveDeps(ctx, tenantID, param.LLMID, param.EmbeddingModel)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	deps.TenantID = tenantID
	deps.DatasetID = kb.ID

	outputs, err := runTrialVariant(ctx, variant, deps, param, kccommon.Inputs{
		DocID: doc.ID,
		// The variants read the template config from this key; the compiler
		// component delivers it the same way, so the trial and a real compile
		// see the same model.
		VariantSpecific: map[string]any{"parser_config": template.Config},
		Chunks:          chunks,
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}

	baseline, err := knowledge_compile.NewEngineReader(engine.Get()).LoadDocProducts(ctx, tenantID, kb.ID, doc.ID)
	if err != nil {
		result.Error = fmt.Sprintf("the compiled rows of this document could not be read: %v", err)
		return result
	}

	result.Added, result.Removed, result.Unchanged = diffTrialAssertions(
		trialAssertions(baseline), trialAssertions(outputs.Products),
	)
	return result
}

// runTrialVariant dispatches one compile run to the variant the model's kind
// selects. It mirrors the component's dispatch, minus everything that only
// exists to serve a canvas run.
func runTrialVariant(ctx context.Context, variant kccommon.Variant, deps kccommon.Deps, param kccommon.Param, inputs kccommon.Inputs) (kccommon.Outputs, error) {
	switch variant {
	case kccommon.VariantStructure:
		return structure.Run(ctx, deps, param, inputs)
	case kccommon.VariantWiki:
		return wiki.Run(ctx, deps, param, inputs)
	case kccommon.VariantTree:
		return tree.Run(ctx, deps, param, inputs)
	case kccommon.VariantMindmap:
		return mindmap.Run(ctx, deps, param, inputs)
	default:
		return kccommon.Outputs{}, fmt.Errorf("compilation model trial: unsupported variant %q", variant)
	}
}

// trialParam builds the parameters the compiler component would pass to the
// variant for this template.
//
// The scalar parameters come from the dataset's compiler node, because that is
// where the corpus's own runs read the chat model and embedding from: a trial
// that picked its own would compare two different systems and report the
// difference as the model change.
func trialParam(template *entity.CompilationTemplate, variant kccommon.Variant, nodeParams map[string]any) (kccommon.Param, error) {
	param := kccommon.Param{}.Defaults()
	if len(nodeParams) > 0 {
		parsed, err := kccommon.ParseParam(nodeParams)
		if err != nil {
			return param, fmt.Errorf("the dataset's compilation parameters could not be read: %w", err)
		}
		param.LLMID = parsed.LLMID
		param.EmbeddingModel = parsed.EmbeddingModel
		param.Language = parsed.Language
		param.SimilarityThreshold = parsed.SimilarityThreshold
		param.EnableHistoricalDedup = parsed.EnableHistoricalDedup
		param.Plan = parsed.Plan
		param.Extra = parsed.Extra
	}
	param.Variant = variant
	param.TemplateID = template.ID
	param.TemplateConfig = template.Config
	return param, nil
}

// compilerNodeParams finds the compilation node parameters inside a dataset
// config.
//
// The walk mirrors the compiler's own search: the parameters sit at the dataset
// level or nested inside a canvas component node, and the config is the only
// place that says which chat model and embedding the corpus compiles with.
func compilerNodeParams(config interface{}) map[string]any {
	switch typed := config.(type) {
	case entity.JSONMap:
		return compilerNodeParams(map[string]interface{}(typed))
	case map[string]interface{}:
		if namesModel(typed) {
			return typed
		}
		for _, child := range typed {
			if found := compilerNodeParams(child); found != nil {
				return found
			}
		}
	case []interface{}:
		for _, child := range typed {
			if found := compilerNodeParams(child); found != nil {
				return found
			}
		}
	}
	return nil
}

func namesModel(params map[string]interface{}) bool {
	for _, key := range []string{"compilation_template_group_id", "compilation_template_group_ids", "compilation_template_id", "compilation_template_ids"} {
		if _, ok := params[key]; ok {
			return true
		}
	}
	return false
}

// loadTrialChunks reads the parsed source text of one document.
//
// It reads the document engine rather than the ingestion pipeline: the trial
// has to run outside a pipeline, and the parsed chunks are exactly what a
// pipeline would have compiled. Compiled rows are skipped — compiling a
// document's own output back into it would compare the model with itself.
func loadTrialChunks(ctx context.Context, tenantID, datasetID, docID string) ([]kccommon.Chunk, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return nil, fmt.Errorf("the document engine is unavailable")
	}
	chunks := make([]kccommon.Chunk, 0, modelTrialChunkLimit)
	for offset := 0; len(chunks) < modelTrialChunkLimit; offset += modelTrialChunkPageSize {
		result, err := docEngine.Search(ctx, &enginetypes.SearchRequest{
			IndexNames: []string{fmt.Sprintf("ragflow_%s", tenantID)},
			KbIDs:      []string{datasetID},
			Offset:     offset,
			Limit:      modelTrialChunkPageSize,
			SelectFields: []string{
				"id", "doc_id", "content_with_weight", "available_int", "compile_kwd",
			},
			Filter: map[string]any{
				"doc_id":        []string{docID},
				"available_int": 1,
			},
		})
		if err != nil {
			return nil, err
		}
		if result == nil || len(result.Chunks) == 0 {
			break
		}
		for _, row := range result.Chunks {
			if strings.TrimSpace(trialString(row["compile_kwd"])) != "" {
				continue
			}
			text := strings.TrimSpace(trialString(row["content_with_weight"]))
			if text == "" {
				continue
			}
			chunks = append(chunks, kccommon.Chunk{ID: trialString(row["id"]), Text: text})
			if len(chunks) >= modelTrialChunkLimit {
				break
			}
		}
		if int64(offset+len(result.Chunks)) >= result.Total {
			break
		}
	}
	return chunks, nil
}

// trialAssertions reduces a compile run to the knowledge it produced.
//
// Container rows (a graph or a page holding the others) are left out: their
// content is a rendering of the assertions below them, so counting them would
// double every change and make a small diff look large.
func trialAssertions(products []kccommon.Product) []ModelTrialAssertion {
	seen := make(map[string]struct{}, len(products))
	out := make([]ModelTrialAssertion, 0, len(products))
	for _, product := range products {
		assertion, ok := trialAssertionOf(product)
		if !ok {
			continue
		}
		if _, duplicate := seen[assertion.Key]; duplicate {
			continue
		}
		seen[assertion.Key] = struct{}{}
		out = append(out, assertion)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func trialAssertionOf(product kccommon.Product) (ModelTrialAssertion, bool) {
	kind := trialString(product.Meta["kind"])
	switch kind {
	case "entity":
		name := trialString(product.Meta["name"])
		if name == "" {
			return ModelTrialAssertion{}, false
		}
		entityType := trialString(product.Meta["entity_type"])
		return ModelTrialAssertion{
			Kind: kind,
			Type: entityType,
			Name: name,
			Key:  strings.Join([]string{kind, entityType, name}, "|"),
		}, true
	case "relation":
		from := trialString(product.Meta["from"])
		to := trialString(product.Meta["to"])
		if from == "" || to == "" {
			return ModelTrialAssertion{}, false
		}
		propertyType := trialString(product.Meta["relation_type"])
		return ModelTrialAssertion{
			Kind: kind,
			Type: propertyType,
			Name: from + " -> " + to,
			Key:  strings.Join([]string{kind, propertyType, from, to}, "|"),
		}, true
	}
	// Pages, summaries and any other variant product: named by whatever label
	// the variant gave it.
	if kind == "" || kind == "graph" {
		return ModelTrialAssertion{}, false
	}
	name := trialString(product.Meta["title"])
	if name == "" {
		name = trialString(product.Meta["name"])
	}
	if name == "" {
		name = trialString(product.Meta["slug"])
	}
	if name == "" {
		return ModelTrialAssertion{}, false
	}
	pageType := trialString(product.Meta["page_type"])
	return ModelTrialAssertion{
		Kind: kind,
		Type: pageType,
		Name: name,
		Key:  strings.Join([]string{kind, pageType, name}, "|"),
	}, true
}

// diffTrialAssertions reports what the draft adds and drops, and how much the
// two runs agree on.
func diffTrialAssertions(current, draft []ModelTrialAssertion) (added, removed []ModelTrialAssertion, unchanged int) {
	currentByKey := make(map[string]struct{}, len(current))
	for _, assertion := range current {
		currentByKey[assertion.Key] = struct{}{}
	}
	draftByKey := make(map[string]struct{}, len(draft))
	for _, assertion := range draft {
		draftByKey[assertion.Key] = struct{}{}
	}
	added = make([]ModelTrialAssertion, 0)
	removed = make([]ModelTrialAssertion, 0)
	for _, assertion := range draft {
		if _, ok := currentByKey[assertion.Key]; ok {
			unchanged++
			continue
		}
		added = append(added, assertion)
	}
	for _, assertion := range current {
		if _, ok := draftByKey[assertion.Key]; ok {
			continue
		}
		removed = append(removed, assertion)
	}
	return added, removed, unchanged
}

func trialString(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
