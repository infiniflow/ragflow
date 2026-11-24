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

package nats

import (
	"testing"

	"ragflow/internal/common"
)

// BenchmarkIngestorLaneSelection records the per-call cost and allocation
// profile of resolving an ingestor tier to its allowed lanes. The shared
// drain helper introduced for both pull entry points must not add allocations
// to this selection step.
func BenchmarkIngestorLaneSelection(b *testing.B) {
	for i := 0; i < b.N; i++ {
		lanes, err := ingestorLanes(common.TierDefault)
		if err != nil || len(lanes) != 1 {
			b.Fatalf("ingestorLanes: %v, %v", lanes, err)
		}
	}
}
