package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/service/nlp"
)

type fakeOntologyScope struct {
	scope ontologyABoxScope
	err   error
}

func (f fakeOntologyScope) Resolve(_ context.Context, _ string, _ []string) (ontologyABoxScope, error) {
	return f.scope, f.err
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

func newOntologyABoxTestService(
	scope ontologyABoxScope,
	filter map[string]interface{},
	filterErr error,
	results ...*nlp.RetrievalResult,
) (*OntologyABoxRetrievalService, *fakeOntologySearcher) {
	if len(results) == 0 {
		results = []*nlp.RetrievalResult{{
			Chunks: []map[string]interface{}{{"content_with_weight": "row"}},
			Total:  1,
		}}
	}
	searcher := &fakeOntologySearcher{results: results}
	svc := &OntologyABoxRetrievalService{
		scopeResolver: fakeOntologyScope{scope: scope},
		searcher:      searcher,
		structureFilter: func(_ context.Context, _, _ string, _ OntologyStructureConstraints) (map[string]interface{}, error) {
			if filterErr != nil {
				return nil, filterErr
			}
			return filter, nil
		},
	}
	return svc, searcher
}

func validOntologyABoxRequest() *OntologyABoxRetrievalRequest {
	return &OntologyABoxRetrievalRequest{
		UserID:             "user-1",
		DatasetIDs:         []string{"kb-1"},
		Question:           "who leads the guild",
		OntologyTemplateID: "tpl-1",
		EntityTypes:        []string{"agent"},
	}
}

// Without a constraint this endpoint would be "search the compiled rows", which
// the main retrieval already owns. Refusing keeps one owner per question, and
// nothing should reach the engine.
func TestOntologyABoxRetrievalRequiresAConstraint(t *testing.T) {
	svc, searcher := newOntologyABoxTestService(ontologyABoxScope{TenantIDs: []string{"tenant-1"}}, nil, nil)
	req := validOntologyABoxRequest()
	req.EntityTypes = nil

	if _, err := svc.Retrieve(context.Background(), req); err == nil {
		t.Fatal("expected a constraint-less request to be refused")
	}
	if searcher.callCount() != 0 {
		t.Fatal("the engine must not be searched without a constraint")
	}
}

// The filter reaches the engine unchanged and comes back on the result: the
// expansion is the answer to "why these rows", so the caller has to see it.
func TestOntologyABoxRetrievalAppliesAndEchoesTheFilter(t *testing.T) {
	filter := map[string]interface{}{
		"entity_type_kwd":     []string{"agent", "person", "organization"},
		"knowledge_graph_kwd": []string{"entity"},
	}
	svc, searcher := newOntologyABoxTestService(
		ontologyABoxScope{TenantIDs: []string{"tenant-1"}},
		filter, nil,
	)

	result, err := svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if searcher.callCount() != 1 {
		t.Fatalf("engine calls = %d, want the applied structure searched once", searcher.callCount())
	}
	issued := searcher.calls[0]
	if got := issued.Filter; got["entity_type_kwd"] == nil {
		t.Fatalf("filter = %#v, want the expanded classes", got)
	}
	if result.Filter["knowledge_graph_kwd"] == nil {
		t.Fatalf("result filter = %#v, want the applied filter echoed", result.Filter)
	}
	if !result.StructureApplied || result.DegradeCode != "" {
		t.Fatalf("result = %#v, want an applied structure with no degrade code", result)
	}
	if len(issued.KbIDs) != 1 || issued.KbIDs[0] != "kb-1" {
		t.Fatalf("kb ids = %v, want the requested scope", issued.KbIDs)
	}
	if issued.PageSize != 10 {
		t.Fatalf("page size = %d, want the default top_k", issued.PageSize)
	}
	if len(result.Chunks) != 1 || result.Total != 1 {
		t.Fatalf("result = %#v, want the engine rows passed through", result)
	}
}

// A model that cannot be read must not make the corpus unsearchable: the rows
// still come back, unnarrowed, with the code that says which half is missing.
func TestOntologyABoxRetrievalDegradesWhenTheModelCannotBeRead(t *testing.T) {
	svc, searcher := newOntologyABoxTestService(
		ontologyABoxScope{TenantIDs: []string{"tenant-1"}},
		nil, errors.New("template could not be read"),
	)

	result, err := svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if searcher.callCount() != 1 {
		t.Fatalf("engine calls = %d, want the unfiltered search still issued", searcher.callCount())
	}
	if got := searcher.calls[0].Filter; got != nil {
		t.Fatalf("filter = %#v, want no narrowing when the model is unreadable", got)
	}
	if result.StructureApplied || result.DegradeCode != OntologyABoxDegradeNoModel {
		t.Fatalf("result = %#v, want a no_model degrade", result)
	}
	if result.DegradeReason == "" {
		t.Fatal("degrade reason must be reported, not swallowed")
	}
	if result.Filter != nil {
		t.Fatalf("result filter = %#v, want nil so the echo never shows an unapplied filter", result.Filter)
	}
	if len(result.Chunks) != 1 {
		t.Fatalf("chunks = %#v, want the unnarrowed rows", result.Chunks)
	}
}

// A declared structure that matches nothing is answered from the same question
// asked without it, and the code tells the caller the rows are not the narrowed
// ones — that is the signal the model's coverage came up short.
func TestOntologyABoxRetrievalFallsBackWhenTheStructureMatchesNothing(t *testing.T) {
	svc, searcher := newOntologyABoxTestService(
		ontologyABoxScope{TenantIDs: []string{"tenant-1"}},
		map[string]interface{}{"entity_type_kwd": []string{"agent"}}, nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
		&nlp.RetrievalResult{
			Chunks: []map[string]interface{}{{"content_with_weight": "chunk"}},
			Total:  1,
		},
	)

	result, err := svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if searcher.callCount() != 2 {
		t.Fatalf("engine calls = %d, want the narrowing and then the retry", searcher.callCount())
	}
	if searcher.calls[0].Filter == nil {
		t.Fatal("the first attempt must carry the narrowing")
	}
	if searcher.calls[1].Filter != nil {
		t.Fatalf("retry filter = %#v, want the same question without the narrowing", searcher.calls[1].Filter)
	}
	if result.StructureApplied || result.DegradeCode != OntologyABoxDegradeNoMatch {
		t.Fatalf("result = %#v, want a no_match degrade", result)
	}
	if result.Total != 1 || len(result.Chunks) != 1 {
		t.Fatalf("result = %#v, want the fallback rows", result)
	}
	if result.Filter != nil {
		t.Fatalf("result filter = %#v, want nil for rows that were not narrowed", result.Filter)
	}
}

// The retry is a single second attempt, not a loop: an empty corpus stays empty
// and the answer still says the structure was not what produced the rows.
func TestOntologyABoxRetrievalDoesNotRetryTwice(t *testing.T) {
	svc, searcher := newOntologyABoxTestService(
		ontologyABoxScope{TenantIDs: []string{"tenant-1"}},
		map[string]interface{}{"entity_type_kwd": []string{"agent"}}, nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
	)

	result, err := svc.Retrieve(context.Background(), validOntologyABoxRequest())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if searcher.callCount() != 2 {
		t.Fatalf("engine calls = %d, want exactly one retry", searcher.callCount())
	}
	if result.DegradeCode != OntologyABoxDegradeNoMatch {
		t.Fatalf("degrade code = %q, want the no_match code on an empty corpus", result.DegradeCode)
	}
	if result.Chunks == nil {
		t.Fatal("chunks must be an empty slice, never nil, for a caller that iterates it")
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
			svc, searcher := newOntologyABoxTestService(ontologyABoxScope{TenantIDs: []string{"tenant-1"}}, nil, nil)
			req := validOntologyABoxRequest()
			mutate(req)

			if _, err := svc.Retrieve(context.Background(), req); err == nil {
				t.Fatalf("%s: expected a refusal", name)
			}
			if searcher.callCount() != 0 {
				t.Fatalf("%s: the engine must not be searched", name)
			}
		})
	}
}
