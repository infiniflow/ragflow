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

package chunk

import (
	"context"
	"encoding/json"
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
	searchCalled  []string // records index names searched
	filterCapture map[string]interface{}
}

func (m *mockVectorFetcher) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if len(req.IndexNames) > 0 {
		m.searchCalled = append(m.searchCalled, req.IndexNames[0])
	}
	if m.filterCapture != nil {
		m.filterCapture = req.Filter
	}
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	if m.searchResults == nil {
		return &types.SearchResult{}, nil
	}
	if len(req.IndexNames) > 0 {
		if res, ok := m.searchResults[req.IndexNames[0]]; ok {
			return res, nil
		}
	}
	return &types.SearchResult{}, nil
}

func (m *mockVectorFetcher) GetType() string { return m.engineType }

var bg = context.Background()

// --- FetchChunkVectors tests ---

func TestFetchChunkVectors_EmptyInput(t *testing.T) {
	// nil chunkIDs
	mock := &mockVectorFetcher{engineType: "elasticsearch"}
	result := FetchChunkVectors(bg, mock, nil, []string{"t1"}, []string{"kb1"}, 1024)
	if len(result) != 0 {
		t.Errorf("expected empty map for nil chunkIDs, got %d entries", len(result))
	}
	if len(mock.searchCalled) > 0 {
		t.Error("Search should not be called with nil chunkIDs")
	}

	// Empty slice
	mock = &mockVectorFetcher{engineType: "elasticsearch"}
	result = FetchChunkVectors(bg, mock, []string{}, []string{"t1"}, []string{"kb1"}, 1024)
	if len(result) != 0 {
		t.Errorf("expected empty map for empty chunkIDs, got %d entries", len(result))
	}
}

func TestFetchChunkVectors_ZeroDimReturnsEmpty(t *testing.T) {
	mock := &mockVectorFetcher{engineType: "elasticsearch"}
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 0)
	if len(result) != 0 {
		t.Errorf("expected empty map for dim=0, got %d entries", len(result))
	}
	result = FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, -1)
	if len(result) != 0 {
		t.Errorf("expected empty map for dim=-1, got %d entries", len(result))
	}
}

func TestFetchChunkVectors_InfinitySkipsSearch(t *testing.T) {
	mock := &mockVectorFetcher{engineType: "infinity"}
	result := FetchChunkVectors(bg, mock, []string{"c1", "c2"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if len(mock.searchCalled) > 0 {
		t.Error("Search should not be called for Infinity engine")
	}
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c1"], zero) {
		t.Errorf("expected zero vector for c1, got %v", result["c1"])
	}
	// Verify independence.
	result["c1"][0] = 1.0
	if result["c2"][0] != 0.0 {
		t.Errorf("zero vectors should be independent; c2[0] = %v", result["c2"][0])
	}
}

func TestFetchChunkVectors_OceanbaseSkipsSearch(t *testing.T) {
	mock := &mockVectorFetcher{engineType: "oceanbase"}
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if len(mock.searchCalled) > 0 {
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
	result := FetchChunkVectors(bg, mock, []string{"c1", "c2"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if !reflect.DeepEqual(result["c1"], []float64{0.1, 0.2, 0.3}) {
		t.Errorf("c1 = %v, want [0.1 0.2 0.3]", result["c1"])
	}
	if !reflect.DeepEqual(result["c2"], []float64{0.4, 0.5, 0.6}) {
		t.Errorf("c2 = %v, want [0.4 0.5 0.6]", result["c2"])
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
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 2)
	if !reflect.DeepEqual(result["c1"], []float64{1.0, 2.0}) {
		t.Errorf("c1 = %v, want [1.0 2.0]", result["c1"])
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
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 2)
	if !reflect.DeepEqual(result["c1"], []float64{1.0, 2.0}) {
		t.Errorf("c1 = %v, want [1.0 2.0]", result["c1"])
	}
}

func TestFetchChunkVectors_JSONNumberVector(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_2_vec": []interface{}{
						json.Number("1.5"),
						json.Number("2.5"),
					}},
				},
			},
		},
	}
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 2)
	if !reflect.DeepEqual(result["c1"], []float64{1.5, 2.5}) {
		t.Errorf("c1 = %v, want [1.5 2.5]", result["c1"])
	}
}

func TestFetchChunkVectors_SearchErrorDegradesGracefully(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchErr:  errors.New("connection refused"),
	}
	result := FetchChunkVectors(bg, mock, []string{"c1", "c2"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c1"], zero) {
		t.Errorf("c1 should be zero on error, got %v", result["c1"])
	}
	if !reflect.DeepEqual(result["c2"], zero) {
		t.Errorf("c2 should be zero on error, got %v", result["c2"])
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
	result := FetchChunkVectors(bg, mock, []string{"c1", "c2"}, []string{"t1"}, []string{"kb1"}, 3)
	if !reflect.DeepEqual(result["c1"], []float64{0.1, 0.2, 0.3}) {
		t.Errorf("c1 = %v, want [0.1 0.2 0.3]", result["c1"])
	}
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c2"], zero) {
		t.Errorf("c2 should be zero, got %v", result["c2"])
	}
}

func TestFetchChunkVectors_WrongDimVectorReturnsZero(t *testing.T) {
	// String with wrong dim
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_3_vec": "0.1\t0.2"},
				},
			},
		},
	}
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 3)
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c1"], zero) {
		t.Errorf("expected zero for wrong-dim string, got %v", result["c1"])
	}

	// []float64 with wrong dim
	mock = &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_3_vec": []float64{0.1}},
				},
			},
		},
	}
	result = FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 3)
	if !reflect.DeepEqual(result["c1"], zero) {
		t.Errorf("expected zero for wrong-dim []float64, got %v", result["c1"])
	}
}

func TestFetchChunkVectors_ZeroVectorsAreIndependent(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchErr:  errors.New("search down"),
	}
	result := FetchChunkVectors(bg, mock, []string{"c1", "c2", "c3"}, []string{"t1"}, []string{"kb1"}, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}
	result["c1"][0] = 999.0
	if result["c2"][0] != 0.0 {
		t.Errorf("c2[0] = %v — zero vectors share backing array", result["c2"][0])
	}
	if result["c3"][0] != 0.0 {
		t.Errorf("c3[0] = %v — zero vectors share backing array", result["c3"][0])
	}
}

func TestFetchChunkVectors_DuplicateChunkID(t *testing.T) {
	// First result wins when the same chunk ID appears in multiple indices.
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_2_vec": "0.1\t0.2"},
				},
			},
			"ragflow_t2": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_2_vec": "9.9\t9.9"},
				},
			},
		},
	}
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1", "t2"}, []string{"kb1"}, 2)
	if !reflect.DeepEqual(result["c1"], []float64{0.1, 0.2}) {
		t.Errorf("first index should win: got %v, want [0.1 0.2]", result["c1"])
	}
}

func TestFetchChunkVectors_ChunkWithEmptyID(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "", "q_2_vec": "0.1\t0.2"},
					{"id": "c1", "q_2_vec": "0.3\t0.4"},
				},
			},
		},
	}
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, []string{"kb1"}, 2)
	if !reflect.DeepEqual(result["c1"], []float64{0.3, 0.4}) {
		t.Errorf("chunk with empty id should be skipped: got %v", result["c1"])
	}
}

func TestFetchChunkVectors_EmptyTenantIDs(t *testing.T) {
	mock := &mockVectorFetcher{engineType: "elasticsearch"}
	result := FetchChunkVectors(bg, mock, []string{"c1", "c2"}, nil, nil, 3)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	zero := make([]float64, 3)
	if !reflect.DeepEqual(result["c1"], zero) {
		t.Errorf("c1 should be zero for empty tenantIDs, got %v", result["c1"])
	}
	if !reflect.DeepEqual(result["c2"], zero) {
		t.Errorf("c2 should be zero for empty tenantIDs, got %v", result["c2"])
	}
}

func TestFetchChunkVectors_NilKbIDs(t *testing.T) {
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_2_vec": "0.1\t0.2"},
				},
			},
		},
	}
	result := FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, nil, 2)
	if !reflect.DeepEqual(result["c1"], []float64{0.1, 0.2}) {
		t.Errorf("c1 = %v, want [0.1 0.2]", result["c1"])
	}
}

func TestFetchChunkVectors_FilterIsSliceOfInterface(t *testing.T) {
	// Verify the ES filter uses []interface{} not []string (which would be silently dropped).
	mock := &mockVectorFetcher{
		engineType: "elasticsearch",
		searchResults: map[string]*types.SearchResult{
			"ragflow_t1": {
				Chunks: []map[string]interface{}{
					{"id": "c1", "q_2_vec": "0.1\t0.2"},
				},
			},
		},
	}
	// Verify filter type: FetchChunkVectors must convert []string to []interface{}.
	// The filterCapture field on the mock records the Filter from the SearchRequest.
	mock.filterCapture = make(map[string]interface{})
	FetchChunkVectors(bg, mock, []string{"c1"}, []string{"t1"}, nil, 2)

	if len(mock.searchCalled) == 0 {
		t.Error("Search should be called for ES engine")
	}
	idVal, ok := mock.filterCapture["id"]
	if !ok {
		t.Fatal("filter should contain 'id' key")
	}
	if _, ok := idVal.([]interface{}); !ok {
		t.Errorf("filter 'id' must be []interface{}, got %T", idVal)
	}
}

// --- parseVectorField tests ---

func TestParseVectorField_MissingField(t *testing.T) {
	result := parseVectorField(map[string]interface{}{"id": "c1"}, "q_3_vec", 3)
	if result != nil {
		t.Errorf("expected nil for missing field, got %v", result)
	}
}

func TestParseVectorField_EmptyString(t *testing.T) {
	result := parseVectorField(map[string]interface{}{"q_3_vec": ""}, "q_3_vec", 3)
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

func TestParseVectorField_UnsupportedType(t *testing.T) {
	result := parseVectorField(map[string]interface{}{"q_3_vec": 12345}, "q_3_vec", 3)
	if result != nil {
		t.Errorf("expected nil for unsupported type (int), got %v", result)
	}
	result = parseVectorField(map[string]interface{}{"q_3_vec": true}, "q_3_vec", 3)
	if result != nil {
		t.Errorf("expected nil for unsupported type (bool), got %v", result)
	}
}

func TestParseVectorField_Float32Vector(t *testing.T) {
	result := parseVectorField(
		map[string]interface{}{"q_2_vec": []interface{}{float32(1.5), float32(2.5)}},
		"q_2_vec", 2)
	if result == nil {
		t.Fatal("expected non-nil for float32 vector")
	}
	if result[0] != 1.5 || result[1] != 2.5 {
		t.Errorf("got %v, want [1.5 2.5]", result)
	}
}

func TestParseVectorField_InterfaceSliceWithStrings(t *testing.T) {
	result := parseVectorField(
		map[string]interface{}{"q_2_vec": []interface{}{"1.5", "2.5"}},
		"q_2_vec", 2)
	if result == nil {
		t.Fatal("expected non-nil for string elements")
	}
	if !reflect.DeepEqual(result, []float64{1.5, 2.5}) {
		t.Errorf("got %v, want [1.5 2.5]", result)
	}
}

func TestParseVectorField_InterfaceSliceTooShort(t *testing.T) {
	result := parseVectorField(
		map[string]interface{}{"q_3_vec": []interface{}{float64(1.0)}},
		"q_3_vec", 3)
	if result != nil {
		t.Errorf("expected nil for too-short []interface{}, got %v", result)
	}
}

func TestParseVectorField_Float64SliceIsIndependent(t *testing.T) {
	original := []float64{1.0, 2.0, 3.0}
	chunk := map[string]interface{}{"q_3_vec": original}
	result := parseVectorField(chunk, "q_3_vec", 3)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	result[0] = 999.0
	if original[0] != 1.0 {
		t.Errorf("original[0] = %v — returned slice aliases chunk data", original[0])
	}
}

// --- parseVectorString tests ---

func TestParseVectorString_InvalidFloat(t *testing.T) {
	if result := parseVectorString("0.1\tnot_a_number", 2); result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseVectorString_WithSpaces(t *testing.T) {
	result := parseVectorString(" 0.1 \t 0.2 ", 2)
	if !reflect.DeepEqual(result, []float64{0.1, 0.2}) {
		t.Errorf("got %v, want [0.1 0.2]", result)
	}
}

func TestParseVectorString_SingleElement(t *testing.T) {
	result := parseVectorString("3.14", 1)
	if !reflect.DeepEqual(result, []float64{3.14}) {
		t.Errorf("got %v, want [3.14]", result)
	}
}

func TestParseVectorString_TrailingTab(t *testing.T) {
	result := parseVectorString("0.1\t0.2\t", 3)
	if result != nil {
		t.Errorf("expected nil for trailing tab (empty element is invalid float), got %v", result)
	}
	result = parseVectorString("0.1\t0.2", 2)
	if !reflect.DeepEqual(result, []float64{0.1, 0.2}) {
		t.Errorf("got %v, want [0.1 0.2]", result)
	}
}
