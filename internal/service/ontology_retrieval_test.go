package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/service/nlp"
)

// fakeOntologyScope stands in for the dataset records: the tenants whose indexes
// hold the rows and the embedding model the question is embedded with.
type fakeOntologyScope struct {
	scope ontologyABoxScope
	err   error

	userID     string
	datasetIDs []string
}

func (f *fakeOntologyScope) Resolve(_ context.Context, userID string, datasetIDs []string) (ontologyABoxScope, error) {
	f.userID, f.datasetIDs = userID, datasetIDs
	return f.scope, f.err
}

// fakeOntologyModel stands in for the published model: it records how it was
// asked for the declared structure, because the version pin and the tenant are
// part of the contract the search depends on.
type fakeOntologyModel struct {
	declared declaredOntology
	ref      OntologyModelRef
	err      error

	tenantID   string
	templateID string
	version    int
}

func (f *fakeOntologyModel) load(_ context.Context, tenantID, templateID string, version int) (declaredOntology, OntologyModelRef, error) {
	f.tenantID, f.templateID, f.version = tenantID, templateID, version
	if f.err != nil {
		return declaredOntology{}, f.ref, f.err
	}
	return f.declared, f.ref, nil
}

// fakeOntologyCoverage stands in for the bounded scan of the knowledge scope.
type fakeOntologyCoverage struct {
	sample ontologyCoverageSample
	err    error

	datasetIDs []string
	templateID string
	limit      int
}

func (f *fakeOntologyCoverage) scan(_ context.Context, _ string, datasetIDs []string, templateID string, limit int) (ontologyCoverageSample, error) {
	f.datasetIDs, f.templateID, f.limit = datasetIDs, templateID, limit
	if f.err != nil {
		return ontologyCoverageSample{}, f.err
	}
	return f.sample, nil
}

// fakeOntologySearcher records every call, because the fallback path asks the
// engine twice and the difference between the two requests IS the behaviour
// under test.
type fakeOntologySearcher struct {
	calls   []*nlp.RetrievalRequest
	results []*nlp.RetrievalResult
	err     error
}

func (f *fakeOntologySearcher) Retrieval(_ context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error) {
	index := len(f.calls)
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.results) == 0 {
		return nil, nil
	}
	if index >= len(f.results) {
		index = len(f.results) - 1
	}
	return f.results[index], nil
}

func (f *fakeOntologySearcher) callCount() int { return len(f.calls) }

type ontologyTestService struct {
	svc      *OntologyABoxRetrievalService
	searcher *fakeOntologySearcher
	model    *fakeOntologyModel
	scope    *fakeOntologyScope
}

// newOntologyTestService wires the four injectable boundaries. A nil model means
// "a published model that declares nothing this question mentions", which is the
// case the schema-free grade exists for; no results at all means an empty page.
func newOntologyTestService(
	scope ontologyABoxScope,
	model *fakeOntologyModel,
	coverage *fakeOntologyCoverage,
	results ...*nlp.RetrievalResult,
) *ontologyTestService {
	if model == nil {
		model = &fakeOntologyModel{ref: OntologyModelRef{ID: "tpl-1", Name: "Guild model", Version: 3, PublishStatus: "published"}}
	}
	if coverage == nil {
		coverage = &fakeOntologyCoverage{}
	}
	if len(results) == 0 {
		results = []*nlp.RetrievalResult{{Chunks: []map[string]interface{}{}, Total: 0}}
	}
	searcher := &fakeOntologySearcher{results: results}
	scopeResolver := &fakeOntologyScope{scope: scope}
	return &ontologyTestService{
		svc: &OntologyABoxRetrievalService{
			scopeResolver:   scopeResolver,
			searcher:        searcher,
			structureLoader: model.load,
			coverageScanner: coverage.scan,
		},
		searcher: searcher,
		model:    model,
		scope:    scopeResolver,
	}
}

// guildOntology is a published model with one abstract class, two declared
// subclasses and one object property. The hierarchy is what makes the expansion
// observable, and `leads` is a term the question below names.
func guildOntology() declaredOntology {
	return declaredOntology{
		classOrder: []string{"agent", "person", "organization"},
		classes: map[string]declaredClass{
			"agent":        {label: "agent"},
			"person":       {label: "person", parents: []string{"agent"}},
			"organization": {label: "organization", parents: []string{"agent"}},
		},
		propOrder: []string{"leads"},
		props: map[string]declaredProperty{
			"leads": {label: "leads", kind: "object", domains: []string{"person"}, ranges: []string{"organization"}},
		},
	}
}

// assertOntologyBaseFilter checks the two conditions every knowledge-layer
// search carries: the row-kind pin that makes "the compiled rows" mean
// assertions, and the scope pin that makes them THIS model's assertions. Neither
// narrows the question, so both are present even when the search is unnarrowed.
func assertOntologyBaseFilter(t *testing.T, filter map[string]interface{}, templateID string) {
	t.Helper()
	if filter["knowledge_graph_kwd"] == nil {
		t.Fatalf("filter = %#v, want the row-kind pin", filter)
	}
	ids, _ := filter["compilation_template_ids"].([]string)
	if len(ids) != 1 || ids[0] != templateID {
		t.Fatalf("filter = %#v, want the scope pin %q", filter, templateID)
	}
	if len(filter) != 2 {
		t.Fatalf("filter = %#v, want exactly the row-kind pin and the scope pin", filter)
	}
}

// Rows compiled by another model are excluded from the search by the scope pin,
// and the sample reports how many were excluded. Silent exclusion would look
// exactly like a model with thin coverage, and the reader would go hunting for
// missing compilations instead of a mis-scoped dataset.
func TestOntologyABoxRetrievalReportsForeignModelRows(t *testing.T) {
	coverage := &fakeOntologyCoverage{sample: ontologyCoverageSample{
		Classes:    map[string]bool{"agent": true},
		Properties: map[string]bool{"leads": true},
		Templates:  map[string]int{"tpl-1": 7, "tpl-2": 4},
		Sampled:    11,
	}}
	svc := newOntologyTestService(guildScope(), nil, coverage,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	result, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if coverage.templateID != "tpl-1" {
		t.Fatalf("coverage scanned with model %q, want the bound model", coverage.templateID)
	}
	if result.ScopeSampledRows != 11 {
		t.Fatalf("sampled rows = %d, want 11", result.ScopeSampledRows)
	}
	if result.ScopeForeignTemplates["tpl-2"] != 4 {
		t.Fatalf("foreign = %#v, want tpl-2 counted", result.ScopeForeignTemplates)
	}
	if _, sawSelf := result.ScopeForeignTemplates["tpl-1"]; sawSelf {
		t.Fatalf("foreign = %#v, want the bound model excluded from its own foreign list", result.ScopeForeignTemplates)
	}
}

// A caller that never bound a model gets no scope pin: pinning to the empty id
// would match nothing and turn a working search into an empty one.
func TestOntologyScopeFilterIsEmptyWithoutAModel(t *testing.T) {
	if got := OntologyScopeFilter("  "); got != nil {
		t.Fatalf("filter = %#v, want none", got)
	}
	got := OntologyScopeFilter("tpl-9")
	ids, _ := got["compilation_template_ids"].([]string)
	if len(ids) != 1 || ids[0] != "tpl-9" {
		t.Fatalf("filter = %#v, want the model pinned", got)
	}
}

func validOntologyABoxRequest() *OntologyABoxRetrievalRequest {
	return &OntologyABoxRetrievalRequest{
		UserID:             "user-1",
		DatasetIDs:         []string{"kb-1"},
		Question:           "who leads the guild",
		OntologyTemplateID: "tpl-1",
	}
}

func guildScope() ontologyABoxScope {
	return ontologyABoxScope{TenantIDs: []string{"tenant-1"}}
}

func entityRow() map[string]interface{} {
	return map[string]interface{}{
		"type_kwd":        "entity",
		"entity_type_kwd": "person",
		"name_kwd":        "ada",
	}
}

// Retrieving the compiled rows without a structural constraint is a complete
// request, not a misuse of the endpoint: the prefetch and the direct-answer mode
// both need "what does the knowledge scope say about this question". The rows
// are graded schema_free, because unnarrowed rows are not the model's statement
// about the question and must not read as one.
func TestOntologyABoxRetrievalSearchesWithoutAConstraint(t *testing.T) {
	svc := newOntologyTestService(guildScope(), nil, nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	result, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if svc.searcher.callCount() != 1 {
		t.Fatalf("engine calls = %d, want the unnarrowed search issued once", svc.searcher.callCount())
	}
	issued := svc.searcher.calls[0]
	assertOntologyBaseFilter(t, issued.Filter, validOntologyABoxRequest().OntologyTemplateID)
	if result.StructureApplied {
		t.Fatalf("result = %#v, want no narrowing applied", result)
	}
	if result.Filter != nil {
		t.Fatalf("filter = %#v, want nil so the echo never shows an unapplied filter", result.Filter)
	}
	if result.DegradeCode != "" {
		t.Fatalf("degrade = %q, want none: nothing was attempted and dropped", result.DegradeCode)
	}
	if result.Digest.EvidenceLevel != agentic_rag.OntologyEvidenceSchemaFree {
		t.Fatalf("evidence level = %q, want %q", result.Digest.EvidenceLevel, agentic_rag.OntologyEvidenceSchemaFree)
	}
	if len(result.Chunks) != 1 || result.Total != 1 {
		t.Fatalf("result = %#v, want the engine rows passed through", result)
	}
	// The receipt is what a person reads, and it must not make the reader infer
	// the grade from the absence of a filter.
	if receipt := OntologyDigestReceipt(result.Digest); receipt["evidence_level"] != agentic_rag.OntologyEvidenceSchemaFree {
		t.Fatalf("receipt = %#v, want the grade carried into the receipt", receipt["evidence_level"])
	}
	if len(issued.KbIDs) != 1 || issued.KbIDs[0] != "kb-1" {
		t.Fatalf("kb ids = %v, want the requested scope", issued.KbIDs)
	}
	if issued.PageSize != 10 {
		t.Fatalf("page size = %d, want the default top_k", issued.PageSize)
	}
	if svc.scope.userID != "user-1" || svc.scope.datasetIDs[0] != "kb-1" {
		t.Fatalf("scope resolution = %q/%v, want the caller and the requested datasets", svc.scope.userID, svc.scope.datasetIDs)
	}
	if svc.model.tenantID != "tenant-1" || svc.model.templateID != "tpl-1" || svc.model.version != 0 {
		t.Fatalf("model read = %q/%q/v%d, want the tenant, the model and the published version",
			svc.model.tenantID, svc.model.templateID, svc.model.version)
	}
	if result.Model.ID != "tpl-1" || result.Model.Version != 3 {
		t.Fatalf("model = %#v, want the version that answered echoed back", result.Model)
	}
}

// The TBox half of query interpretation: when the caller states no constraint,
// the question's own words are matched against the declared vocabulary. The
// match is a candidate set, so it is echoed — a reader who sees the wrong class
// matched has to be able to say so.
func TestOntologyABoxRetrievalDerivesCandidateConstraintsFromTheQuestion(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1", Version: 3}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	result, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if got := result.SuggestedConstraints.Properties; len(got) != 1 || got[0] != "leads" {
		t.Fatalf("suggested = %#v, want the declared property the question names", result.SuggestedConstraints)
	}
	if got := svc.searcher.calls[0].Filter["prop_kwd"]; got == nil {
		t.Fatalf("filter = %#v, want the derived property applied", svc.searcher.calls[0].Filter)
	}
	if !result.StructureApplied || result.Digest.EvidenceLevel != agentic_rag.OntologyEvidenceStructured {
		t.Fatalf("result = %#v, want a structured retrieval", result)
	}
	if result.Filter["prop_kwd"] == nil || result.Filter["knowledge_graph_kwd"] != nil {
		t.Fatalf("filter echo = %#v, want only the narrowing", result.Filter)
	}
}

// A stated class is expanded along the declared hierarchy: a reader who asks for
// `agent` means the instances typed `person` and `organization` too, and the
// expansion is what the caller reads back to see why these rows.
func TestOntologyABoxRetrievalAppliesAndEchoesTheExpandedStructure(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1", Name: "Guild model", Version: 3}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	req := validOntologyABoxRequest()
	req.EntityTypes = []string{"agent"}
	req.TopK = 5
	result, err := svc.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if svc.searcher.callCount() != 1 {
		t.Fatalf("engine calls = %d, want the applied structure searched once", svc.searcher.callCount())
	}
	issued := svc.searcher.calls[0]
	classes, _ := issued.Filter["entity_type_kwd"].([]string)
	for _, want := range []string{"agent", "person", "organization"} {
		if !containsStringValue(classes, want) {
			t.Fatalf("entity_type_kwd = %v, want %s among the expanded classes", classes, want)
		}
	}
	if kinds, _ := issued.Filter["knowledge_graph_kwd"].([]string); len(kinds) != 1 || kinds[0] != "entity" {
		t.Fatalf("kind pin = %v, want the constraint's own kind", kinds)
	}
	if issued.PageSize != 5 {
		t.Fatalf("page size = %d, want the requested top_k", issued.PageSize)
	}
	if !result.StructureApplied || result.DegradeCode != "" {
		t.Fatalf("result = %#v, want an applied structure with no degrade code", result)
	}
	if result.Digest.EvidenceLevel != agentic_rag.OntologyEvidenceStructured {
		t.Fatalf("evidence level = %q, want %q", result.Digest.EvidenceLevel, agentic_rag.OntologyEvidenceStructured)
	}
	if result.Filter["entity_type_kwd"] == nil || result.Filter["knowledge_graph_kwd"] != nil {
		t.Fatalf("filter echo = %#v, want the expansion and not the kind pin", result.Filter)
	}
	if result.SuggestedConstraints.EntityTypes != nil {
		t.Fatalf("suggested = %#v, want none: the caller stated the constraint", result.SuggestedConstraints)
	}
}

// The projection has to be the graph readers' own contract plus the kind
// columns. A narrower one silently empties fields the digest reads — the row
// still comes back, just without the column that explains it.
func TestOntologyABoxRetrievalProjectsTheRowsTheDigestReads(t *testing.T) {
	svc := newOntologyTestService(guildScope(), nil, nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	if _, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest()); err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	projected := svc.searcher.calls[0].SelectFields
	for _, want := range []string{
		"content_with_weight", "attr", "source_chunk_ids", "source_doc_ids", "doc_ids_kwd",
		"name_kwd", "entity_type_kwd", "prop_kwd", "from_entity_kwd", "to_entity_kwd",
		"review_status_kwd", "compilation_template_ids", "type_kwd", "knowledge_graph_kwd",
	} {
		if !containsStringValue(projected, want) {
			t.Fatalf("projection = %v, want %s among the requested columns", projected, want)
		}
	}
}

// A structure that matches nothing is answered from the same question asked
// without it, and the grade says the rows are not the narrowed ones — that is
// the signal the model's coverage, not the corpus, came up short.
func TestOntologyABoxRetrievalGradesSchemaFreeWhenTheStructureMatchesNothing(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1"}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	req := validOntologyABoxRequest()
	req.EntityTypes = []string{"agent"}
	result, err := svc.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if svc.searcher.callCount() != 2 {
		t.Fatalf("engine calls = %d, want the narrowing and then the retry", svc.searcher.callCount())
	}
	if svc.searcher.calls[0].Filter["entity_type_kwd"] == nil {
		t.Fatal("the first attempt must carry the narrowing")
	}
	assertOntologyBaseFilter(t, svc.searcher.calls[1].Filter, req.OntologyTemplateID)
	if result.StructureApplied || result.DegradeCode != OntologyABoxDegradeNoMatch {
		t.Fatalf("result = %#v, want a no_match degrade", result)
	}
	if result.Digest.EvidenceLevel != agentic_rag.OntologyEvidenceSchemaFree {
		t.Fatalf("evidence level = %q, want %q", result.Digest.EvidenceLevel, agentic_rag.OntologyEvidenceSchemaFree)
	}
	if result.Total != 1 || len(result.Chunks) != 1 {
		t.Fatalf("result = %#v, want the fallback rows", result)
	}
	if result.Filter != nil {
		t.Fatalf("result filter = %#v, want nil for rows that were not narrowed", result.Filter)
	}
}

// The retry is a single second attempt, not a loop: an empty corpus stays empty,
// and the grade that comes back is none rather than a promise of evidence.
func TestOntologyABoxRetrievalDoesNotRetryTwice(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1"}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0})

	req := validOntologyABoxRequest()
	req.EntityTypes = []string{"agent"}
	result, err := svc.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if svc.searcher.callCount() != 2 {
		t.Fatalf("engine calls = %d, want exactly one retry", svc.searcher.callCount())
	}
	if result.DegradeCode != OntologyABoxDegradeNoMatch {
		t.Fatalf("degrade code = %q, want the no_match code on an empty corpus", result.DegradeCode)
	}
	if result.Digest.EvidenceLevel != agentic_rag.OntologyEvidenceNone {
		t.Fatalf("evidence level = %q, want %q on an empty scope", result.Digest.EvidenceLevel, agentic_rag.OntologyEvidenceNone)
	}
	if result.Chunks == nil {
		t.Fatal("chunks must be an empty slice, never nil, for a caller that iterates it")
	}
}

// A model that cannot be read must not make the corpus unsearchable: the rows
// still come back, unnarrowed, graded schema_free, with the code that says which
// half is missing.
func TestOntologyABoxRetrievalDegradesWhenTheModelCannotBeRead(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{err: errors.New("ontology model \"tpl-1\" not found"), ref: OntologyModelRef{ID: "tpl-1"}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	req := validOntologyABoxRequest()
	req.EntityTypes = []string{"agent"}
	result, err := svc.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if svc.searcher.callCount() != 1 {
		t.Fatalf("engine calls = %d, want the unfiltered search still issued", svc.searcher.callCount())
	}
	assertOntologyBaseFilter(t, svc.searcher.calls[0].Filter, req.OntologyTemplateID)
	if result.StructureApplied || result.DegradeCode != OntologyABoxDegradeNoModel {
		t.Fatalf("result = %#v, want a no_model degrade", result)
	}
	if result.DegradeReason == "" {
		t.Fatal("degrade reason must be reported, not swallowed")
	}
	if result.Digest.EvidenceLevel != agentic_rag.OntologyEvidenceSchemaFree {
		t.Fatalf("evidence level = %q, want %q", result.Digest.EvidenceLevel, agentic_rag.OntologyEvidenceSchemaFree)
	}
	if result.Filter != nil {
		t.Fatalf("result filter = %#v, want nil so the echo never shows an unapplied filter", result.Filter)
	}
	if len(result.Chunks) != 1 {
		t.Fatalf("chunks = %#v, want the unnarrowed rows", result.Chunks)
	}
}

// The engine pages are projected into the digest the agent reads: the row's role
// decides whether it is an assertion at all, the quotation comes out of the
// payload the writer stored, and the datatype values come out of the attr column.
func TestOntologyABoxRetrievalProjectsRowsIntoTheDigest(t *testing.T) {
	rows := []map[string]interface{}{
		// A navigation row is not something the model states: it must not reach
		// the digest as an assertion.
		{"type_kwd": "nav_cluster", "id": "nav-1"},
		{
			"type_kwd":             "entity",
			"entity_type_kwd":      "person",
			"name_kwd":             "ada",
			"attr":                 `{"alias":"A","birth_date":"1815"}`,
			"review_status_kwd":    "unreviewed",
			"source_chunk_ids":     []string{"c1"},
			"doc_id":               "doc-1",
			"doc_ids_kwd":          []string{"doc-1"},
			"content_with_weight":  `{"name":"Ada","evidence":[{"quote":"Ada leads the guild.","chunk_id":"c2"}]}`,
			"mention_count_int":    2,
			"compilation_template": "tpl-1",
		},
		{
			"type_kwd":            "relation",
			"prop_kwd":            "leads",
			"from_entity_kwd":     "ada",
			"to_entity_kwd":       "guild",
			"from_type_kwd":       "person",
			"to_type_kwd":         "organization",
			"content_with_weight": `{}`,
		},
	}
	svc := newOntologyTestService(guildScope(), nil, nil,
		&nlp.RetrievalResult{Chunks: rows, Total: int64(len(rows))})

	result, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	digest := result.Digest
	if len(digest.Assertions) != 2 {
		t.Fatalf("assertions = %#v, want the entity and the relation only", digest.Assertions)
	}
	entity := digest.Assertions[0]
	if entity.Kind != "entity" || entity.Type != "person" {
		t.Fatalf("entity = %#v, want the row's own role and class", entity)
	}
	if entity.Evidence != "Ada leads the guild." {
		t.Fatalf("evidence = %q, want the quotation the payload carries", entity.Evidence)
	}
	if len(entity.ChunkIDs) != 2 || entity.ChunkIDs[0] != "c2" || entity.ChunkIDs[1] != "c1" {
		t.Fatalf("chunk ids = %v, want the evidence's chunk first", entity.ChunkIDs)
	}
	if len(entity.DocIDs) != 1 || entity.DocIDs[0] != "doc-1" {
		t.Fatalf("doc ids = %v, want the row's document", entity.DocIDs)
	}
	if entity.Attributes["alias"] != "A" || entity.Attributes["birth_date"] != "1815" {
		t.Fatalf("attributes = %#v, want the attr column decoded", entity.Attributes)
	}
	if entity.ReviewStatus != string(AssertionReviewUnreviewed) {
		t.Fatalf("review status = %q, want the row's governance state", entity.ReviewStatus)
	}
	relation := digest.Assertions[1]
	if relation.From != "ada" || relation.To != "guild" || relation.FromType != "person" || relation.ToType != "organization" {
		t.Fatalf("relation = %#v, want both endpoints and their declared classes", relation)
	}
	// Both rows count as unreviewed: the relation carries no review column at
	// all, and an absent status is the same state as an unset one — the rule the
	// review queue counts by.
	if relation.ReviewStatus != string(AssertionReviewUnreviewed) {
		t.Fatalf("relation review status = %q, want an absent column read as unreviewed", relation.ReviewStatus)
	}
	if digest.UnreviewedCount != 2 {
		t.Fatalf("unreviewed = %d, want both rows counted as pending", digest.UnreviewedCount)
	}
	if digest.Citations.Resolvable != 2 || digest.Citations.Unresolvable != 0 {
		t.Fatalf("citations = %#v, want the entity's two chunks readable with no scope pinned", digest.Citations)
	}
	if digest.Coverage.Sampled != 0 || digest.Coverage.SampleLimit != ontologyCoverageSampleLimit {
		t.Fatalf("coverage = %#v, want the sample limit reported even when nothing was sampled", digest.Coverage)
	}
}

// An assertion whose sources are all outside the conversation's readable
// documents is still returned — its wording is a search hint — but the quotations
// may not be cited, and the digest says so instead of letting them disappear.
func TestOntologyABoxRetrievalCountsUnreadableQuotations(t *testing.T) {
	rows := []map[string]interface{}{{
		"type_kwd":            "entity",
		"entity_type_kwd":     "person",
		"name_kwd":            "ada",
		"source_chunk_ids":    []string{"c1"},
		"doc_id":              "doc-1",
		"doc_ids_kwd":         []string{"doc-1"},
		"content_with_weight": `{"name":"Ada"}`,
	}}
	svc := newOntologyTestService(guildScope(), nil, nil,
		&nlp.RetrievalResult{Chunks: rows, Total: 1})

	req := validOntologyABoxRequest()
	req.DocumentScope = []string{}
	result, err := svc.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if result.Digest.Citations.Resolvable != 0 || result.Digest.Citations.Unresolvable != 1 {
		t.Fatalf("citations = %#v, want the chunk counted as unreadable", result.Digest.Citations)
	}
	if result.Digest.Degrade == nil || result.Digest.Degrade.Code != agentic_rag.OntologyDegradeCitationsUnresolvable {
		t.Fatalf("degrade = %#v, want the unresolvable-citations code", result.Digest.Degrade)
	}
	if result.Digest.EvidenceLevel != agentic_rag.OntologyEvidenceSchemaFree {
		t.Fatalf("evidence level = %q, want the grade to describe the rows, not the citations", result.Digest.EvidenceLevel)
	}
}

// Coverage is a bounded sample, and the bound travels with it: "the class has no
// instance" and "the scan stopped before it" must not read the same.
func TestOntologyABoxRetrievalReportsCoverageAndItsLimits(t *testing.T) {
	coverage := &fakeOntologyCoverage{sample: ontologyCoverageSample{
		Classes:    map[string]bool{"person": true},
		Properties: map[string]bool{},
		Sampled:    7,
		Truncated:  true,
	}}
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1"}},
		coverage)

	result, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	got := result.Digest.Coverage
	if !containsStringValue(got.MatchedClasses, "person") {
		t.Fatalf("coverage = %#v, want the class the sample exercised", got)
	}
	for _, want := range []string{"agent", "organization"} {
		if !containsStringValue(got.MissingClasses, want) {
			t.Fatalf("coverage = %#v, want %s reported as declared-without-instance", got, want)
		}
	}
	if !got.Truncated || got.Sampled != 7 || got.SampleLimit != ontologyCoverageSampleLimit {
		t.Fatalf("coverage = %#v, want the sample and its limit reported", got)
	}
	if coverage.limit != ontologyCoverageSampleLimit || len(coverage.datasetIDs) != 1 || coverage.datasetIDs[0] != "kb-1" {
		t.Fatalf("coverage scan = %v/%d, want the knowledge scope and the bound", coverage.datasetIDs, coverage.limit)
	}
}

// A coverage scan that could not run is reported as unknown, never as an empty
// coverage: the two call for different actions and only one is about the corpus.
func TestOntologyABoxRetrievalReportsUnknownCoverage(t *testing.T) {
	svc := newOntologyTestService(guildScope(), nil,
		&fakeOntologyCoverage{err: errors.New("engine unreachable")})

	result, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	degrade := result.Digest.Degrade
	if degrade == nil || degrade.Code != agentic_rag.OntologyDegradeCoverageUnknown {
		t.Fatalf("degrade = %#v, want the coverage-unknown code", degrade)
	}
	if result.Digest.Coverage.Sampled != 0 {
		t.Fatalf("coverage = %#v, want no sample claimed", result.Digest.Coverage)
	}
}

// Skipping pending assertions is an explicit request, and it has to follow the
// rows into the search rather than be filtered afterwards: the page is bounded,
// so a post-filter would return fewer rows than it carried. The condition is the
// accepted status, not the absence of the unreviewed one — a rejected assertion
// is also one nobody should read, and a row with no status column at all is
// unreviewed.
func TestOntologyABoxRetrievalExcludesUnreviewedWhenAsked(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1"}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1})

	req := validOntologyABoxRequest()
	req.EntityTypes = []string{"agent"}
	req.SkipUnreviewed = true
	if _, err := svc.svc.Retrieve(context.Background(), req); err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	must, ok := svc.searcher.calls[0].Filter["must"].(map[string]interface{})
	if !ok {
		t.Fatalf("filter = %#v, want the accepted rows required in the request", svc.searcher.calls[0].Filter)
	}
	statuses, _ := must["review_status_kwd"].([]string)
	if len(statuses) != 1 || statuses[0] != string(AssertionReviewAccepted) {
		t.Fatalf("must = %#v, want only the accepted status selectable", must)
	}
	if _, excluded := svc.searcher.calls[0].Filter["must_not"]; excluded {
		t.Fatalf("filter = %#v, want the accepted selection, not an exclusion list", svc.searcher.calls[0].Filter)
	}
}

// A coverage caveat must not overwrite the reason the rows are unnarrowed: the
// search-level reason explains the rows, the coverage one explains the report.
func TestOntologyABoxRetrievalKeepsTheFirstDegradeReason(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{err: errors.New("model not found")},
		&fakeOntologyCoverage{err: errors.New("engine unreachable")})

	result, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if result.Digest.Degrade.Code != agentic_rag.OntologyDegradeNoModel {
		t.Fatalf("degrade = %#v, want the search-level reason kept", result.Digest.Degrade)
	}
}

// The remaining refusals: each is a request that cannot be answered honestly.
func TestOntologyABoxRetrievalRefusesIncompleteRequests(t *testing.T) {
	cases := map[string]func(*OntologyABoxRetrievalRequest){
		"no caller":   func(r *OntologyABoxRetrievalRequest) { r.UserID = "" },
		"no datasets": func(r *OntologyABoxRetrievalRequest) { r.DatasetIDs = nil },
		"no question": func(r *OntologyABoxRetrievalRequest) { r.Question = "  " },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			svc := newOntologyTestService(guildScope(), nil, nil)
			req := validOntologyABoxRequest()
			mutate(req)

			if _, err := svc.svc.Retrieve(context.Background(), req); err == nil {
				t.Fatalf("%s: expected a refusal", name)
			}
			if svc.searcher.callCount() != 0 {
				t.Fatalf("%s: the engine must not be searched", name)
			}
		})
	}
}

// An unwired service refuses instead of searching with a nil boundary.
func TestOntologyABoxRetrievalRefusesWhenUnwired(t *testing.T) {
	svc := &OntologyABoxRetrievalService{}
	if _, err := svc.Retrieve(context.Background(), validOntologyABoxRequest()); err == nil {
		t.Fatal("expected an unwired service to refuse")
	}
	if _, err := svc.Retrieve(context.Background(), nil); err == nil {
		t.Fatal("expected a nil request to be refused")
	}
}

// The dataset access check is the isolation boundary of the knowledge scope: a
// caller who may read the conversation's documents is not thereby allowed to
// read another library's compiled rows.
func TestOntologyABoxRetrievalPropagatesTheScopeRefusal(t *testing.T) {
	svc := newOntologyTestService(guildScope(), nil, nil)
	svc.svc.scopeResolver = &fakeOntologyScope{err: errors.New("only the owner of dataset kb-1 is authorized for this operation")}

	_, err := svc.svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err == nil || !strings.Contains(err.Error(), "authorized") {
		t.Fatalf("err = %v, want the access refusal surfaced", err)
	}
	if svc.searcher.callCount() != 0 {
		t.Fatalf("engine calls = %d, want none after a refused scope", svc.searcher.callCount())
	}
}
