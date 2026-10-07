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

// Tests for Engine.MetaFacets.
//
// The fake below holds documents rather than canned counts: it applies the
// caller's kb_id and document-id scope, pages composite buckets the way ES
// does, counts a document once per distinct value, and answers the coverage
// and carrying filters from the documents in scope. A facet that ignored the
// id scope, stopped after one page or one id batch, or counted a value twice
// would therefore come back wrong rather than pass on a forgiving fixture.

package elasticsearch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/engine/types"
)

// facetDoc is one row of the fake doc-meta index. ignored names the keys an
// indexing decision dropped a value of (what ES records in _ignored).
type facetDoc struct {
	id      string
	kb      string
	meta    map[string][]string
	ignored []string
}

type fakeFacetES struct {
	mu      sync.Mutex
	mapping map[string]string // key -> ES type ("text" gets a .keyword subfield)
	docs    []facetDoc

	// Recorded for assertions.
	searches     int
	maxIDsInTerm int

	// Injected failures.
	failedShards int
	dropCarrying bool
}

type facetSearchBody struct {
	Query struct {
		Bool struct {
			Filter []struct {
				Term map[string]string `json:"term"`
				Bool struct {
					Should []struct {
						Terms map[string][]string `json:"terms"`
					} `json:"should"`
				} `json:"bool"`
			} `json:"filter"`
		} `json:"bool"`
	} `json:"query"`
	Aggs map[string]struct {
		Composite struct {
			Size    int                        `json:"size"`
			Sources []compositeSource          `json:"sources"`
			After   map[string]json.RawMessage `json:"after"`
		} `json:"composite"`
		Filters struct {
			Filters map[string]json.RawMessage `json:"filters"`
		} `json:"filters"`
		Filter *struct {
			Bool struct {
				Should []struct {
					Exists struct {
						Field string `json:"field"`
					} `json:"exists"`
				} `json:"should"`
			} `json:"bool"`
		} `json:"filter"`
	} `json:"aggs"`
}

// scope returns the documents the query selects: the kb_id term, and the id
// terms (the fake keeps id and _id equal, as the store does).
func (f *fakeFacetES) scope(t *testing.T, body facetSearchBody) []facetDoc {
	t.Helper()
	var kb string
	ids := map[string]bool{}
	sawIDs := false
	for _, clause := range body.Query.Bool.Filter {
		if v, ok := clause.Term["kb_id"]; ok {
			kb = v
		}
		for _, should := range clause.Bool.Should {
			for field, values := range should.Terms {
				if field != "id" && field != "_id" {
					t.Errorf("unexpected terms field %q in the id scope", field)
				}
				if len(values) > metaFacetIDBatch {
					t.Errorf("terms on %s names %d ids, over index.max_terms_count %d", field, len(values), metaFacetIDBatch)
				}
				f.maxIDsInTerm = max(f.maxIDsInTerm, len(values))
				sawIDs = true
				for _, id := range values {
					ids[id] = true
				}
			}
		}
	}
	if kb == "" || !sawIDs {
		t.Errorf("facet query must be scoped by kb_id and document id: %+v", body.Query)
	}
	var out []facetDoc
	for _, doc := range f.docs {
		if doc.kb == kb && ids[doc.id] {
			out = append(out, doc)
		}
	}
	return out
}

func (f *fakeFacetES) aggPath(key string) string {
	if f.mapping[key] == "text" {
		return "meta_fields." + key + ".keyword"
	}
	return "meta_fields." + key
}

func (f *fakeFacetES) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/_mapping"):
			props := map[string]interface{}{}
			for key, typ := range f.mapping {
				if typ == "text" {
					props[key] = map[string]interface{}{
						"type":   "text",
						"fields": map[string]interface{}{"keyword": map[string]interface{}{"type": "keyword"}},
					}
					continue
				}
				props[key] = map[string]interface{}{"type": typ}
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"idx": map[string]interface{}{"mappings": map[string]interface{}{"properties": map[string]interface{}{
					"meta_fields": map[string]interface{}{"properties": props},
				}}},
			})
		case strings.HasSuffix(r.URL.Path, "/_search"):
			f.mu.Lock()
			defer f.mu.Unlock()
			f.searches++
			if got := r.URL.Query().Get("allow_partial_search_results"); got != "false" {
				t.Errorf("allow_partial_search_results: got %q, want false", got)
			}
			raw, _ := io.ReadAll(r.Body)
			var body facetSearchBody
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("search body is not JSON: %v", err)
			}
			docs := f.scope(t, body)

			aggregations := map[string]interface{}{}
			for name, agg := range body.Aggs {
				switch {
				case name == metaValueSpaceCoverageAgg:
					buckets := map[string]interface{}{}
					for key, filter := range agg.Filters.Filters {
						buckets[key] = map[string]interface{}{"doc_count": f.uncovered(t, docs, key, filter)}
					}
					aggregations[name] = map[string]interface{}{"buckets": buckets}
				case name == metaFacetCarryingAgg:
					if f.dropCarrying {
						continue
					}
					count := 0
					for _, doc := range docs {
						for _, should := range agg.Filter.Bool.Should {
							if len(doc.meta[strings.TrimPrefix(should.Exists.Field, "meta_fields.")]) > 0 {
								count++
								break
							}
						}
					}
					aggregations[name] = map[string]interface{}{"doc_count": count}
				default:
					var key string
					for k := range agg.Composite.Sources[0] {
						key = k
					}
					if path := agg.Composite.Sources[0][key].Terms.Field; path != f.aggPath(key) {
						t.Errorf("composite source for %q reads %q, want %q", key, path, f.aggPath(key))
					}
					aggregations[name] = f.page(docs, key, agg.Composite.Size, storedForm(agg.Composite.After[key]))
				}
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"timed_out":    false,
				"_shards":      map[string]interface{}{"total": 3, "successful": 3 - f.failedShards, "failed": f.failedShards},
				"aggregations": aggregations,
			})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// uncovered answers one coverage filter from the documents in scope.
func (f *fakeFacetES) uncovered(t *testing.T, docs []facetDoc, key string, raw json.RawMessage) int {
	t.Helper()
	var clause struct {
		Term   map[string]string `json:"term"`
		Exists *struct {
			Field string `json:"field"`
		} `json:"exists"`
	}
	if err := json.Unmarshal(raw, &clause); err != nil {
		t.Fatalf("coverage filter for %q is not JSON: %v", key, err)
	}
	count := 0
	for _, doc := range docs {
		switch {
		case clause.Term["_ignored"] != "":
			for _, k := range doc.ignored {
				if f.aggPath(k) == clause.Term["_ignored"] {
					count++
					break
				}
			}
		case clause.Exists != nil:
			if len(doc.meta[strings.TrimPrefix(clause.Exists.Field, "meta_fields.")]) > 0 {
				count++
			}
		default:
			t.Errorf("unsupported coverage filter for %q: %s", key, raw)
		}
	}
	return count
}

// page returns one composite page of key's distinct values in scope, each with
// the number of documents holding it -- a document once per distinct value.
func (f *fakeFacetES) page(docs []facetDoc, key string, size int, after string) map[string]interface{} {
	holders := map[string]map[string]bool{}
	for _, doc := range docs {
		for _, value := range doc.meta[key] {
			if holders[value] == nil {
				holders[value] = map[string]bool{}
			}
			holders[value][doc.id] = true
		}
	}
	values := make([]string, 0, len(holders))
	for value := range holders {
		if after == "" || value > after {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	if len(values) > size {
		values = values[:size]
	}
	buckets := make([]interface{}, 0, len(values))
	for _, value := range values {
		buckets = append(buckets, map[string]interface{}{
			"key":       map[string]interface{}{key: bucketKey(value, f.mapping[key])},
			"doc_count": len(holders[value]),
		})
	}
	page := map[string]interface{}{"buckets": buckets}
	if len(values) == size && len(values) > 0 {
		page["after_key"] = map[string]interface{}{key: bucketKey(values[len(values)-1], f.mapping[key])}
	}
	return page
}

func newMetaFacetsEngine(t *testing.T, fake *fakeFacetES) *Engine {
	t.Helper()
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)
	return newTestEngine(t, srv.URL)
}

// The facet counts documents per value inside the caller's document scope: a
// row of another knowledge base, or one whose document the caller did not list
// (an orphaned doc-meta row), contributes nothing, and a document holding the
// same value twice is one document.
func TestMetaFacets_CountsDocumentsInScope(t *testing.T) {
	fake := &fakeFacetES{
		mapping: map[string]string{"project": "text", "phase": "keyword", "published": "date"},
		docs: []facetDoc{
			{id: "d1", kb: "kb1", meta: map[string][]string{"project": {"alpha"}, "phase": {"draft", "draft"}}},
			{id: "d2", kb: "kb1", meta: map[string][]string{"project": {"alpha", "beta"}, "published": {"1784764800000"}}},
			{id: "d3", kb: "kb1"},
			{id: "orphan", kb: "kb1", meta: map[string][]string{"project": {"ghost"}, "phase": {"ghost"}}},
			{id: "d1", kb: "kb2", meta: map[string][]string{"project": {"other-kb"}}},
		},
	}
	e := newMetaFacetsEngine(t, fake)

	counts, carrying, err := e.MetaFacets(t.Context(), "t1", "kb1", []string{"d1", "d2", "d3"})
	if err != nil {
		t.Fatalf("MetaFacets: %v", err)
	}
	want := map[string]map[string]int64{
		"project":   {"alpha": 2, "beta": 1},
		"phase":     {"draft": 1},
		"published": {"2026-07-23": 1},
	}
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("counts: got %v, want %v", counts, want)
	}
	if carrying != 2 {
		t.Errorf("carrying: got %d, want 2 (d3 holds no metadata)", carrying)
	}
	if fake.searches != 1 {
		t.Errorf("searches: got %d, want 1", fake.searches)
	}
}

// ES caps a terms query at index.max_terms_count, so the ids go in batches and
// the counts are added across them.
func TestMetaFacets_BatchesTheIDScope(t *testing.T) {
	docIDs := make([]string, 0, metaFacetIDBatch+2)
	for i := range metaFacetIDBatch + 2 {
		docIDs = append(docIDs, fmt.Sprintf("d%06d", i))
	}
	first, last := docIDs[0], docIDs[len(docIDs)-1]
	fake := &fakeFacetES{
		mapping: map[string]string{"phase": "keyword"},
		docs: []facetDoc{
			{id: first, kb: "kb1", meta: map[string][]string{"phase": {"draft"}}},
			{id: last, kb: "kb1", meta: map[string][]string{"phase": {"draft", "final"}}},
		},
	}
	e := newMetaFacetsEngine(t, fake)

	counts, carrying, err := e.MetaFacets(t.Context(), "t1", "kb1", docIDs)
	if err != nil {
		t.Fatalf("MetaFacets: %v", err)
	}
	if want := map[string]map[string]int64{"phase": {"draft": 2, "final": 1}}; !reflect.DeepEqual(counts, want) {
		t.Errorf("counts: got %v, want %v", counts, want)
	}
	if carrying != 2 {
		t.Errorf("carrying: got %d, want 2", carrying)
	}
	if fake.searches != 2 {
		t.Errorf("searches: got %d, want 2 (one per id batch)", fake.searches)
	}
	if fake.maxIDsInTerm != metaFacetIDBatch {
		t.Errorf("largest id batch: got %d, want %d", fake.maxIDsInTerm, metaFacetIDBatch)
	}
}

// A key with more distinct values than one page is counted whole.
func TestMetaFacets_PagesPastOnePage(t *testing.T) {
	values := series("v", metaValueSpacePageSize+5)
	fake := &fakeFacetES{
		mapping: map[string]string{"code": "keyword"},
		docs: []facetDoc{
			{id: "d1", kb: "kb1", meta: map[string][]string{"code": values}},
			{id: "d2", kb: "kb1", meta: map[string][]string{"code": {values[len(values)-1]}}},
		},
	}
	e := newMetaFacetsEngine(t, fake)

	counts, _, err := e.MetaFacets(t.Context(), "t1", "kb1", []string{"d1", "d2"})
	if err != nil {
		t.Fatalf("MetaFacets: %v", err)
	}
	if got := len(counts["code"]); got != len(values) {
		t.Fatalf("code values: got %d, want %d", got, len(values))
	}
	if got := counts["code"][values[len(values)-1]]; got != 2 {
		t.Errorf("count of the last value: got %d, want 2", got)
	}
	if fake.searches != 2 {
		t.Errorf("searches: got %d, want 2", fake.searches)
	}
}

// Wherever the aggregation could be quietly wrong it refuses, so the caller
// counts the documents instead: a facet count is read as exact.
func TestMetaFacets_RefusesWhatItCannotCountExactly(t *testing.T) {
	cases := []struct {
		name  string
		fake  *fakeFacetES
		apply func(*fakeFacetES)
	}{
		{
			name: "value past ignore_above beside a short one",
			fake: &fakeFacetES{
				mapping: map[string]string{"project": "text"},
				docs:    []facetDoc{{id: "d1", kb: "kb1", meta: map[string][]string{"project": {"alpha"}}, ignored: []string{"project"}}},
			},
		},
		{
			name: "key with no aggregatable field",
			fake: &fakeFacetES{
				mapping: map[string]string{"phase": "keyword", "nested": "object"},
				docs:    []facetDoc{{id: "d1", kb: "kb1", meta: map[string][]string{"phase": {"draft"}, "nested": {"{}"}}}},
			},
		},
		{
			name: "only unaggregatable keys",
			fake: &fakeFacetES{mapping: map[string]string{"nested": "object"}},
		},
		{
			// exists counts a blank string, so the document would be neither in
			// a value bucket nor in empty_metadata.
			name: "blank value",
			fake: &fakeFacetES{
				mapping: map[string]string{"phase": "keyword"},
				docs:    []facetDoc{{id: "d1", kb: "kb1", meta: map[string][]string{"phase": {" "}}}},
			},
		},
		{
			name: "failed shard",
			fake: &fakeFacetES{
				mapping: map[string]string{"phase": "keyword"},
				docs:    []facetDoc{{id: "d1", kb: "kb1", meta: map[string][]string{"phase": {"draft"}}}},
			},
			apply: func(f *fakeFacetES) { f.failedShards = 1 },
		},
		{
			name: "carrying count missing",
			fake: &fakeFacetES{
				mapping: map[string]string{"phase": "keyword"},
				docs:    []facetDoc{{id: "d1", kb: "kb1", meta: map[string][]string{"phase": {"draft"}}}},
			},
			apply: func(f *fakeFacetES) { f.dropCarrying = true },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.apply != nil {
				tc.apply(tc.fake)
			}
			e := newMetaFacetsEngine(t, tc.fake)

			counts, carrying, err := e.MetaFacets(t.Context(), "t1", "kb1", []string{"d1"})
			if !errors.Is(err, types.ErrMetaValueSpaceIncomplete) {
				t.Fatalf("error: got %v, want ErrMetaValueSpaceIncomplete", err)
			}
			if counts != nil || carrying != 0 {
				t.Errorf("a refused facet must return no counts, got %v / %d", counts, carrying)
			}
		})
	}
}

// The mapping lists every key the tenant ever indexed; one that no document in
// scope holds must not force the slow path for this dataset.
func TestMetaFacets_UnaggregatableKeyOutsideScopeIsFine(t *testing.T) {
	fake := &fakeFacetES{
		mapping: map[string]string{"phase": "keyword", "nested": "object"},
		docs: []facetDoc{
			{id: "d1", kb: "kb1", meta: map[string][]string{"phase": {"draft"}}},
			{id: "d9", kb: "kb2", meta: map[string][]string{"nested": {"{}"}}},
		},
	}
	e := newMetaFacetsEngine(t, fake)

	counts, carrying, err := e.MetaFacets(t.Context(), "t1", "kb1", []string{"d1"})
	if err != nil {
		t.Fatalf("MetaFacets: %v", err)
	}
	if want := map[string]map[string]int64{"phase": {"draft": 1}}; !reflect.DeepEqual(counts, want) || carrying != 1 {
		t.Errorf("got %v / %d, want %v / 1", counts, carrying, want)
	}
}

// No documents in scope, or no index yet, is an empty facet and costs no
// aggregation.
func TestMetaFacets_NothingToCount(t *testing.T) {
	fake := &fakeFacetES{mapping: map[string]string{"phase": "keyword"}}
	e := newMetaFacetsEngine(t, fake)

	counts, carrying, err := e.MetaFacets(t.Context(), "t1", "kb1", nil)
	if err != nil || len(counts) != 0 || carrying != 0 {
		t.Fatalf("no documents: got %v / %d, err %v", counts, carrying, err)
	}
	if fake.searches != 0 {
		t.Errorf("searches: got %d, want 0", fake.searches)
	}

	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method != http.MethodHead {
			t.Errorf("unexpected request %s %s for a missing index", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(missing.Close)
	counts, carrying, err = newTestEngine(t, missing.URL).MetaFacets(t.Context(), "t1", "kb1", []string{"d1"})
	if err != nil || len(counts) != 0 || carrying != 0 {
		t.Fatalf("missing index: got %v / %d, err %v", counts, carrying, err)
	}
}
