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

package transaction

import "testing"

func TestDocumentQueryCircuitOpensOnlyForEmptyAppliedIDFilter(t *testing.T) {
	tests := []struct {
		name               string
		docIDFilterApplied bool
		docIDs             []string
		wantCircuitOpen    bool
	}{
		{
			name:            "filter not applied keeps every document reachable",
			docIDs:          []string{"doc-1"},
			wantCircuitOpen: false,
		},
		{
			name:               "applied filter with an id selects that document",
			docIDFilterApplied: true,
			docIDs:             []string{"doc-1"},
			wantCircuitOpen:    false,
		},
		{
			name:               "applied filter with an empty slice selects nothing",
			docIDFilterApplied: true,
			docIDs:             []string{},
			wantCircuitOpen:    true,
		},
		{
			name:               "applied filter with no ids selects nothing",
			docIDFilterApplied: true,
			wantCircuitOpen:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			circuit := NewDocumentQueryCircuit(tt.docIDFilterApplied, tt.docIDs)
			if got := circuit.IsCircuitOpen(); got != tt.wantCircuitOpen {
				t.Fatalf("IsCircuitOpen() = %v, want %v", got, tt.wantCircuitOpen)
			}
		})
	}
}

func TestCircuitOpenFiltersKeepsBucketTypes(t *testing.T) {
	circuit := NewDocumentQueryCircuit(true, nil)
	filters := circuit.CircuitOpenFilters()

	if _, ok := filters["suffix"].(map[string]int64); !ok {
		t.Fatalf("suffix bucket type = %T, want map[string]int64", filters["suffix"])
	}
	if _, ok := filters["ingestion_status"].(map[string]int64); !ok {
		t.Fatalf("ingestion_status bucket type = %T, want map[string]int64", filters["ingestion_status"])
	}
	if _, ok := filters["metadata"].(map[string]interface{}); !ok {
		t.Fatalf("metadata bucket type = %T, want map[string]interface{}", filters["metadata"])
	}
	if len(filters) != 3 {
		t.Fatalf("filter keys = %v, want exactly suffix/ingestion_status/metadata", filters)
	}
}
