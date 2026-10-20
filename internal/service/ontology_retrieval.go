package service

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
	"ragflow/internal/service/nlp"
)

// OntologyABoxRetrievalRequest asks for the compiled (ABox) rows that fit the
// structure a published model declares.
//
// It is a separate entry point on purpose. The main retrieval pipeline answers
// "what does the corpus say about this question"; this one answers "which
// assertions does the declared structure say exist, for this scope" — and other
// retrievals (the agentic loop among them) call it rather than growing the main
// pipeline another set of flags.
type OntologyABoxRetrievalRequest struct {
	// UserID is the caller, used for the dataset access check.
	UserID string
	// DatasetIDs is the knowledge scope, required: compiled rows carry kb_id, so
	// a search without a dataset would sweep the tenant. It is a different set
	// from the conversation's document datasets, which is what makes the
	// knowledge layer its own governance boundary.
	DatasetIDs []string
	// DocumentIDs narrows the search to those documents when set.
	DocumentIDs []string
	// Question is what the dense leg ranks against. Required.
	Question string
	// OntologyTemplateID names the published model whose declared hierarchy
	// expands the classes below and whose declarations the digest reports.
	OntologyTemplateID string
	// OntologyVersion pins the snapshot to read. Zero means the model's current
	// published version, or the working config when it has never been published.
	OntologyVersion int
	// The constraints, all optional, all explicit. Nothing is parsed out of the
	// question by a model: a structure derived from prose would narrow the search
	// to whatever the reader happened to say. When none are given, the
	// candidates come from OntologyQueryCandidates — a deterministic match
	// against the declared vocabulary that the caller sees in the filter echo.
	EntityTypes []string
	Properties  []string
	FromTypes   []string
	ToTypes     []string
	// TopK bounds the rows returned. Defaults to 10.
	TopK int
	// SimilarityThreshold is the dense-leg floor. Defaults to 0.2.
	SimilarityThreshold float64
	// DocumentScope is the conversation's readable document set, used to count
	// resolvable citations. Nil means unrestricted; a non-nil empty slice means
	// no document is readable.
	DocumentScope []string
	// SkipUnreviewed drops pending assertions. The default keeps them, tagged:
	// a model whose latest compile is unreviewed would otherwise be invisible
	// until a human got to it.
	SkipUnreviewed bool
	// HypergraphRecall adds the compiled hyperedge layer to the digest: two
	// hypernode recall legs, greedy selection of the factual blocks those slots
	// belong to, the original chunks behind them, and the coverage they explain.
	// Off by default: it is a measurable addition, and the assertion-only path
	// has to stay exactly as it was for the two to be compared.
	HypergraphRecall bool
}

// OntologyABoxDegrade* are the machine-readable reasons the returned rows are
// not narrowed by the declared structure.
//
// They exist because the reasons call for different next actions and the UI
// says different things about them: a model that could not be read is a model
// to fix, and a model that matched nothing is a model whose coverage is
// incomplete. A bare message would force the caller to match on prose.
const (
	// OntologyABoxDegradeNoModel means the declared structure could not be read
	// at all: a missing model, a foreign tenant's model, a kind that is not an
	// ontology, or an unreadable snapshot.
	OntologyABoxDegradeNoModel = "no_model"
	// OntologyABoxDegradeNoMatch means the structure was read and applied and
	// matched no assertion, so the rows come from asking the same question
	// without the narrowing.
	OntologyABoxDegradeNoMatch = "no_match"
)

// OntologyABoxRetrievalResult carries the rows together with the filter that was
// actually applied. The filter is echoed because the answer to "why did I get
// these rows" is the expansion: a caller who asked for `agent` searched
// `agent`, `person` and `organization`, and has to be able to see that.
type OntologyABoxRetrievalResult struct {
	Chunks []map[string]interface{}
	Total  int64
	// Filter is the narrowing the returned rows were searched with, and is nil
	// when none was applied. It is never a filter that was merely attempted:
	// echoing an unapplied filter would make the one field a caller reads to
	// explain the result the one field that lies.
	Filter map[string]interface{}
	// StructureApplied reports whether the returned rows were narrowed by the
	// declared structure. False means they are the unfiltered search: a model
	// that cannot be read must not take retrieval down with it, so the rows still
	// come back — but the caller has to be able to see which of the two it got.
	StructureApplied bool
	// DegradeCode is one of the OntologyABoxDegrade* values when StructureApplied
	// is false, and empty when it is true.
	DegradeCode string
	// DegradeReason is the human-readable form of DegradeCode.
	DegradeReason string
	// Model names the version the declared structure was read from.
	Model OntologyModelRef
	// SuggestedConstraints is the narrowing derived from the question when the
	// caller supplied none. It is echoed so the match is arguable: a reader who
	// sees the wrong class matched can say so, which a hidden derivation would
	// not allow.
	SuggestedConstraints OntologyStructureConstraints
	// Digest is the knowledge layer's output for an agent loop: the assertions,
	// the vocabulary, the coverage, the citation counts and the receipt.
	Digest *agentic_rag.OntologyDigest
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

// ontologyStructureLoader reads a published model's declared structure.
type ontologyStructureLoader func(ctx context.Context, tenantID, templateID string, version int) (declaredOntology, OntologyModelRef, error)

// ontologyCoverageScanner samples the knowledge scope's compiled rows to find
// which declared classes and properties it exercises. It is injectable because
// it needs a database, and the rest of the flow does not.
type ontologyCoverageScanner func(ctx context.Context, tenantID string, datasetIDs []string, limit int) (ontologyCoverageSample, error)

// ontologyCoverageSample is one bounded read of the knowledge scope's type
// columns.
type ontologyCoverageSample struct {
	Classes    map[string]bool
	Properties map[string]bool
	Sampled    int
	Truncated  bool
}

// ontologyCoverageSampleLimit bounds the coverage scan per dataset. The engines
// expose no aggregation, so "which declared classes have instances" is answered
// by reading a bounded page and saying that it was bounded. A scan that claims
// completeness it cannot have would turn "not sampled" into "not present".
const ontologyCoverageSampleLimit = 300

// OntologyABoxRetrievalService is the standalone ABox retrieval API.
type OntologyABoxRetrievalService struct {
	scopeResolver ontologyABoxScopeResolver
	searcher      ontologyABoxSearcher
	// structureLoader and coverageScanner are fields so a test can exercise the
	// flow without a database: they are the two steps that need one.
	structureLoader ontologyStructureLoader
	coverageScanner ontologyCoverageScanner
	// rowReader and chunkReader back the hypergraph section: a block by id, the
	// blocks an entity's name belongs to, and the original text behind a block.
	// They are seams for the same reason as the two above — the section's logic
	// (selection, assembly, coverage) is what has to be testable without a
	// document engine.
	rowReader   ontologyRowReader
	chunkReader ontologyChunkReader
}

// NewOntologyABoxRetrievalService wires the production boundaries: the dataset
// records for scope and embedding model, the model table for the declared
// hierarchy, the ordinary retrieval engine for the search itself, and the
// tenant index for the coverage scan.
func NewOntologyABoxRetrievalService(docEngine engine.DocEngine, documentDAO *dao.DocumentDAO) *OntologyABoxRetrievalService {
	return &OntologyABoxRetrievalService{
		scopeResolver:   &kbScopeResolver{kbDAO: dao.NewKnowledgebaseDAO()},
		searcher:        nlp.NewRetrievalService(docEngine, documentDAO),
		structureLoader: loadOntologyTemplate,
		coverageScanner: scanOntologyCoverage,
		rowReader:       readOntologyRows,
		chunkReader:     fetchChunksByIDs,
	}
}

// Retrieve answers with the compiled rows that fit the requested structure.
//
// The remaining refusals are deliberate: a request for a dataset the caller does
// not own must not read its rows. A structure that cannot be read is deliberately
// NOT a refusal — the rows come back unnarrowed with the degrade code that says
// so, because a model nobody has written yet must not make the corpus
// unsearchable.
//
// The rows come back in two shapes at once, and both are needed: Chunks is what
// the REST caller renders, and Digest is what an agent loop consumes. They are
// built from the same page so the two can never disagree about what was found.
func (s *OntologyABoxRetrievalService) Retrieve(ctx context.Context, req *OntologyABoxRetrievalRequest) (*OntologyABoxRetrievalResult, error) {
	if s == nil || s.scopeResolver == nil || s.searcher == nil || s.structureLoader == nil {
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

	scope, err := s.scopeResolver.Resolve(ctx, req.UserID, req.DatasetIDs)
	if err != nil {
		return nil, err
	}
	if len(scope.TenantIDs) == 0 {
		return nil, fmt.Errorf("no tenant owns the given datasets")
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}
	similarity := req.SimilarityThreshold
	if similarity <= 0 {
		similarity = 0.2
	}

	out := &OntologyABoxRetrievalResult{}
	tenantID := scope.TenantIDs[0]

	// The declared structure is read even when the caller gave no constraints:
	// the digest's vocabulary, its coverage and its model identity all come from
	// the declarations, and a knowledge lookup that reported none of them would
	// leave the reader unable to tell which model answered.
	declared, ref, modelErr := s.structureLoader(ctx, tenantID, req.OntologyTemplateID, req.OntologyVersion)
	out.Model = ref

	constraints := OntologyStructureConstraints{
		EntityTypes: req.EntityTypes,
		Properties:  req.Properties,
		FromTypes:   req.FromTypes,
		ToTypes:     req.ToTypes,
	}
	suggested := OntologyStructureConstraints{}
	if constraints.IsZero() && modelErr == nil {
		// The TBox half of query interpretation: which declared classes and
		// properties this question's own words name. It is a candidate set, and
		// an empty one means "search without narrowing" rather than "search for
		// nothing".
		suggested = OntologyQueryCandidates(req.Question, declared,
			OntologyQueryMaxCandidateClasses, OntologyQueryMaxCandidateProperties)
		constraints = suggested
	}
	out.SuggestedConstraints = suggested

	// Every knowledge-layer search is pinned to compiled assertions. The
	// knowledge scope may also hold navigation clusters, wiki pages and build
	// markers, and none of them is a fact the model states.
	base := OntologyAssertionKindFilter()
	filter := base
	var degrade *agentic_rag.OntologyDegrade
	structureApplied := false
	switch {
	case modelErr != nil:
		// Unavailable model, answerable question. Searching unfiltered by
		// structure keeps the corpus reachable, and the degrade code says the
		// rows are unnarrowed.
		out.DegradeCode = OntologyABoxDegradeNoModel
		out.DegradeReason = modelErr.Error()
		degrade = &agentic_rag.OntologyDegrade{Code: OntologyABoxDegradeNoModel, Reason: modelErr.Error()}
	case !constraints.IsZero():
		structureApplied = true
		filter = mergeOntologyFilters(base, OntologyStructureFilter(declared, constraints))
	default:
		// Declarations read, but the question named nothing they declare. The
		// rows are still the knowledge scope's assertions; only the narrowing is
		// absent, and the echo says so.
		structureApplied = false
	}
	out.Filter = filterIfNarrowed(base, filter)
	out.StructureApplied = structureApplied

	if req.SkipUnreviewed {
		filter = withOntologyFilter(filter, "must_not", map[string]interface{}{
			"review_status_kwd": []string{string(AssertionReviewUnreviewed)},
		})
	}

	page, err := s.searchOntologyRows(ctx, scope, req, filter, topK, similarity)
	if err != nil {
		return nil, err
	}
	applyOntologyABoxPage(out, page)
	if structureApplied && out.Total == 0 {
		// The declared structure matched nothing. Re-asking without it keeps the
		// question answerable, and the code is the signal that the model's
		// coverage — not the corpus — is what came up short.
		structureApplied = false
		out.StructureApplied = false
		out.Filter = filterIfNarrowed(base, base)
		out.DegradeCode = OntologyABoxDegradeNoMatch
		out.DegradeReason = "the declared structure matched no assertion; the rows are the search without narrowing"
		degrade = &agentic_rag.OntologyDegrade{Code: OntologyABoxDegradeNoMatch, Reason: out.DegradeReason}
		filter = base
		if req.SkipUnreviewed {
			filter = withOntologyFilter(filter, "must_not", map[string]interface{}{
				"review_status_kwd": []string{string(AssertionReviewUnreviewed)},
			})
		}
		page, err = s.searchOntologyRows(ctx, scope, req, filter, topK, similarity)
		if err != nil {
			return nil, err
		}
		applyOntologyABoxPage(out, page)
	}
	if out.Chunks == nil {
		out.Chunks = []map[string]interface{}{}
	}

	digest := buildOntologyDigest(out.Chunks, out.Total, declared, ref, out.Filter, out.StructureApplied, degrade, req.DocumentScope)
	// The hypergraph section is assembled from the same page the digest was
	// built from, so the two can never describe different row sets. It is
	// attached after the digest because it also *narrows* it: an assertion whose
	// endpoints both appear inside one assembled block is no longer a separate
	// fact, and the counts that follow are recomputed over what is left.
	if hg := s.buildOntologyHypergraph(ctx, scope, req, out.Chunks); hg != nil {
		applyOntologyHypergraph(digest, hg, req.DocumentScope)
	}
	if digest.Degrade == nil && len(digest.Assertions) > 0 &&
		digest.Citations.Resolvable == 0 && digest.Citations.Unresolvable > 0 {
		// The assertions are real, but none of their sources is readable in this
		// conversation. Saying so is what lets the caller use the wording as a
		// search hint while refusing to present the quotation as evidence.
		digest.Degrade = &agentic_rag.OntologyDegrade{
			Code: agentic_rag.OntologyDegradeCitationsUnresolvable,
			Reason: "every assertion's source document is outside this conversation's readable scope, " +
				"so their quotations cannot be cited",
		}
	}
	if coverage, covErr := s.scanCoverage(ctx, tenantID, req.DatasetIDs, declared); covErr != nil {
		// A coverage scan that fails is reported as unknown rather than as
		// empty: "we could not sample" and "nothing is there" are different
		// facts, and only one of them is about the corpus.
		digest.Degrade = firstDegrade(digest.Degrade, &agentic_rag.OntologyDegrade{
			Code:   agentic_rag.OntologyDegradeCoverageUnknown,
			Reason: "the knowledge scope could not be sampled, so which declared classes have instances is unknown",
		})
	} else {
		digest.Coverage = coverage
	}
	out.Digest = digest
	return out, nil
}

// searchOntologyRows runs one knowledge-layer search. It is the only place the
// request is built, so a retry cannot drift from the first attempt in any field
// other than the filter.
func (s *OntologyABoxRetrievalService) searchOntologyRows(
	ctx context.Context,
	scope ontologyABoxScope,
	req *OntologyABoxRetrievalRequest,
	filter map[string]interface{},
	topK int,
	similarity float64,
) (*nlp.RetrievalResult, error) {
	// The assertion projection is explicit: the main pipeline's narrow
	// projection is its own choice, and a digest that cannot see
	// source_chunk_ids or evidence cannot cite anything.
	return s.searcher.Retrieval(ctx, &nlp.RetrievalRequest{
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
		SelectFields:          ontologyAssertionRowFields,
	})
}

// scanCoverage samples the knowledge scope and turns the sample into the
// declared-structure coverage the digest reports.
func (s *OntologyABoxRetrievalService) scanCoverage(ctx context.Context, tenantID string, datasetIDs []string, declared declaredOntology) (agentic_rag.OntologyCoverage, error) {
	if s.coverageScanner == nil {
		return agentic_rag.OntologyCoverage{}, fmt.Errorf("coverage scan is not wired")
	}
	sample, err := s.coverageScanner(ctx, tenantID, datasetIDs, ontologyCoverageSampleLimit)
	if err != nil {
		return agentic_rag.OntologyCoverage{}, err
	}
	return ontologyCoverage(declared, sample.Classes, sample.Properties, sample.Sampled, ontologyCoverageSampleLimit, sample.Truncated), nil
}

// firstDegrade keeps the earlier of two degrade reasons: the search-level one
// explains the rows, and a coverage caveat must not overwrite it.
func firstDegrade(existing, fallback *agentic_rag.OntologyDegrade) *agentic_rag.OntologyDegrade {
	if existing != nil {
		return existing
	}
	return fallback
}

// scanOntologyCoverage reads a bounded page of the knowledge scope's entity and
// assertion rows per dataset and collects their class and property columns.
func scanOntologyCoverage(ctx context.Context, tenantID string, datasetIDs []string, limit int) (ontologyCoverageSample, error) {
	sample := ontologyCoverageSample{Classes: map[string]bool{}, Properties: map[string]bool{}}
	if limit <= 0 {
		limit = ontologyCoverageSampleLimit
	}
	for _, datasetID := range datasetIDs {
		rows, total, err := graphRowSearch(ctx, tenantID, datasetID, ontologyCoverageRowFields,
			OntologyAssertionKindFilter(), nil, 0, limit, nil)
		if err != nil {
			return sample, err
		}
		sample.Sampled += len(rows)
		if total > int64(len(rows)) {
			sample.Truncated = true
		}
		for _, row := range rows {
			kind := ontologyRowKind(row)
			switch kind {
			case "entity":
				if name := strings.TrimSpace(firstStringValue(row["entity_type_kwd"])); name != "" {
					sample.Classes[name] = true
				}
			case "relation":
				if name := strings.TrimSpace(firstStringValue(row["prop_kwd"])); name != "" {
					sample.Properties[name] = true
				}
			}
		}
	}
	return sample, nil
}

// mergeOntologyFilters returns the kind pin together with the structure filter.
// The structure filter already pins the kinds it implies; the pin is applied
// first so a structure filter that names no kind (an empty constraint set) still
// stays inside compiled assertions.
func mergeOntologyFilters(base, structure map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(structure))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range structure {
		out[k] = v
	}
	return out
}

// withOntologyFilter adds one condition without mutating the caller's map: two
// searches in one call share the input filter, and mutating it would make the
// retry differ from the first attempt in a way nothing logs.
func withOntologyFilter(filter map[string]interface{}, key string, value interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(filter)+1)
	for k, v := range filter {
		out[k] = v
	}
	out[key] = value
	return out
}

// filterIfNarrowed reports the filter that narrowed the search, and nil when the
// only condition was the kind pin. The pin is not a narrowing of the question —
// it is what "the compiled rows" means — and echoing it as one would make every
// knowledge lookup look like it applied a structure.
func filterIfNarrowed(base, applied map[string]interface{}) map[string]interface{} {
	if len(applied) == 0 {
		return nil
	}
	extra := make(map[string]interface{}, len(applied))
	for k, v := range applied {
		if _, isBase := base[k]; isBase {
			continue
		}
		extra[k] = v
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

// applyOntologyABoxPage copies an engine page onto the result, treating a nil
// page as an empty one so the caller always reads a slice.
func applyOntologyABoxPage(out *OntologyABoxRetrievalResult, page *nlp.RetrievalResult) {
	if page == nil {
		out.Chunks, out.Total = nil, 0
		return
	}
	out.Chunks, out.Total = page.Chunks, page.Total
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
