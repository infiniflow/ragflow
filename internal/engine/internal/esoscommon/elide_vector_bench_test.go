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

package esoscommon

import "testing"

// BenchmarkElideQueryVector guards the hot path (the per-vector-search log
// call): eliding a named dense-vector field must stay cheap and allocation-free
// after the regexes are precomputed at init. Both engine field tokens are
// exercised since each engine calls this with its own constant.
func BenchmarkElideQueryVector(b *testing.B) {
	bodies := map[string][]byte{
		QueryVectorFieldName: []byte(`{"knn":{"query_vector":[0.1,0.2,0.3,0.4,0.5],"k":3},"filter":{"term":{"kb_id":"kb-1"}}}`),
		VectorFieldName:      []byte(`{"query":{"knn":{"q_2_vec":{"vector":[0.1,0.2,0.3,0.4,0.5],"k":3}}}}`),
	}
	b.ReportAllocs()
	for _, field := range elideFieldNames {
		b.Run(field, func(b *testing.B) {
			body := bodies[field]
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = ElideQueryVector(body, field)
			}
		})
	}
}
