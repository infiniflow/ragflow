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

// Tests for Engine.MetaValueSpace.
//
// The value space is what the metadata filter generator picks a filter from,
// and that filter is applied as a document scope *before* scoring -- so a value
// the model was never shown cannot be chosen, and a wrong choice excludes the
// right documents without raising.
//
// The fake below pages the way a composite aggregation does: values sorted, one
// page at a time, after_key set while more remain. A caller that stopped after
// the first page would therefore provably return an incomplete value list,
// rather than the suite passing on a fake that always hands back everything.

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
	"strconv"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/engine/types"
)

// fakeField is one metadata key in the fake index: the mapped ES type, the
// distinct values the aggregation would report for it, and how the indexer
// treated the documents carrying it.
//
// carrying/indexed/ignored are what the coverage question is answered from.
// ignore_above is applied to each array element on its own, so a document can
// be counted in both indexed and ignored at once -- ["short", <too long>]
// leaves the .keyword subfield present and still names it in _ignored. Leaving
// carrying and indexed at zero means "one document per distinct value, all of
// them indexed".
type fakeField struct {
	typ      string
	values   []string
	carrying int
	indexed  int
	ignored  int
}

// counts resolves the defaults: a key whose documents were not described holds
// one document per value, each fully indexed.
func (f fakeField) counts() (carrying, indexed int) {
	if f.carrying == 0 && f.indexed == 0 {
		return len(f.values), len(f.values)
	}
	return f.carrying, f.indexed
}

// fakeES answers the three calls MetaValueSpace makes: the index existence
// check, the mapping read, and the composite aggregation rounds.
type fakeES struct {
	mu     sync.Mutex
	fields map[string]fakeField

	// Recorded for assertions.
	searches      int
	requestedSize map[string]int
	partialParam  []string
	searchedKeys  [][]string

	// Injected failures for one response.
	timedOut     bool
	failedShards int
	dropAgg           string
	coverageRequested int
}

// uncoveredCount answers one coverage filter the way the fake's index would,
// by reading the filter the caller actually sent rather than a canned number.
func (f *fakeES) uncoveredCount(t *testing.T, key string, raw json.RawMessage) int {
	t.Helper()
	var clause map[string]json.RawMessage
	if err := json.Unmarshal(raw, &clause); err != nil {
		t.Errorf("coverage filter for %q is not JSON: %v", key, err)
		return 0
	}
	field := f.fields[key]
	carrying, indexed := field.counts()
	aggPath := "meta_fields." + key
	if field.typ == "text" {
		aggPath += ".keyword"
	}

	if raw, ok := clause["term"]; ok {
		var term map[string]string
		if err := json.Unmarshal(raw, &term); err != nil {
			t.Errorf("term filter for %q is not a string map: %v", key, err)
			return 0
		}
		path, ok := term["_ignored"]
		if !ok {
			t.Errorf("term filter for %q asks about %v, want _ignored", key, term)
			return 0
		}
		if path != aggPath {
			t.Errorf("_ignored filter for %q names %q, want %q", key, path, aggPath)
			return 0
		}
		return field.ignored
	}
	if raw, ok := clause["exists"]; ok {
		var exists struct {
			Field string `json:"field"`
		}
		_ = json.Unmarshal(raw, &exists)
		switch exists.Field {
		case "meta_fields." + key:
			return carrying
		case aggPath:
			return indexed
		}
		t.Errorf("exists filter for %q names %q", key, exists.Field)
		return 0
	}
	if raw, ok := clause["bool"]; ok {
		// The shape this check used before _ignored: the documents that carry
		// the key with nothing in the aggregatable field. It cannot see a
		// document whose other element was indexed.
		var b struct {
			Filter  []json.RawMessage `json:"filter"`
			MustNot []json.RawMessage `json:"must_not"`
		}
		_ = json.Unmarshal(raw, &b)
		if len(b.Filter) == 1 && len(b.MustNot) == 1 {
			return carrying - indexed
		}
	}
	t.Errorf("unsupported coverage filter for %q: %s", key, raw)
	return 0
}

func newFakeES(fields map[string]fakeField) *fakeES {
	return &fakeES{fields: fields, requestedSize: map[string]int{}}
}

type compositeSource map[string]struct {
	Terms struct {
		Field string `json:"field"`
	} `json:"terms"`
}

type searchBody struct {
	Aggs map[string]struct {
		Composite struct {
			Size    int                        `json:"size"`
			Sources []compositeSource          `json:"sources"`
			After   map[string]json.RawMessage `json:"after"`
		} `json:"composite"`
		Filters struct {
			Filters map[string]json.RawMessage `json:"filters"`
		} `json:"filters"`
	} `json:"aggs"`
	Query map[string]interface{} `json:"query"`
}

func (f *fakeES) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/_mapping"):
			props := map[string]interface{}{}
			for key, field := range f.fields {
				if field.typ == "text" {
					props[key] = map[string]interface{}{
						"type":   "text",
						"fields": map[string]interface{}{"keyword": map[string]interface{}{"type": "keyword"}},
					}
					continue
				}
				props[key] = map[string]interface{}{"type": field.typ}
			}
			body := map[string]interface{}{
				"idx": map[string]interface{}{
					"mappings": map[string]interface{}{
						"properties": map[string]interface{}{
							"meta_fields": map[string]interface{}{"properties": props},
						},
					},
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(body)
		case strings.HasSuffix(r.URL.Path, "/_search"):
			f.mu.Lock()
			defer f.mu.Unlock()
			f.searches++
			f.partialParam = append(f.partialParam, r.URL.Query().Get("allow_partial_search_results"))
			raw, _ := io.ReadAll(r.Body)
			var body searchBody
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("search body is not JSON: %v\nbody=%s", err, raw)
			}

			aggregations := map[string]interface{}{}
			var keys []string
			for name, agg := range body.Aggs {
				if name == metaValueSpaceCoverageAgg {
					f.coverageRequested++
					buckets := map[string]interface{}{}
					for key, filter := range agg.Filters.Filters {
						buckets[key] = map[string]interface{}{"doc_count": f.uncoveredCount(t, key, filter)}
					}
					aggregations[name] = map[string]interface{}{"buckets": buckets}
					continue
				}
				if len(agg.Composite.Sources) != 1 {
					t.Errorf("expected exactly one composite source, got %d", len(agg.Composite.Sources))
					continue
				}
				var key string
				for k := range agg.Composite.Sources[0] {
					key = k
				}
				keys = append(keys, key)
				f.requestedSize[key] = agg.Composite.Size
				if name != "vs_"+key {
					t.Errorf("aggregation name %q does not match key %q", name, key)
				}
				if name == f.dropAgg {
					continue
				}
				aggregations[name] = f.page(key, agg.Composite.Size, storedForm(agg.Composite.After[key]))
			}
			sort.Strings(keys)
			f.searchedKeys = append(f.searchedKeys, keys)

			response := map[string]interface{}{
				"timed_out":    f.timedOut,
				"_shards":      map[string]interface{}{"total": 3, "successful": 3 - f.failedShards, "failed": f.failedShards},
				"aggregations": aggregations,
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(response)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// storedForm renders an echoed after_key back into the fake's stored string
// form: after_key travels in the store's own representation (a number for a
// date), never in the rendered one.
func storedForm(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value interface{}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return string(raw)
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// page returns one composite page of the key's values, honoring size and after
// exactly as a composite aggregation does.
func (f *fakeES) page(key string, size int, after string) map[string]interface{} {
	field := f.fields[key]
	values := append([]string(nil), field.values...)
	sort.Strings(values)
	if after != "" {
		kept := values[:0:0]
		for _, value := range values {
			if value > after {
				kept = append(kept, value)
			}
		}
		values = kept
	}
	if len(values) > size {
		values = values[:size]
	}

	buckets := make([]interface{}, 0, len(values))
	for _, value := range values {
		buckets = append(buckets, map[string]interface{}{"key": map[string]interface{}{key: bucketKey(value, field.typ)}})
	}
	page := map[string]interface{}{"buckets": buckets}
	if len(values) == size && len(values) > 0 {
		page["after_key"] = map[string]interface{}{key: bucketKey(values[len(values)-1], field.typ)}
	}
	return page
}

// bucketKey renders a stored value the way ES keys a composite bucket for that
// field type: a date is keyed by epoch milliseconds, never by the date string.
func bucketKey(value, typ string) interface{} {
	if typ == "date" || typ == "long" {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			return n
		}
	}
	return value
}

func newMetaValueSpaceEngine(t *testing.T, fake *fakeES) *Engine {
	t.Helper()
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)
	return newTestEngine(t, srv.URL)
}

// series returns n zero-padded values, so lexical order matches numeric order
// and the fake's paging is deterministic.
func series(prefix string, n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, fmt.Sprintf("%s%05d", prefix, i))
	}
	return out
}

// A value space that fits in one page costs exactly one search, and every key
// travels in that single request.
func TestMetaValueSpace_SingleRoundTrip(t *testing.T) {
	fake := newFakeES(map[string]fakeField{
		"project": {typ: "text", values: []string{"beta", "alpha", "gamma"}},
		"phase":   {typ: "keyword", values: []string{"draft", "final"}},
	})
	e := newMetaValueSpaceEngine(t, fake)

	space, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1", "kb2"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	want := map[string][]string{
		"project": {"alpha", "beta", "gamma"},
		"phase":   {"draft", "final"},
	}
	if !reflect.DeepEqual(space, want) {
		t.Errorf("space: got %v, want %v", space, want)
	}
	if fake.searches != 1 {
		t.Errorf("searches: got %d, want 1 (every key shares one request)", fake.searches)
	}
}

// A key with more distinct values than one page comes back whole: the caller
// pages from after_key until the key is exhausted, and only the unfinished key
// is carried into the next round.
func TestMetaValueSpace_PagesPastOnePage(t *testing.T) {
	fake := newFakeES(map[string]fakeField{
		"phase": {typ: "keyword", values: series("p", 2*metaValueSpacePageSize+7)},
		// text, so the coverage question has a subfield to ask about.
		"project": {typ: "text", values: []string{"alpha"}},
	})
	e := newMetaValueSpaceEngine(t, fake)

	space, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	if got, want := len(space["phase"]), 2*metaValueSpacePageSize+7; got != want {
		t.Fatalf("phase values: got %d, want %d (a truncated page would show %d)", got, want, metaValueSpacePageSize)
	}
	if space["phase"][0] != "p00000" || space["phase"][len(space["phase"])-1] != fmt.Sprintf("p%05d", 2*metaValueSpacePageSize+6) {
		t.Errorf("phase values are not the complete ordered run: first=%q last=%q", space["phase"][0], space["phase"][len(space["phase"])-1])
	}
	if fake.searches != 3 {
		t.Fatalf("searches: got %d, want 3", fake.searches)
	}
	// The coverage question is asked once, not once per round.
	if fake.coverageRequested != 1 {
		t.Errorf("coverage aggregation requested %d times, want 1", fake.coverageRequested)
	}
	// The low-cardinality key finished in round one and must not be re-requested.
	if got := fake.searchedKeys[1]; !reflect.DeepEqual(got, []string{"phase"}) {
		t.Errorf("round 2 requested %v, want only the unfinished key [phase]", got)
	}
	if got := fake.searchedKeys[2]; !reflect.DeepEqual(got, []string{"phase"}) {
		t.Errorf("round 3 requested %v, want only the unfinished key [phase]", got)
	}
}

// The per-key page size is divided out of the cluster's shared bucket budget,
// so adding keys cannot push a single request past search.max_buckets.
func TestMetaValueSpace_PageSizeSharesBucketBudget(t *testing.T) {
	fields := map[string]fakeField{}
	for i := range 100 {
		fields[fmt.Sprintf("key%03d", i)] = fakeField{typ: "keyword", values: []string{"v"}}
	}
	fake := newFakeES(fields)
	e := newMetaValueSpaceEngine(t, fake)

	if _, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1"}); err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	want := esMaxBuckets / 100
	total := 0
	for _, size := range fake.requestedSize {
		if size != want {
			t.Fatalf("page size: got %d, want %d", size, want)
		}
		total += size
	}
	if total > esMaxBuckets {
		t.Errorf("one request would build %d buckets, over the %d budget", total, esMaxBuckets)
	}
}

// The default lets ES answer HTTP 200 with shards missing; the request must opt
// out of that so a partial answer is an error rather than a short value list.
func TestMetaValueSpace_RefusesPartialSearchResults(t *testing.T) {
	fake := newFakeES(map[string]fakeField{"phase": {typ: "keyword", values: []string{"draft"}}})
	e := newMetaValueSpaceEngine(t, fake)

	if _, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1"}); err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	if len(fake.partialParam) != 1 || fake.partialParam[0] != "false" {
		t.Errorf("allow_partial_search_results: got %v, want [false]", fake.partialParam)
	}
}

// A failed shard, a timeout, or an aggregation that came back absent each mean
// values may be missing. Refuse rather than hand back a space that silently
// omits them.
func TestMetaValueSpace_RefusesIncompleteResponses(t *testing.T) {
	cases := []struct {
		name  string
		apply func(*fakeES)
	}{
		{"failed_shard", func(f *fakeES) { f.failedShards = 1 }},
		{"timed_out", func(f *fakeES) { f.timedOut = true }},
		{"missing_aggregation", func(f *fakeES) { f.dropAgg = "vs_phase" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeES(map[string]fakeField{
				"phase":   {typ: "keyword", values: []string{"draft", "final"}},
				"project": {typ: "keyword", values: []string{"alpha"}},
			})
			tc.apply(fake)
			e := newMetaValueSpaceEngine(t, fake)

			space, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1"})
			if !errors.Is(err, types.ErrMetaValueSpaceIncomplete) {
				t.Fatalf("error: got %v, want ErrMetaValueSpaceIncomplete", err)
			}
			if space != nil {
				t.Errorf("a refused read must return no space, got %v", space)
			}
		})
	}
}

// A dynamically mapped string aggregates through its .keyword subfield, which
// carries ignore_above (256 by default): a longer value is indexed as text only
// and has no bucket. A key mapped as an object -- what a dict-valued metadata
// entry creates -- has no aggregatable field at all. Either way the space would
// be missing values the filter generator is then unable to choose, so refuse it.
//
// The mixed case is the one existence cannot answer: ignore_above is applied to
// each array element, so ["alpha", <too long>] keeps the subfield present on
// the document while the long element is missing from the buckets. Only
// _ignored, which names the field the indexer dropped it from, sees it.
func TestMetaValueSpace_RefusesValuesNoBucketCanShow(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]fakeField
	}{
		{
			name:   "no element short enough to index",
			fields: map[string]fakeField{"project": {typ: "text", values: []string{"alpha"}, carrying: 3, indexed: 1, ignored: 2}},
		},
		{
			name:   "one element past ignore_above beside a short one",
			fields: map[string]fakeField{"project": {typ: "text", values: []string{"alpha"}, carrying: 1, indexed: 1, ignored: 1}},
		},
		{
			name: "key with no aggregatable field",
			fields: map[string]fakeField{
				"phase":  {typ: "keyword", values: []string{"draft"}},
				"nested": {typ: "object", carrying: 3},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeES(tc.fields)
			e := newMetaValueSpaceEngine(t, fake)

			space, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1"})
			if !errors.Is(err, types.ErrMetaValueSpaceIncomplete) {
				t.Fatalf("error: got %v, want ErrMetaValueSpaceIncomplete", err)
			}
			if space != nil {
				t.Errorf("a refused read must return no space, got %v", space)
			}
		})
	}
}

// The mapping enumerates every key the tenant ever indexed, so a key no
// document in these knowledge bases carries must not disable filtering for
// them: the coverage question is asked inside the caller's own scope.
func TestMetaValueSpace_UnaggregatableKeyNothingCarriesIsFine(t *testing.T) {
	fake := newFakeES(map[string]fakeField{
		"phase":  {typ: "keyword", values: []string{"draft"}},
		"nested": {typ: "object"},
	})
	e := newMetaValueSpaceEngine(t, fake)

	space, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	if !reflect.DeepEqual(space, map[string][]string{"phase": {"draft"}}) {
		t.Errorf("space: got %v, want map[phase:[draft]]", space)
	}
	if fake.coverageRequested != 1 {
		t.Errorf("coverage aggregation requested %d times, want 1", fake.coverageRequested)
	}
}

// A composite terms source over a date field keys its buckets by epoch millis.
// Left as they arrive the model is offered 1784851200000 where the document
// holds 2026-07-23, so date buckets come back rendered; nothing else changes.
func TestMetaValueSpace_RendersDateBuckets(t *testing.T) {
	fake := newFakeES(map[string]fakeField{
		// 2026-07-23T00:00:00Z and 2026-07-23T10:30:00Z.
		"published": {typ: "date", values: []string{"1784764800000", "1784802600000"}},
		"count":     {typ: "long", values: []string{"7", "42"}},
		"project":   {typ: "text", values: []string{"alpha"}},
	})
	e := newMetaValueSpaceEngine(t, fake)

	space, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	want := map[string][]string{
		"published": {"2026-07-23", "2026-07-23T10:30:00Z"},
		"count":     {"42", "7"}, // lexical order is the fake's, not the point here
		"project":   {"alpha"},
	}
	if !reflect.DeepEqual(space, want) {
		t.Errorf("space: got %v, want %v", space, want)
	}
}

// An index whose mapping has nothing aggregatable yields an empty space and no
// aggregation round, so the caller can fall back to the flattened scan.
func TestMetaValueSpace_NoAggregatableFields(t *testing.T) {
	fake := newFakeES(map[string]fakeField{"nested": {typ: "object", values: []string{"x"}}})
	e := newMetaValueSpaceEngine(t, fake)

	space, err := e.MetaValueSpace(t.Context(), "t1", []string{"kb1"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	if len(space) != 0 {
		t.Errorf("space: got %v, want empty", space)
	}
	if fake.searches != 0 {
		t.Errorf("searches: got %d, want 0", fake.searches)
	}
}

// No knowledge bases means no value space, and no calls to the cluster.
func TestMetaValueSpace_NoKBs(t *testing.T) {
	fake := newFakeES(map[string]fakeField{"phase": {typ: "keyword", values: []string{"draft"}}})
	e := newMetaValueSpaceEngine(t, fake)

	space, err := e.MetaValueSpace(t.Context(), "t1", nil)
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	if len(space) != 0 {
		t.Errorf("space: got %v, want empty", space)
	}
	if fake.searches != 0 {
		t.Errorf("searches: got %d, want 0", fake.searches)
	}
}

// text is analysed and not aggregatable; its keyword subfield is. Every other
// aggregatable type is used directly.
func TestMetaAggFields_FieldPaths(t *testing.T) {
	fake := newFakeES(map[string]fakeField{
		"project":   {typ: "text"},
		"phase":     {typ: "keyword"},
		"published": {typ: "date"},
		"count":     {typ: "long"},
		"nested":    {typ: "object"},
	})
	e := newMetaValueSpaceEngine(t, fake)

	fields, unaggregatable, err := e.metaAggFields(t.Context(), "ragflow_doc_meta_t1")
	if err != nil {
		t.Fatalf("metaAggFields: %v", err)
	}
	if !reflect.DeepEqual(unaggregatable, []string{"nested"}) {
		t.Errorf("unaggregatable: got %v, want [nested]", unaggregatable)
	}
	want := map[string]metaAggField{
		"project":   {path: "meta_fields.project.keyword", typ: "text"},
		"phase":     {path: "meta_fields.phase", typ: "keyword"},
		"published": {path: "meta_fields.published", typ: "date"},
		"count":     {path: "meta_fields.count", typ: "long"},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Errorf("fields: got %v, want %v", fields, want)
	}
}

func TestFormatMetaValue(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		typ  string
		want string
	}{
		{"string", `"alpha"`, "text", "alpha"},
		{"integer", `42`, "long", "42"},
		{"large integer keeps its digits", `9007199254740993`, "long", "9007199254740993"},
		{"float", `1.5`, "double", "1.5"},
		{"bool", `true`, "boolean", "true"},
		{"date at midnight", `1784764800000`, "date", "2026-07-23"},
		{"date with a time of day", `1784802600000`, "date", "2026-07-23T10:30:00Z"},
		{"date with milliseconds", `1784802600123`, "date", "2026-07-23T10:30:00.123000Z"},
		{"epoch millis on a non-date field are left alone", `1784764800000`, "long", "1784764800000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatMetaValue(json.RawMessage(tc.raw), tc.typ); got != tc.want {
				t.Errorf("formatMetaValue(%s, %s): got %q, want %q", tc.raw, tc.typ, got, tc.want)
			}
		})
	}
}
