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

package document

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/service"
)

func stubMetadataFacets(t *testing.T, fn func(ctx context.Context, metadataSvc *service.MetadataService, kbID string, docIDs []string) ([]types.MetaValueCount, int64, bool, error)) {
	t.Helper()
	orig := metadataFacets
	metadataFacets = fn
	t.Cleanup(func() { metadataFacets = orig })
}

// When the doc store aggregates the facet, the file list's metadata filter is
// built from it alone -- no document's metadata is read -- with every value
// labelled the way GetMetadataByKBs keys it, so a value picked in the filter
// resolves to the documents counted for it. The empty-metadata bucket is the
// documents in scope that hold no value at all.
func TestGetDocumentMetadataFilter_UsesTheAggregatedFacet(t *testing.T) {
	var gotKB string
	var gotIDs []string
	stubMetadataFacets(t, func(_ context.Context, _ *service.MetadataService, kbID string, docIDs []string) ([]types.MetaValueCount, int64, bool, error) {
		gotKB, gotIDs = kbID, docIDs
		return []types.MetaValueCount{
			{Key: "project", Value: "alpha", Docs: 2},
			{Key: "project", Value: "beta", Docs: 1},
			{Key: "price", Value: float64(1234567), Docs: 1},
			{Key: "draft", Value: true, Docs: 1},
			// A number and the same digits as a string share a label, as they
			// do in the scan.
			{Key: "year", Value: float64(2024), Docs: 1},
			{Key: "year", Value: "2024", Docs: 1},
		}, 3, true, nil
	})
	// metadataSvc is nil: reaching the document scan would panic.
	s := &DocumentService{}

	docIDs := []string{"d1", "d2", "d3", "d4", "d5"}
	got, err := s.getDocumentMetadataFilter(t.Context(), "kb1", docIDs)
	if err != nil {
		t.Fatalf("getDocumentMetadataFilter: %v", err)
	}
	want := map[string]interface{}{
		"project":        map[string]int64{"alpha": 2, "beta": 1},
		"price":          map[string]int64{metadataValueLabel(float64(1234567)): 1},
		"draft":          map[string]int64{"true": 1},
		"year":           map[string]int64{"2024": 2},
		"empty_metadata": map[string]int64{"true": 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter: got %v, want %v", got, want)
	}
	if gotKB != "kb1" || !reflect.DeepEqual(gotIDs, docIDs) {
		t.Errorf("facet scoped to %q %v, want kb1 %v (the documents the list filters select)", gotKB, gotIDs, docIDs)
	}
}

// A doc store that fails is reported, not answered with a scan of the same
// store.
func TestGetDocumentMetadataFilter_ReturnsStoreErrors(t *testing.T) {
	unreachable := errors.New("connection refused")
	stubMetadataFacets(t, func(context.Context, *service.MetadataService, string, []string) ([]types.MetaValueCount, int64, bool, error) {
		return nil, 0, false, unreachable
	})

	if _, err := (&DocumentService{}).getDocumentMetadataFilter(t.Context(), "kb1", []string{"d1"}); !errors.Is(err, unreachable) {
		t.Fatalf("err = %v, want %v", err, unreachable)
	}
}
