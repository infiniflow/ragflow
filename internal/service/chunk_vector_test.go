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
	"reflect"
	"testing"

	"ragflow/internal/engine/types"
)

// mockVectorFetcher implements vectorFetcher for testing.
type mockVectorFetcher struct {
	searchResults map[string]*types.SearchResult // keyed by index name
	searchErr     error
	engineType    string
	searchCalled  bool
}

func (m *mockVectorFetcher) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	m.searchCalled = true
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	if m.searchResults == nil {
		return &types.SearchResult{}, nil
	}
	// Match by index name.
	if len(req.IndexNames) > 0 {
		if res, ok := m.searchResults[req.IndexNames[0]]; ok {
			return res, nil
		}
	}
	return &types.SearchResult{}, nil
}

func (m *mockVectorFetcher) GetType() string { return m.engineType }

func TestFetchChunkVectors_EmptyInput(t *testing.T) {
	mock := &mockVectorFetcher{engineType: "elasticsearch"}
	result := FetchChunkVectors(mock, nil, []string{"t1"}, []string{"kb1"}, 1024)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
	if mock.searchCalled {
		t.Error("Search should not be called with empty chunkIDs")
	}
}

func TestFetchChunkVectors_InfinitySkipsSearch(t *testing.T) {
	mock := &mockVectorFetcher{engineType: "infinity"}
	result := FetchChunkVectors(mock, []string{"c1", "c2"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if mock.searchCalled {
		t.Error("Search should not be called for Infinity engine")
	}
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c1"], zero) {
		t.Errorf("expected zero vector for c1, got %v", result["c1"])
	}
}

func TestFetchChunkVectors_OceanbaseSkipsSearch(t *testing.T) {
	mock := &mockVectorFetcher{engineType: "oceanbase"}
	result := FetchChunkVectors(mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if mock.searchCalled {
		t.Error("Search should not be called for OceanBase engine")
	}
}

func TestFetchChunkVectors_ESStringVector(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_3_vec": "0.1\t0.2\t0.3"},
					{"id": "c2", "q_3_vec": "0.4\t0.5\t0.6"},
				},
			},
		},
	}
	result := FetchChunkVectors(mock, []string{"c1", "c2"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	expected1 := []float64{0.1, 0.2, 0.3}
	if !reflect.DeepEqual(result["c1"], expected1) {
		t.Errorf("c1 = %v, want %v", result["c1"], expected1)
	}
	expected2 := []float64{0.4, 0.5, 0.6}
	if !reflect.DeepEqual(result["c2"], expected2) {
		t.Errorf("c2 = %v, want %v", result["c2"], expected2)
	}
}

func TestFetchChunkVectors_ESFloatSliceVector(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_2_vec": []float64{1.0, 2.0}},
				},
			},
		},
	}
	result := FetchChunkVectors(mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 2)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	expected := []float64{1.0, 2.0}
	if !reflect.DeepEqual(result["c1"], expected) {
		t.Errorf("c1 = %v, want %v", result["c1"], expected)
	}
}

func TestFetchChunkVectors_ESInterfaceSliceVector(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_2_vec": []interface{}{float64(1.0), float64(2.0)}},
				},
			},
		},
	}
	result := FetchChunkVectors(mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 2)
	expected := []float64{1.0, 2.0}
	if !reflect.DeepEqual(result["c1"], expected) {
		t.Errorf("c1 = %v, want %v", result["c1"], expected)
	}
}

func TestFetchChunkVectors_SearchErrorDegradesGracefully(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchErr:  errors.New("connection refused"),
	}
	result := FetchChunkVectors(mock, []string{"c1", "c2"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c1"], zero) {
		t.Errorf("expected zero vector for c1 on error, got %v", result["c1"])
	}
	if !reflect.DeepEqual(result["c2"], zero) {
		t.Errorf("expected zero vector for c2 on error, got %v", result["c2"])
	}
}

func TestFetchChunkVectors_MissingChunkGetsZero(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_3_vec": "0.1\t0.2\t0.3"},
				},
			},
		},
	}
	// Request c1 and c2, but only c1 matches.
	result := FetchChunkVectors(mock, []string{"c1", "c2"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	expected := []float64{0.1, 0.2, 0.3}
	if !reflect.DeepEqual(result["c1"], expected) {
		t.Errorf("c1 = %v, want %v", result["c1"], expected)
	}
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c2"], zero) {
		t.Errorf("c2 should be zero, got %v", result["c2"])
	}
}

func TestFetchChunkVectors_WrongDimVectorReturnsZero(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					// String with wrong number of components.
					{"id": "c1", "q_3_vec": "0.1\t0.2"},
				},
			},
		},
	}
	result := FetchChunkVectors(mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 3)
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c1"], zero) {
		t.Errorf("expected zero vector for wrong-dim input, got %v", result["c1"])
	}
}

func TestParseVectorField_MissingField(t *testing.T) {
	chunk := map[string]interface{}{"id": "c1"}
	zero := make([]float64, 3)
	result := parseVectorField(chunk, "q_3_vec", 3, zero)
	if !reflect.DeepEqual(result, zero) {
		t.Errorf("expected zero, got %v", result)
	}
}

func TestParseVectorString_InvalidFloat(t *testing.T) {
	zero := make([]float64, 2)
	result := parseVectorString("0.1\tnot_a_number", 2, zero)
	if !reflect.DeepEqual(result, zero) {
		t.Errorf("expected zero, got %v", result)
	}
}

func TestParseVectorString_WithSpaces(t *testing.T) {
	result := parseVectorString(" 0.1 \t 0.2 ", 2, nil)
	expected := []float64{0.1, 0.2}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}
