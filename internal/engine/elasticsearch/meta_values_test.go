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

// fakeMetaDoc is one doc-meta row: its _id, kb_id and meta_fields as stored.
type fakeMetaDoc struct {
	id     string
	kbID   string
	fields interface{}
}

// fakeMetaES holds doc-meta rows and answers the scripted composite
// aggregation the way Elasticsearch does: it applies the query's kb_id and ids
// scope, turns each row into the script's tokens (a Go rendition of
// metaValuesScript -- the integration test checks the real one), counts rows
// per distinct token and pages the sorted buckets from after_key.
type fakeMetaES struct {
	mu   sync.Mutex
	docs []fakeMetaDoc

	// Recorded for assertions.
	searches     int
	partialParam []string
	skipParams   [][]string
	idBatches    []int

	// Injected for every response.
	timedOut     bool
	failedShards int
	dropAgg      bool
	noIndex      bool
}

type fakeMetaSearch struct {
	Query struct {
		Bool struct {
			Filter []map[string]map[string]interface{} `json:"filter"`
		} `json:"bool"`
	} `json:"query"`
	Aggs map[string]struct {
		Composite struct {
			Size    int               `json:"size"`
			After   map[string]string `json:"after"`
			Sources []map[string]struct {
				Terms struct {
					Script struct {
						Params struct {
							Skip []string `json:"skip"`
						} `json:"params"`
					} `json:"script"`
				} `json:"terms"`
			} `json:"sources"`
		} `json:"composite"`
	} `json:"aggs"`
}

func (f *fakeMetaES) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method == http.MethodHead {
			if f.noIndex {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/_search") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body fakeMetaSearch
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode search body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		f.mu.Lock()
		defer f.mu.Unlock()
		f.searches++
		f.partialParam = append(f.partialParam, r.URL.Query().Get("allow_partial_search_results"))

		agg, ok := body.Aggs[metaValuesAgg]
		if !ok || len(agg.Composite.Sources) != 1 {
			t.Errorf("search carries no single-source %s composite: %s", metaValuesAgg, raw)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		skip := agg.Composite.Sources[0]["v"].Terms.Script.Params.Skip
		f.skipParams = append(f.skipParams, skip)
		for _, clause := range body.Query.Bool.Filter {
			if ids, ok := clause["ids"]; ok {
				f.idBatches = append(f.idBatches, len(toStrings(ids["values"])))
			}
		}

		counts := map[string]int64{}
		for _, doc := range f.docs {
			if !f.inScope(doc, body) {
				continue
			}
			for _, token := range fakeMetaTokens(doc.fields, skip) {
				counts[token]++
			}
		}
		tokens := make([]string, 0, len(counts))
		for token := range counts {
			if after := agg.Composite.After["v"]; after != "" && token <= after {
				continue
			}
			tokens = append(tokens, token)
		}
		sort.Strings(tokens)
		if len(tokens) > agg.Composite.Size {
			tokens = tokens[:agg.Composite.Size]
		}
		buckets := make([]interface{}, 0, len(tokens))
		for _, token := range tokens {
			buckets = append(buckets, map[string]interface{}{"key": map[string]string{"v": token}, "doc_count": counts[token]})
		}
		aggregation := map[string]interface{}{"buckets": buckets}
		if len(tokens) > 0 {
			aggregation["after_key"] = map[string]string{"v": tokens[len(tokens)-1]}
		}
		aggs := map[string]interface{}{metaValuesAgg: aggregation}
		if f.dropAgg {
			aggs = map[string]interface{}{}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"timed_out":    f.timedOut,
			"_shards":      map[string]int{"total": 2, "successful": 2 - f.failedShards, "failed": f.failedShards},
			"aggregations": aggs,
		})
	}
}

// inScope applies the query's kb_id terms/term and ids filters.
func (f *fakeMetaES) inScope(doc fakeMetaDoc, body fakeMetaSearch) bool {
	for _, clause := range body.Query.Bool.Filter {
		for kind, spec := range clause {
			switch kind {
			case "terms":
				if !containsString(toStrings(spec["kb_id"]), doc.kbID) {
					return false
				}
			case "term":
				if spec["kb_id"] != doc.kbID {
					return false
				}
			case "ids":
				if !containsString(toStrings(spec["values"]), doc.id) {
					return false
				}
			}
		}
	}
	return true
}

func toStrings(v interface{}) []string {
	list, _ := v.([]interface{})
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// fakeMetaTokens is metaValuesScript in Go.
func fakeMetaTokens(fields interface{}, skip []string) []string {
	if fields == nil {
		return nil
	}
	m, ok := fields.(map[string]interface{})
	if !ok {
		return []string{"u"}
	}
	seen := map[string]bool{}
	var out []string
	add := func(token string) {
		if !seen[token] {
			seen[token] = true
			out = append(out, token)
		}
	}
	carrying := false
	for key, value := range m {
		if containsString(skip, key) || value == nil {
			continue
		}
		items := []interface{}{value}
		if list, ok := value.([]interface{}); ok {
			items = list
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			carrying = true
			prefix := "k" + key + "\x00"
			switch v := item.(type) {
			case string:
				add(prefix + "s" + v)
			case bool:
				add(prefix + "b" + strconv.FormatBool(v))
			case float64:
				add(prefix + "n" + strconv.FormatFloat(v, 'E', -1, 64))
			}
		}
	}
	if carrying {
		add("c")
	}
	return out
}

func newMetaValuesEngine(t *testing.T, fake *fakeMetaES) *Engine {
	t.Helper()
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)
	return newTestEngine(t, srv.URL)
}

// metaDoc decodes meta_fields from JSON, so numbers arrive as float64 exactly
// as they do from a real _source.
func metaDoc(t *testing.T, id, kbID, fieldsJSON string) fakeMetaDoc {
	t.Helper()
	var fields interface{}
	if fieldsJSON != "" {
		if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
			t.Fatalf("decode %s: %v", fieldsJSON, err)
		}
	}
	return fakeMetaDoc{id: id, kbID: kbID, fields: fields}
}

func sortedValues(values []interface{}) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprintf("%T:%v", v, v))
	}
	sort.Strings(out)
	return out
}

// Values come back typed the way encoding/json decodes the stored value, and a
// list contributes its elements; nested objects and lists contribute nothing.
func TestMetaValueSpace_TypedValuesFromSource(t *testing.T) {
	fake := &fakeMetaES{docs: []fakeMetaDoc{
		metaDoc(t, "doc-1", "kb-1", `{"author": "Ann", "price": 1234567, "score": 0.1, "tags": ["red", "blue"], "draft": true}`),
		metaDoc(t, "doc-2", "kb-1", `{"author": ["Ann", "Bob"], "price": "1234567", "nested": {"x": 1}, "pairs": [["a", "b"]], "gone": null}`),
	}}

	space, err := newMetaValuesEngine(t, fake).MetaValueSpace(t.Context(), "tenant-1", []string{"kb-1"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	got := map[string][]string{}
	for key, values := range space {
		got[key] = sortedValues(values)
	}
	want := map[string][]string{
		"author": {"string:Ann", "string:Bob"},
		"price":  {"float64:1.234567e+06", "string:1234567"},
		"score":  {"float64:0.1"},
		"tags":   {"string:blue", "string:red"},
		"draft":  {"bool:true"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("value space:\n got %v\nwant %v", got, want)
	}
	if fake.partialParam[0] != "false" {
		t.Errorf("allow_partial_search_results = %q, want false", fake.partialParam[0])
	}
	if len(fake.skipParams[0]) != 0 {
		t.Errorf("the value space skipped keys %v, want none", fake.skipParams[0])
	}
}

func TestMetaValueSpace_ScopedToTheKnowledgeBases(t *testing.T) {
	fake := &fakeMetaES{docs: []fakeMetaDoc{
		metaDoc(t, "doc-1", "kb-1", `{"author": "Ann"}`),
		metaDoc(t, "doc-2", "kb-2", `{"author": "Bob"}`),
		metaDoc(t, "doc-3", "kb-3", `{"author": "Eve"}`),
	}}

	space, err := newMetaValuesEngine(t, fake).MetaValueSpace(t.Context(), "tenant-1", []string{"kb-1", "kb-2"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	if got, want := sortedValues(space["author"]), []string{"string:Ann", "string:Bob"}; !reflect.DeepEqual(got, want) {
		t.Errorf("author values = %v, want %v", got, want)
	}
}

// One page stops at metaValuesPageSize buckets; stopping there would drop the
// rest silently.
func TestMetaValueSpace_PagesPastOnePage(t *testing.T) {
	values := make([]string, 0, metaValuesPageSize+5)
	for i := range metaValuesPageSize + 5 {
		values = append(values, fmt.Sprintf("%q", fmt.Sprintf("v%06d", i)))
	}
	fake := &fakeMetaES{docs: []fakeMetaDoc{
		metaDoc(t, "doc-1", "kb-1", `{"project": [`+strings.Join(values, ",")+`]}`),
	}}

	space, err := newMetaValuesEngine(t, fake).MetaValueSpace(t.Context(), "tenant-1", []string{"kb-1"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	if got := len(space["project"]); got != metaValuesPageSize+5 {
		t.Errorf("project has %d values, want %d", got, metaValuesPageSize+5)
	}
	if fake.searches != 2 {
		t.Errorf("searches = %d, want 2", fake.searches)
	}
}

func TestMetaValueSpace_RefusesPartialAnswers(t *testing.T) {
	for name, inject := range map[string]func(*fakeMetaES){
		"timed out":          func(f *fakeMetaES) { f.timedOut = true },
		"failed shard":       func(f *fakeMetaES) { f.failedShards = 1 },
		"absent aggregation": func(f *fakeMetaES) { f.dropAgg = true },
		// The script cannot read meta_fields stored as a JSON string.
		"unreadable meta_fields": func(f *fakeMetaES) { f.docs[0].fields = `{"author": "Ann"}` },
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeMetaES{docs: []fakeMetaDoc{metaDoc(t, "doc-1", "kb-1", `{"author": "Ann"}`)}}
			inject(fake)

			_, err := newMetaValuesEngine(t, fake).MetaValueSpace(t.Context(), "tenant-1", []string{"kb-1"})
			if !errors.Is(err, types.ErrMetaValueSpaceIncomplete) {
				t.Fatalf("err = %v, want ErrMetaValueSpaceIncomplete", err)
			}
		})
	}
}

func TestMetaValueSpace_MissingIndexIsEmpty(t *testing.T) {
	fake := &fakeMetaES{noIndex: true}

	space, err := newMetaValuesEngine(t, fake).MetaValueSpace(t.Context(), "tenant-1", []string{"kb-1"})
	if err != nil {
		t.Fatalf("MetaValueSpace: %v", err)
	}
	if len(space) != 0 || fake.searches != 0 {
		t.Errorf("space = %v after %d searches, want empty and none", space, fake.searches)
	}
}

func TestDecodeMetaValueToken(t *testing.T) {
	cases := []struct {
		token string
		key   string
		value interface{}
	}{
		{"author\x00sAnn", "author", "Ann"},
		{"author\x00s", "author", ""},
		{"draft\x00btrue", "draft", true},
		{"draft\x00bfalse", "draft", false},
		// Java's Double.toString forms.
		{"price\x00n1234567.0", "price", float64(1234567)},
		{"tiny\x00n1.0E-7", "tiny", 1e-7},
		{"big\x00n9.007199254740992E15", "big", float64(9007199254740992)},
	}
	for _, tc := range cases {
		key, value, err := decodeMetaValueToken(tc.token)
		if err != nil || key != tc.key || value != tc.value {
			t.Errorf("decodeMetaValueToken(%q) = (%q, %#v, %v), want (%q, %#v)", tc.token, key, value, err, tc.key, tc.value)
		}
	}
	for _, bad := range []string{"author", "author\x00", "author\x00x1", "price\x00nabc"} {
		if _, _, err := decodeMetaValueToken(bad); err == nil {
			t.Errorf("decodeMetaValueToken(%q) accepted a malformed token", bad)
		}
	}
}
