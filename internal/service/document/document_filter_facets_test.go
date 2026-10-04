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
	"reflect"
	"testing"

	"ragflow/internal/service"
)

func stubMetadataFacets(t *testing.T, fn func(ctx context.Context, metadataSvc *service.MetadataService, kbID string, docIDs []string) (map[string]map[string]int64, int64, bool)) {
	t.Helper()
	orig := metadataFacets
	metadataFacets = fn
	t.Cleanup(func() { metadataFacets = orig })
}

// When the doc store aggregates the facet, the file list's metadata filter is
// built from it alone -- no document's metadata is read -- and the
// empty-metadata bucket is the documents in scope that hold no value at all.
func TestGetDocumentMetadataFilter_UsesTheAggregatedFacet(t *testing.T) {
	var gotKB string
	var gotIDs []string
	stubMetadataFacets(t, func(_ context.Context, _ *service.MetadataService, kbID string, docIDs []string) (map[string]map[string]int64, int64, bool) {
		gotKB, gotIDs = kbID, docIDs
		return map[string]map[string]int64{
			"project": {"alpha": 2, "beta": 1},
			"unused":  {},
		}, 2, true
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
		"empty_metadata": map[string]int64{"true": 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter: got %v, want %v", got, want)
	}
	if gotKB != "kb1" || !reflect.DeepEqual(gotIDs, docIDs) {
		t.Errorf("facet scoped to %q %v, want kb1 %v (the documents the list filters select)", gotKB, gotIDs, docIDs)
	}
}
