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
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"ragflow/internal/engine/types"
)

// facetCounts flattens MetaFacets' answer for comparison.
func facetCounts(facets []types.MetaValueCount) map[string]int64 {
	out := map[string]int64{}
	for _, f := range facets {
		out[fmt.Sprintf("%s=%T:%v", f.Key, f.Value, f.Value)] += f.Docs
	}
	return out
}

// Counts are per document and per distinct value; a document carrying any
// value counts once towards carrying, and the keys the file list never shows
// are left out.
func TestMetaFacets_CountsDocumentsInScope(t *testing.T) {
	fake := &fakeMetaES{docs: []fakeMetaDoc{
		metaDoc(t, "doc-1", "kb-1", `{"author": "Ann", "tags": ["red", "blue"], "price": 1234567, "id": "doc-1"}`),
		metaDoc(t, "doc-2", "kb-1", `{"author": ["Ann", "Bob"], "tags": "red", "nested": {"x": 1}}`),
		metaDoc(t, "doc-3", "kb-1", `{}`),
		metaDoc(t, "doc-4", "kb-1", `{"kb_id": "kb-1"}`),
		// Not listed, or another dataset's: never counted.
		metaDoc(t, "orphan", "kb-1", `{"author": "Zed"}`),
		metaDoc(t, "doc-5", "kb-2", `{"author": "Eve"}`),
	}}

	facets, carrying, err := newMetaValuesEngine(t, fake).MetaFacets(t.Context(), "tenant-1", "kb-1", []string{"doc-1", "doc-2", "doc-3", "doc-4", "doc-5"})
	if err != nil {
		t.Fatalf("MetaFacets: %v", err)
	}
	want := map[string]int64{
		"author=string:Ann":          2,
		"author=string:Bob":          1,
		"tags=string:red":            2,
		"tags=string:blue":           1,
		"price=float64:1.234567e+06": 1,
	}
	if got := facetCounts(facets); !reflect.DeepEqual(got, want) {
		t.Errorf("facets:\n got %v\nwant %v", got, want)
	}
	// doc-1 and doc-2; doc-2's nested object counts as metadata, doc-4 only
	// holds a skipped key.
	if carrying != 2 {
		t.Errorf("carrying = %d, want 2", carrying)
	}
	if got, want := fake.skipParams[0], metaFacetSkipKeys; !reflect.DeepEqual(got, want) {
		t.Errorf("skipped keys = %v, want %v", got, want)
	}
}

// The ids scope goes in batches within index.max_terms_count, and the counts
// add up across them.
func TestMetaFacets_BatchesTheIDScope(t *testing.T) {
	n := metaFacetIDBatch + 10
	docIDs := make([]string, 0, n)
	fake := &fakeMetaES{}
	for i := range n {
		id := fmt.Sprintf("doc-%06d", i)
		docIDs = append(docIDs, id)
		if i%1000 == 0 {
			fake.docs = append(fake.docs, metaDoc(t, id, "kb-1", `{"author": "Ann"}`))
		}
	}

	facets, carrying, err := newMetaValuesEngine(t, fake).MetaFacets(t.Context(), "tenant-1", "kb-1", docIDs)
	if err != nil {
		t.Fatalf("MetaFacets: %v", err)
	}
	if got := facetCounts(facets)["author=string:Ann"]; got != int64(len(fake.docs)) {
		t.Errorf("author=Ann counted %d, want %d", got, len(fake.docs))
	}
	if carrying != int64(len(fake.docs)) {
		t.Errorf("carrying = %d, want %d", carrying, len(fake.docs))
	}
	batches := append([]int(nil), fake.idBatches...)
	sort.Ints(batches)
	if want := []int{10, metaFacetIDBatch}; !reflect.DeepEqual(batches, want) {
		t.Errorf("id batches = %v, want %v", batches, want)
	}
}

func TestMetaFacets_RefusesPartialAnswers(t *testing.T) {
	fake := &fakeMetaES{docs: []fakeMetaDoc{metaDoc(t, "doc-1", "kb-1", `{"author": "Ann"}`)}, failedShards: 1}

	_, _, err := newMetaValuesEngine(t, fake).MetaFacets(t.Context(), "tenant-1", "kb-1", []string{"doc-1"})
	if !errors.Is(err, types.ErrMetaValueSpaceIncomplete) {
		t.Fatalf("err = %v, want ErrMetaValueSpaceIncomplete", err)
	}
}

func TestMetaFacets_NothingToCount(t *testing.T) {
	fake := &fakeMetaES{docs: []fakeMetaDoc{metaDoc(t, "doc-1", "kb-1", `{"author": "Ann"}`)}}
	eng := newMetaValuesEngine(t, fake)

	facets, carrying, err := eng.MetaFacets(t.Context(), "tenant-1", "kb-1", nil)
	if err != nil || len(facets) != 0 || carrying != 0 || fake.searches != 0 {
		t.Errorf("no ids: got %v, %d, %v after %d searches; want nothing and no search", facets, carrying, err, fake.searches)
	}

	fake.noIndex = true
	facets, carrying, err = eng.MetaFacets(t.Context(), "tenant-1", "kb-1", []string{"doc-1"})
	if err != nil || len(facets) != 0 || carrying != 0 || fake.searches != 0 {
		t.Errorf("no index: got %v, %d, %v after %d searches; want nothing and no search", facets, carrying, err, fake.searches)
	}
}
