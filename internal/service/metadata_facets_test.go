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

package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ragflow/internal/engine/types"
)

// facetingDocEngine answers MetaFacets with a fixed result.
type facetingDocEngine struct {
	fakeChatDocEngine
	facets   []types.MetaValueCount
	carrying int64
	err      error

	gotTenantID string
	gotKBID     string
	gotDocIDs   []string
}

func (e *facetingDocEngine) MetaFacets(_ context.Context, tenantID, kbID string, docIDs []string) ([]types.MetaValueCount, int64, error) {
	e.gotTenantID, e.gotKBID, e.gotDocIDs = tenantID, kbID, docIDs
	return e.facets, e.carrying, e.err
}

func TestGetMetadataFacets_UsesTheAggregation(t *testing.T) {
	seedValueSpaceKB(t)
	docEngine := &facetingDocEngine{facets: []types.MetaValueCount{{Key: "author", Value: "Ann", Docs: 2}}, carrying: 2}

	facets, carrying, ok, err := NewMetadataServiceForTest(nil, docEngine).GetMetadataFacets(t.Context(), "kb-1", []string{"doc-1", "doc-2"})
	if err != nil || !ok {
		t.Fatalf("GetMetadataFacets: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(facets, docEngine.facets) || carrying != 2 {
		t.Errorf("got %v, %d; want %v, 2", facets, carrying, docEngine.facets)
	}
	if docEngine.gotTenantID != "tenant-1" || docEngine.gotKBID != "kb-1" || !reflect.DeepEqual(docEngine.gotDocIDs, []string{"doc-1", "doc-2"}) {
		t.Errorf("scoped to %q %q %v", docEngine.gotTenantID, docEngine.gotKBID, docEngine.gotDocIDs)
	}
}

// Only an answer the store cannot give exactly sends the caller to count the
// documents itself; a store that cannot be reached would fail that scan too.
func TestGetMetadataFacets_FallsBackOnlyWhenIncomplete(t *testing.T) {
	seedValueSpaceKB(t)

	incomplete := &facetingDocEngine{err: fmt.Errorf("%w: failed shard", types.ErrMetaValueSpaceIncomplete)}
	_, _, ok, err := NewMetadataServiceForTest(nil, incomplete).GetMetadataFacets(t.Context(), "kb-1", []string{"doc-1"})
	if ok || err != nil {
		t.Errorf("incomplete: ok=%v err=%v, want the scan fallback (false, nil)", ok, err)
	}

	unreachable := errors.New("dial tcp: connection refused")
	_, _, ok, err = NewMetadataServiceForTest(nil, &facetingDocEngine{err: unreachable}).GetMetadataFacets(t.Context(), "kb-1", []string{"doc-1"})
	if ok || !errors.Is(err, unreachable) {
		t.Errorf("unreachable: ok=%v err=%v, want the error returned", ok, err)
	}
}

func TestGetMetadataFacets_NoAggregationPath(t *testing.T) {
	_, _, ok, err := NewMetadataServiceForTest(nil, &cappedScanDocEngine{}).GetMetadataFacets(t.Context(), "kb-1", []string{"doc-1"})
	if ok || err != nil {
		t.Errorf("ok=%v err=%v, want the scan fallback (false, nil)", ok, err)
	}
}
