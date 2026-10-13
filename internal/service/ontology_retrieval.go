package service

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
	"ragflow/internal/service/nlp"
)

// OntologyABoxRetrievalRequest asks for the compiled (ABox) rows that fit the
// structure a template declares.
//
// It is a separate entry point on purpose. The main retrieval pipeline answers
// "what does the corpus say about this question"; this one answers "which
// assertions does the declared structure say exist, for this scope" — and other
// retrievals (the agentic loop among them) call it rather than growing the main
// pipeline another set of flags.
type OntologyABoxRetrievalRequest struct {
	// UserID is the caller, used for the dataset ownership check.
	UserID string
	// DatasetIDs is the scope, required: compiled rows carry kb_id, so a search
	// without a dataset would sweep the tenant.
	DatasetIDs []string
	// DocumentIDs narrows the scope to those documents when set.
	DocumentIDs []string
	// Question is what the dense leg ranks against. Required.
	Question string
	// OntologyTemplateID names the template whose declared hierarchy expands the
	// classes below. Required as soon as a constraint is given.
	OntologyTemplateID string
	// The constraints, all optional, all explicit. Nothing is parsed out of the
	// question: a structure derived from prose would narrow the search to
	// whatever the reader happened to say.
	EntityTypes []string
	Properties  []string
	FromTypes   []string
	ToTypes     []string
	// TopK bounds the rows returned. Defaults to 10.
	TopK int
	// SimilarityThreshold is the dense-leg floor. Defaults to 0.2.
	SimilarityThreshold float64
}

// OntologyABoxRetrievalResult carries the rows together with the filter that was
// actually applied. The filter is echoed because the answer to "why did I get
// these rows" is the expansion: a caller who asked for `agent` searched
// `agent`, `person` and `organization`, and has to be able to see that.
type OntologyABoxRetrievalResult struct {
	Chunks []map[string]interface{}
	Total  int64
	Filter map[string]interface{}
}

// ontologyABoxScope is what a request needs before it can search: the tenants
// whose indexes hold the rows, and the embedding model the question is embedded
// with. Resolving it is a boundary of its own (ownership + model configuration),
// which is why it is injectable.
type ontologyABoxScope struct {
	TenantIDs      []string
	EmbeddingModel *modelModule.EmbeddingModel
}

type ontologyABoxScopeResolver interface {
	Resolve(ctx context.Context, userID string, datasetIDs []string) (ontologyABoxScope, error)
}

// ontologyABoxSearcher is the seam onto the search engine: hybrid search over the
// rows, once the caller knows the scope and the filter.
type ontologyABoxSearcher interface {
	Retrieval(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error)
}

// OntologyABoxRetrievalService is the standalone ABox retrieval API.
type OntologyABoxRetrievalService struct {
	scopeResolver ontologyABoxScopeResolver
	searcher      ontologyABoxSearcher
	// structureFilter is a field so a test can exercise the flow without a
	// database: reading the template is the one step that needs one.
	structureFilter func(ctx context.Context, tenantID, templateID string, c OntologyStructureConstraints) (map[string]interface{}, error)
}

// NewOntologyABoxRetrievalService wires the production boundaries: the dataset
// records for scope and embedding model, the template table for the declared
// hierarchy, and the ordinary retrieval engine for the search itself.
func NewOntologyABoxRetrievalService(docEngine engine.DocEngine, documentDAO *dao.DocumentDAO) *OntologyABoxRetrievalService {
	return &OntologyABoxRetrievalService{
		scopeResolver:   &kbScopeResolver{kbDAO: dao.NewKnowledgebaseDAO()},
		searcher:        nlp.NewRetrievalService(docEngine, documentDAO),
		structureFilter: StructureFilterForTemplate,
	}
}

// Retrieve answers with the compiled rows that fit the requested structure.
//
// Every refusal here is deliberate: a request without constraints would be the
// main pipeline's job, a request without a readable template cannot honour its
// narrowing, and a request for a dataset the caller does not own must not read
// its rows.
func (s *OntologyABoxRetrievalService) Retrieve(ctx context.Context, req *OntologyABoxRetrievalRequest) (*OntologyABoxRetrievalResult, error) {
	if s == nil || s.scopeResolver == nil || s.searcher == nil || s.structureFilter == nil {
		return nil, fmt.Errorf("ontology ABox retrieval is not wired")
	}
	if req == nil {
		return nil, fmt.Errorf("a request is required")
	}
	if strings.TrimSpace(req.UserID) == "" {
		return nil, fmt.Errorf("a caller is required to check dataset ownership")
	}
	if len(req.DatasetIDs) == 0 {
		return nil, fmt.Errorf("dataset_ids is required")
	}
	if strings.TrimSpace(req.Question) == "" {
		return nil, fmt.Errorf("question is required")
	}
	constraints := OntologyStructureConstraints{
		EntityTypes: req.EntityTypes,
		Properties:  req.Properties,
		FromTypes:   req.FromTypes,
		ToTypes:     req.ToTypes,
	}
	if constraints.IsZero() {
		// Without a constraint this endpoint is "search the compiled rows", which
		// the main retrieval already does. Refusing keeps one owner per question.
		return nil, fmt.Errorf("at least one structure constraint is required (entity_types, properties, from_types or to_types)")
	}

	scope, err := s.scopeResolver.Resolve(ctx, req.UserID, req.DatasetIDs)
	if err != nil {
		return nil, err
	}
	if len(scope.TenantIDs) == 0 {
		return nil, fmt.Errorf("no tenant owns the given datasets")
	}
	filter, err := s.structureFilter(ctx, scope.TenantIDs[0], req.OntologyTemplateID, constraints)
	if err != nil {
		return nil, err
	}
	if filter == nil {
		return nil, fmt.Errorf("the requested structure produced no filter")
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}
	similarity := req.SimilarityThreshold
	if similarity <= 0 {
		similarity = 0.2
	}
	result, err := s.searcher.Retrieval(ctx, &nlp.RetrievalRequest{
		TenantIDs:             scope.TenantIDs,
		Question:              req.Question,
		KbIDs:                 req.DatasetIDs,
		DocIDs:                req.DocumentIDs,
		Page:                  1,
		PageSize:              topK,
		RerankCandidatesCount: &topK,
		SimilarityThreshold:   &similarity,
		EmbeddingModel:        scope.EmbeddingModel,
		Filter:                filter,
	})
	if err != nil {
		return nil, err
	}
	out := &OntologyABoxRetrievalResult{Filter: filter}
	if result != nil {
		out.Chunks = result.Chunks
		out.Total = result.Total
	}
	if out.Chunks == nil {
		out.Chunks = []map[string]interface{}{}
	}
	return out, nil
}

// kbScopeResolver reads the datasets: their owners name the indexes to search,
// and their shared embedding model is what the question is embedded with. It
// mirrors the main pipeline's resolution so both paths search the same rows the
// same way.
type kbScopeResolver struct {
	kbDAO *dao.KnowledgebaseDAO
}

func (r *kbScopeResolver) Resolve(ctx context.Context, userID string, datasetIDs []string) (ontologyABoxScope, error) {
	scope := ontologyABoxScope{}
	if r == nil || r.kbDAO == nil {
		return scope, fmt.Errorf("dataset lookup is not wired")
	}
	seen := map[string]bool{}
	records := make([]*entity.Knowledgebase, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		if err := CheckDatasetAccess(ctx, permission.Subject{UserID: userID}, datasetID, permission.OperationUse); err != nil {
			return scope, fmt.Errorf("only the owner of dataset %s is authorized for this operation", datasetID)
		}
		kb, err := r.kbDAO.GetByID(ctx, dao.DB, datasetID)
		if err != nil || kb == nil {
			return scope, fmt.Errorf("dataset %s not found", datasetID)
		}
		if kb.TenantID != "" && !seen[kb.TenantID] {
			seen[kb.TenantID] = true
			scope.TenantIDs = append(scope.TenantIDs, kb.TenantID)
		}
		records = append(records, kb)
	}
	if len(records) == 0 {
		return scope, nil
	}
	if err := ValidateDatasetEmbeddingModels(ctx, dao.DB, records); err != nil {
		return scope, err
	}
	if embdID := records[0].EmbdID; embdID != "" {
		// Same factory the main retrieval uses, so both paths embed the question
		// with the model the dataset itself resolves to.
		model, err := NewModelFactory().NewEmbeddingModel(
			ctx, ModelAccess{TenantID: scope.TenantIDs[0]}, embdID,
		)
		if err != nil {
			return scope, fmt.Errorf("failed to resolve the embedding model for the ontology search: %w", err)
		}
		scope.EmbeddingModel = model
	}
	return scope, nil
}
